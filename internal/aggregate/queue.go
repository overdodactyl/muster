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

	// IsNew is set by the TUI when this job appeared since the last refresh.
	IsNew bool `json:"is_new,omitempty"`

	// IsSelected is set by the TUI when the user has marked this row for
	// bulk operations.
	IsSelected bool `json:"is_selected,omitempty"`

	// Array-job summary fields, mirror of JobRow's. JobID is the parent
	// array_job_id when ArrayCount > 0.
	ArrayCount  int            `json:"array_count,omitempty"`
	ArrayStates map[string]int `json:"array_states,omitempty"`
}

// Queue returns pending jobs (and running too if includeRunning). reasonFilter
// is a case-insensitive substring on the reason code. sortBy: priority|age|user.
// Array tasks are collapsed by array_job_id (mirrors Jobs); pass
// QueueExpanded for the per-task view.
func Queue(jobs []slurm.Job, partition string, includeRunning bool, reasonFilter, sortBy string, now time.Time) []QueueRow {
	return QueueCollapsed(jobs, partition, includeRunning, false, reasonFilter, sortBy, now)
}

// QueueCollapsed is Queue with explicit control over array-task collapsing.
func QueueCollapsed(jobs []slurm.Job, partition string, includeRunning, expandArrays bool, reasonFilter, sortBy string, now time.Time) []QueueRow {
	if now.IsZero() {
		now = time.Now()
	}
	filter := strings.ToLower(reasonFilter)
	var rows []queueCollapseItem
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
		rows = append(rows, queueCollapseItem{j, row})
	}

	var out []QueueRow
	if expandArrays {
		for _, it := range rows {
			out = append(out, it.row)
		}
	} else {
		out = collapseArraysQueue(rows)
	}
	sortQueue(out, sortBy)
	return out
}

type queueCollapseItem struct {
	raw slurm.Job
	row QueueRow
}

// collapseArraysQueue groups pending-array tasks by array_job_id. Same logic
// as collapseArrays for JobRow but on QueueRow.
func collapseArraysQueue(items []queueCollapseItem) []QueueRow {
	type group struct {
		first QueueRow
		sumCPU, sumGPU, sumMem int
		minPrio                int64
		oldest                 time.Time
		earliestEligible       time.Time
		states                 map[string]int
		count                  int
	}
	groups := map[int64]*group{}
	var out []QueueRow
	for _, it := range items {
		if !it.raw.IsArrayTask() {
			out = append(out, it.row)
			continue
		}
		g, ok := groups[it.raw.ArrayJobID]
		if !ok {
			g = &group{
				first: QueueRow{
					JobID:       it.raw.ArrayJobID,
					User:        it.row.User,
					Account:     it.row.Account,
					Name:        it.row.Name,
					Partition:   it.row.Partition,
					Reason:      it.row.Reason,
					ReasonHuman: it.row.ReasonHuman,
					TimeLimit:   it.row.TimeLimit,
				},
				states:  map[string]int{},
				minPrio: it.row.Priority,
				oldest:  it.row.SubmitTime,
			}
			groups[it.raw.ArrayJobID] = g
		}
		g.count++
		g.sumCPU += it.row.CPUs
		g.sumGPU += it.row.GPUs
		g.sumMem += it.row.MemoryMB
		g.states[it.row.State]++
		if it.row.Priority < g.minPrio {
			g.minPrio = it.row.Priority
		}
		if !it.row.SubmitTime.IsZero() && (g.oldest.IsZero() || it.row.SubmitTime.Before(g.oldest)) {
			g.oldest = it.row.SubmitTime
		}
		if !it.row.EligibleStart.IsZero() && (g.earliestEligible.IsZero() || it.row.EligibleStart.Before(g.earliestEligible)) {
			g.earliestEligible = it.row.EligibleStart
		}
	}
	for _, g := range groups {
		row := g.first
		row.CPUs = g.sumCPU
		row.GPUs = g.sumGPU
		row.MemoryMB = g.sumMem
		row.Priority = g.minPrio
		row.SubmitTime = g.oldest
		row.EligibleStart = g.earliestEligible
		if !g.oldest.IsZero() {
			row.SubmitAge = time.Since(g.oldest)
		}
		row.ArrayCount = g.count
		row.ArrayStates = g.states
		row.State = dominantState(g.states)
		out = append(out, row)
	}
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
