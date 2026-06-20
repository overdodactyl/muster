package aggregate

import (
	"sort"
	"time"

	"muster/internal/slurm"
)

type UsageRow struct {
	User          string        `json:"user"`
	Jobs          int           `json:"jobs"`
	CPUHoursReq   float64       `json:"cpu_hours_requested"`
	CPUHoursUsed  float64       `json:"cpu_hours_used"`
	Efficiency    float64       `json:"efficiency_pct"`
	WorstJobID    int64         `json:"worst_job_id,omitempty"`
	WorstJobName  string        `json:"worst_job_name,omitempty"`
	WorstJobEff   float64       `json:"worst_job_efficiency_pct,omitempty"`
	WorstJobUsed  time.Duration `json:"worst_job_used_ns,omitempty"`
}

// minElapsedForEff filters out very short jobs from the worst-job calculation;
// a 5-second startup-failure job with 0 CPU isn't a meaningful "worst case."
const minElapsedForEff = 5 * time.Minute

// Usage rolls completed sacct jobs into per-user efficiency metrics. CPU-hours
// requested = alloc_cpu × elapsed_hours; CPU-hours used = total CPU time
// (user+system from the kernel). Efficiency = used/requested. Anything under
// 100% means cores were sitting idle.
func Usage(jobs []slurm.AcctJob, partition string) []UsageRow {
	byUser := map[string]*UsageRow{}
	for _, j := range jobs {
		if partition != "" && j.Partition != partition {
			continue
		}
		// Only consider completed/failed/timeout jobs - running jobs don't yet
		// have finalized cpu time accounting.
		switch j.State {
		case "COMPLETED", "FAILED", "TIMEOUT", "CANCELLED":
		default:
			continue
		}
		allocCPU := j.AllocTRES["cpu"]
		if allocCPU == 0 || j.Elapsed <= 0 {
			continue
		}
		reqHours := float64(allocCPU) * j.Elapsed.Hours()
		usedHours := j.TotalCPU.Hours()

		u, ok := byUser[j.User]
		if !ok {
			u = &UsageRow{User: j.User}
			byUser[j.User] = u
		}
		u.Jobs++
		u.CPUHoursReq += reqHours
		u.CPUHoursUsed += usedHours

		if j.Elapsed >= minElapsedForEff && reqHours > 0 {
			eff := usedHours / reqHours * 100
			if u.WorstJobID == 0 || eff < u.WorstJobEff {
				u.WorstJobID = j.ID
				u.WorstJobName = j.Name
				u.WorstJobEff = eff
				u.WorstJobUsed = j.TotalCPU
			}
		}
	}

	out := make([]UsageRow, 0, len(byUser))
	for _, u := range byUser {
		if u.CPUHoursReq > 0 {
			u.Efficiency = u.CPUHoursUsed / u.CPUHoursReq * 100
		}
		out = append(out, *u)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CPUHoursReq > out[j].CPUHoursReq })
	return out
}
