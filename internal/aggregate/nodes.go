package aggregate

import (
	"sort"
	"strings"

	"muster/internal/slurm"
)

type NodeRow struct {
	Name        string   `json:"name"`
	Partition   string   `json:"partition"`
	State       []string `json:"state"`
	StateClass  string   `json:"state_class"`
	CPUsAlloc   int      `json:"cpus_alloc"`
	CPUsIdle    int      `json:"cpus_idle"`
	CPUsTotal   int      `json:"cpus_total"`
	MemAllocMB  int      `json:"mem_alloc_mb"`
	MemFreeMB   int      `json:"mem_free_mb"`
	MemTotalMB  int      `json:"mem_total_mb"`
	GPUsAlloc   int      `json:"gpus_alloc"`
	GPUsTotal   int      `json:"gpus_total"`
	GPUModel    string   `json:"gpu_model,omitempty"`
	Users       []string `json:"users,omitempty"`
	UserJobs    []string `json:"user_jobs,omitempty"` // "user(jobid)" when --show-jobs
	JobCount    int      `json:"job_count"`
	Reason      string   `json:"reason,omitempty"`
}

// Nodes builds per-node rows for the given partition (empty = all). Joins jobs
// to nodes via hostlist expansion. stateFilter is a list of state names to
// include (case-insensitive); empty = all. gpuOnly drops nodes with no GPUs.
func Nodes(nodes []slurm.Node, jobs []slurm.Job, partition string, stateFilter []string, gpuOnly bool, showJobs bool) []NodeRow {
	// Build map of node -> []job for the user-on-node join.
	nodeToJobs := map[string][]*slurm.Job{}
	for i := range jobs {
		j := &jobs[i]
		if j.State != "RUNNING" {
			continue
		}
		if j.Nodes == "" {
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

	filterSet := map[string]bool{}
	for _, s := range stateFilter {
		filterSet[strings.ToUpper(strings.TrimSpace(s))] = true
	}

	var out []NodeRow
	for _, n := range nodes {
		// Show this node under each of its partitions that matches the filter.
		for _, p := range n.Partitions {
			if partition != "" && p != partition {
				continue
			}
			class := slurm.Classify(n.State)
			if len(filterSet) > 0 {
				keep := false
				for _, st := range n.State {
					if filterSet[strings.ToUpper(st)] {
						keep = true
						break
					}
				}
				if !keep && !filterSet[strings.ToUpper(class.String())] {
					continue
				}
			}
			gres := slurm.ParseGRES(n.GRESTotal)
			gresUsed := slurm.ParseGRES(n.GRESUsed)
			gpuT := slurm.GPUCount(gres)
			if gpuOnly && gpuT == 0 {
				continue
			}

			users := map[string]bool{}
			var userJobs []string
			jobsOnNode := nodeToJobs[n.Name]
			for _, j := range jobsOnNode {
				users[j.User] = true
				if showJobs {
					userJobs = append(userJobs, j.User+"("+itoa64(j.ID)+")")
				}
			}
			userList := make([]string, 0, len(users))
			for u := range users {
				userList = append(userList, u)
			}
			sort.Strings(userList)
			sort.Strings(userJobs)

			row := NodeRow{
				Name:       n.Name,
				Partition:  p,
				State:      n.State,
				StateClass: class.String(),
				CPUsAlloc:  n.AllocCPUs,
				CPUsIdle:   n.IdleCPUs,
				CPUsTotal:  n.CPUs,
				MemAllocMB: n.AllocMemoryMB,
				MemFreeMB:  n.FreeMemoryMB,
				MemTotalMB: n.RealMemoryMB,
				GPUsAlloc:  slurm.GPUCount(gresUsed),
				GPUsTotal:  gpuT,
				GPUModel:   slurm.GPUModel(gres),
				Users:      userList,
				UserJobs:   userJobs,
				JobCount:   len(jobsOnNode),
				Reason:     n.Reason,
			}
			out = append(out, row)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Partition != out[j].Partition {
			return out[i].Partition < out[j].Partition
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
