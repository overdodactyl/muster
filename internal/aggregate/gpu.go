package aggregate

import (
	"sort"
	"strings"
	"time"

	"muster/internal/slurm"
)

type GPURow struct {
	Node    string        `json:"node"`
	Model   string        `json:"model,omitempty"`
	Index   int           `json:"index"`
	InUse   bool          `json:"in_use"`
	User    string        `json:"user,omitempty"`
	JobID   int64         `json:"job_id,omitempty"`
	JobName string        `json:"job_name,omitempty"`
	Runtime time.Duration `json:"runtime_ns,omitempty"`
}

// GPUs returns one row per (node, gpu-index) for every GPU-bearing node in the
// (optional) partition. Each row is attributed to the user/job/jobname holding
// that index, by joining the running jobs' gres_detail strings to nodes.
func GPUs(nodes []slurm.Node, jobs []slurm.Job, partition string, now time.Time) []GPURow {
	if now.IsZero() {
		now = time.Now()
	}

	// Build node -> []runningJob (jobs are running, on this node, with GPU detail).
	nodeToJobs := map[string][]*slurm.Job{}
	for i := range jobs {
		j := &jobs[i]
		if j.State != "RUNNING" || len(j.GRESDetail) == 0 {
			continue
		}
		expanded, err := slurm.ExpandHostlist(j.Nodes)
		if err != nil {
			continue
		}
		for _, host := range expanded {
			nodeToJobs[host] = append(nodeToJobs[host], j)
		}
	}

	var out []GPURow
	for _, n := range nodes {
		if partition != "" {
			ok := false
			for _, p := range n.Partitions {
				if p == partition {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		gres := slurm.ParseGRES(n.GRESTotal)
		totalGPUs := slurm.GPUCount(gres)
		if totalGPUs == 0 {
			continue
		}
		model := slurm.GPUModel(gres)

		// indexHolder[i] -> the job assigned GPU index i on this node.
		indexHolder := map[int]*slurm.Job{}
		for _, j := range nodeToJobs[n.Name] {
			for _, detail := range j.GRESDetail {
				for _, parsed := range slurm.ParseGRES(detail) {
					if parsed.Kind != "gpu" {
						continue
					}
					for _, idx := range parsed.Index {
						if _, taken := indexHolder[idx]; !taken {
							indexHolder[idx] = j
						}
					}
				}
			}
		}

		// Emit a row per index 0..total-1.
		for i := 0; i < totalGPUs; i++ {
			row := GPURow{Node: n.Name, Model: model, Index: i}
			if j, ok := indexHolder[i]; ok {
				row.InUse = true
				row.User = j.User
				row.JobID = j.ID
				row.JobName = j.Name
				if !j.StartTime.IsZero() {
					row.Runtime = now.Sub(j.StartTime)
				}
			}
			out = append(out, row)
		}
	}

	// Sort by node, then index.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Node != out[j].Node {
			return out[i].Node < out[j].Node
		}
		return out[i].Index < out[j].Index
	})

	// Strip empty whitespace from job names (defensive).
	for i := range out {
		out[i].JobName = strings.TrimSpace(out[i].JobName)
	}
	return out
}
