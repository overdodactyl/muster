package aggregate

import (
	"sort"
	"strings"
	"time"

	"muster/internal/slurm"
)

type JobRow struct {
	JobID     int64         `json:"job_id"`
	User      string        `json:"user"`
	Account   string        `json:"account,omitempty"`
	Name      string        `json:"name"`
	Partition string        `json:"partition"`
	State     string        `json:"state"`
	Nodes     string        `json:"nodes,omitempty"`
	CPUs      int           `json:"cpus"`
	GPUs      int           `json:"gpus"`
	MemoryMB  int           `json:"memory_mb"`
	Runtime   time.Duration `json:"runtime_ns"`
	TimeLimit time.Duration `json:"time_limit_ns"`
}

// Jobs returns running jobs (and pending if includePending) sorted by
// the requested key. Pass top=0 to return all rows.
//
//	sortBy: "cpus" (default) | "gpus" | "mem" | "age" | "runtime"
func Jobs(jobs []slurm.Job, partition, user string, includePending bool, sortBy string, top int, now time.Time) []JobRow {
	if now.IsZero() {
		now = time.Now()
	}
	var out []JobRow
	for _, j := range jobs {
		if partition != "" && j.Partition != partition {
			continue
		}
		if user != "" && j.User != user {
			continue
		}
		if j.State != "RUNNING" && !(includePending && j.State == "PENDING") {
			continue
		}
		runtime := time.Duration(0)
		if j.State == "RUNNING" && !j.StartTime.IsZero() {
			runtime = now.Sub(j.StartTime)
		}
		out = append(out, JobRow{
			JobID:     j.ID,
			User:      j.User,
			Account:   j.Account,
			Name:      j.Name,
			Partition: j.Partition,
			State:     j.State,
			Nodes:     j.Nodes,
			CPUs:      j.CPUs,
			GPUs:      jobGPUs(j),
			MemoryMB:  jobMemory(j),
			Runtime:   runtime,
			TimeLimit: j.TimeLimit,
		})
	}
	sortJobs(out, sortBy)
	if top > 0 && len(out) > top {
		out = out[:top]
	}
	return out
}

func sortJobs(rows []JobRow, by string) {
	switch strings.ToLower(by) {
	case "gpus":
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].GPUs > rows[j].GPUs })
	case "mem":
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].MemoryMB > rows[j].MemoryMB })
	case "age", "runtime":
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].Runtime > rows[j].Runtime })
	case "user":
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].User < rows[j].User })
	default: // "cpus"
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].CPUs > rows[j].CPUs })
	}
}
