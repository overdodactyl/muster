package aggregate

import (
	"sort"
	"strings"
	"time"

	"muster/internal/slurm"
)

type QueueRow struct {
	JobID         int64         `json:"job_id"`
	User          string        `json:"user"`
	Account       string        `json:"account,omitempty"`
	Name          string        `json:"name,omitempty"`
	Partition     string        `json:"partition"`
	State         string        `json:"state"`
	CPUs          int           `json:"cpus"`
	GPUs          int           `json:"gpus"`
	MemoryMB      int           `json:"memory_mb"`
	TimeLimit     time.Duration `json:"time_limit_ns"`
	Priority      int64         `json:"priority"`
	Reason        string        `json:"reason,omitempty"`
	ReasonHuman   string        `json:"reason_human,omitempty"`
	SubmitTime    time.Time     `json:"submit_time"`
	SubmitAge     time.Duration `json:"submit_age_ns"`
	EligibleStart time.Time     `json:"eligible_start,omitempty"`
}

// Queue returns pending jobs (and running too if includeRunning). reasonFilter
// is a case-insensitive substring on the reason code. sortBy: priority|age|user.
func Queue(jobs []slurm.Job, partition string, includeRunning bool, reasonFilter, sortBy string, now time.Time) []QueueRow {
	if now.IsZero() {
		now = time.Now()
	}
	filter := strings.ToLower(reasonFilter)
	var out []QueueRow
	for _, j := range jobs {
		if partition != "" && j.Partition != partition {
			continue
		}
		if j.State != "PENDING" && !(includeRunning && j.State == "RUNNING") {
			continue
		}
		if filter != "" && !strings.Contains(strings.ToLower(j.Reason), filter) {
			continue
		}
		row := QueueRow{
			JobID:       j.ID,
			User:        j.User,
			Account:     j.Account,
			Name:        j.Name,
			Partition:   j.Partition,
			State:       j.State,
			CPUs:        j.CPUs,
			GPUs:        jobGPUs(j),
			MemoryMB:    jobMemory(j),
			TimeLimit:   j.TimeLimit,
			Priority:    j.Priority,
			Reason:      j.Reason,
			ReasonHuman: slurm.ExplainReason(j.Reason),
			SubmitTime:  j.SubmitTime,
		}
		if !j.SubmitTime.IsZero() {
			row.SubmitAge = now.Sub(j.SubmitTime)
		}
		if j.Reason == "BeginTime" && !j.StartTime.IsZero() {
			row.EligibleStart = j.StartTime
			row.ReasonHuman = "Holding until " + j.StartTime.Format("2006-01-02 15:04")
		}
		out = append(out, row)
	}
	sortQueue(out, sortBy)
	return out
}

func sortQueue(rows []QueueRow, by string) {
	switch strings.ToLower(by) {
	case "age":
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].SubmitAge > rows[j].SubmitAge })
	case "user":
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].User < rows[j].User })
	default: // "priority"
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].Priority > rows[j].Priority })
	}
}
