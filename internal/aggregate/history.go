package aggregate

import (
	"sort"
	"strings"

	"muster/internal/slurm"
)

type HistoryRow struct {
	Key           string  `json:"key"` // user / account / state / partition
	Jobs          int     `json:"jobs"`
	Completed     int     `json:"completed"`
	Failed        int     `json:"failed"`
	Timeout       int     `json:"timeout"`
	Cancelled     int     `json:"cancelled"`
	CPUHours      float64 `json:"cpu_hours"`
	GPUHours      float64 `json:"gpu_hours"`
	MemoryGBHours float64 `json:"memory_gb_hours"`
}

// History rolls sacct jobs up by the chosen key.
//
//	by: "user" (default) | "account" | "state" | "partition"
//	stateFilter: case-insensitive set; empty = all
func History(jobs []slurm.AcctJob, by, partition string, stateFilter []string) []HistoryRow {
	stateSet := map[string]bool{}
	for _, s := range stateFilter {
		stateSet[strings.ToUpper(s)] = true
	}

	keyFn := keyFunc(by)
	byKey := map[string]*HistoryRow{}
	for _, j := range jobs {
		if partition != "" && j.Partition != partition {
			continue
		}
		if len(stateSet) > 0 && !stateSet[strings.ToUpper(j.State)] {
			continue
		}
		k := keyFn(j)
		if k == "" {
			continue
		}
		row, ok := byKey[k]
		if !ok {
			row = &HistoryRow{Key: k}
			byKey[k] = row
		}
		row.Jobs++
		switch strings.ToUpper(j.State) {
		case "COMPLETED":
			row.Completed++
		case "FAILED":
			row.Failed++
		case "TIMEOUT":
			row.Timeout++
		case "CANCELLED":
			row.Cancelled++
		}
		hours := j.Elapsed.Hours()
		if hours > 0 {
			cpu := j.AllocTRES["cpu"]
			gpu := j.AllocTRES["gres/gpu"]
			mem := j.AllocTRES["mem"]
			row.CPUHours += float64(cpu) * hours
			row.GPUHours += float64(gpu) * hours
			row.MemoryGBHours += (float64(mem) / 1024.0) * hours
		}
	}
	out := make([]HistoryRow, 0, len(byKey))
	for _, r := range byKey {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CPUHours != out[j].CPUHours {
			return out[i].CPUHours > out[j].CPUHours
		}
		return out[i].Key < out[j].Key
	})
	return out
}

func keyFunc(by string) func(slurm.AcctJob) string {
	switch strings.ToLower(by) {
	case "account":
		return func(j slurm.AcctJob) string { return j.Account }
	case "state":
		return func(j slurm.AcctJob) string { return j.State }
	case "partition":
		return func(j slurm.AcctJob) string { return j.Partition }
	default:
		return func(j slurm.AcctJob) string { return j.User }
	}
}
