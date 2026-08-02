package aggregate

import (
	"fmt"
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

	// Array-job aggregation: when this row represents a collapsed array
	// (ArrayCount > 1), JobID holds the array's parent ID and ArrayStates
	// holds counts per Slurm job_state. Single jobs leave ArrayCount=0.
	ArrayCount    int            `json:"array_count,omitempty"`
	ArrayStates   map[string]int `json:"array_states,omitempty"`
	ArrayThrottle int            `json:"array_throttle,omitempty"` // %N from array_max_tasks

	// Set on expanded (per-task) rows that represent a compact pending
	// range like "73-224%3" — one row covering ArrayTaskCount tasks.
	// Empty on ordinary single-task rows.
	ArrayTaskString string `json:"array_task_string,omitempty"`
	ArrayTaskCount  int    `json:"array_task_count,omitempty"`

	// IsNew is set by the TUI (not aggregate) when this job appeared since
	// the previous refresh — used to flash a green marker on the row.
	IsNew bool `json:"is_new,omitempty"`

	// IsSelected is set by the TUI when the user has marked this row with
	// Space for bulk operations (e.g., cancel-many).
	IsSelected bool `json:"is_selected,omitempty"`
}

// Jobs returns running jobs (and pending if includePending) sorted by
// the requested key. Array-task jobs are collapsed by their array_job_id
// into one summary row by default; pass expandArrays=true to get one row
// per task (e.g. for static --expand-arrays).
//
//	sortBy: "cpus" (default) | "gpus" | "mem" | "age" | "runtime"
func Jobs(jobs []slurm.Job, partition, user string, includePending bool, sortBy string, top int, now time.Time) []JobRow {
	return JobsCollapsed(jobs, partition, user, includePending, false, sortBy, top, now)
}

// JobsCollapsed is Jobs with control over array-job collapsing. When
// expandArrays is true, each array task gets its own row; otherwise the
// tasks are merged into a single summary row per array_job_id.
//
// To drill into a single array, pass expandArrays=true and filter the
// caller's input to jobs whose ArrayJobID matches.
func JobsCollapsed(jobs []slurm.Job, partition, user string, includePending, expandArrays bool, sortBy string, top int, now time.Time) []JobRow {
	if now.IsZero() {
		now = time.Now()
	}
	// First pass: filter to in-scope jobs and convert to raw rows.
	var rows []arrayCollapseItem
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
		rows = append(rows, arrayCollapseItem{j, JobRow{
			JobID:           j.ID,
			User:            j.User,
			Account:         j.Account,
			Name:            j.Name,
			Partition:       j.Partition,
			State:           j.State,
			Nodes:           j.Nodes,
			CPUs:            j.CPUs,
			GPUs:            jobGPUs(j),
			MemoryMB:        jobMemory(j),
			Runtime:         runtime,
			TimeLimit:       j.TimeLimit,
			ArrayThrottle:   j.ArrayThrottle,
			ArrayTaskString: j.ArrayTaskString,
			ArrayTaskCount:  j.ArrayTaskCount,
		}})
	}

	var out []JobRow
	if expandArrays {
		for _, it := range rows {
			out = append(out, it.row)
		}
	} else {
		out = collapseArrays(rows)
	}
	sortJobs(out, sortBy)
	if top > 0 && len(out) > top {
		out = out[:top]
	}
	return out
}

// arrayCollapseItem pairs the raw slurm.Job (for the IsArrayTask check) with
// the already-built JobRow that would be emitted if there were no collapsing.
type arrayCollapseItem struct {
	raw slurm.Job
	row JobRow
}

// collapseArrays groups array tasks (job.IsArrayTask()) by array_job_id into
// one summary row. Non-array jobs pass through untouched. Aggregated fields
// (CPUs, GPUs, MemoryMB) sum across tasks; Runtime is the longest of the
// running tasks; TimeLimit takes the first non-zero value seen.
//
// A compact-range row (raw.ArrayTaskString != "") represents many pending
// tasks in one input row; its ArrayTaskCount contributes to the group's
// count and state histogram, but per-task resource figures are NOT summed —
// multiplying a placeholder allocation by hundreds of pending tasks would
// give misleading totals.
func collapseArrays(items []arrayCollapseItem) []JobRow {
	type group struct {
		first                  JobRow
		sumCPU, sumGPU, sumMem int
		maxRuntime             time.Duration
		timeLimit              time.Duration
		throttle               int
		states                 map[string]int
		count                  int
		nodes                  map[string]bool
	}
	groups := map[int64]*group{}
	var out []JobRow
	for _, it := range items {
		if !it.raw.IsArrayTask() {
			out = append(out, it.row)
			continue
		}
		g, ok := groups[it.raw.ArrayJobID]
		if !ok {
			g = &group{
				first: JobRow{
					JobID:     it.raw.ArrayJobID,
					User:      it.row.User,
					Account:   it.row.Account,
					Name:      it.row.Name,
					Partition: it.row.Partition,
				},
				states: map[string]int{},
				nodes:  map[string]bool{},
			}
			groups[it.raw.ArrayJobID] = g
		}
		if g.throttle == 0 && it.raw.ArrayThrottle > 0 {
			g.throttle = it.raw.ArrayThrottle
		}
		if it.raw.ArrayTaskString != "" {
			n := it.raw.ArrayTaskCount
			if n <= 0 {
				n = 1
			}
			g.count += n
			g.states[it.row.State] += n
			continue
		}
		g.count++
		g.sumCPU += it.row.CPUs
		g.sumGPU += it.row.GPUs
		g.sumMem += it.row.MemoryMB
		if it.row.Runtime > g.maxRuntime {
			g.maxRuntime = it.row.Runtime
		}
		if g.timeLimit == 0 && it.row.TimeLimit > 0 {
			g.timeLimit = it.row.TimeLimit
		}
		g.states[it.row.State]++
		if it.row.Nodes != "" {
			g.nodes[it.row.Nodes] = true
		}
	}
	for _, g := range groups {
		row := g.first
		row.CPUs = g.sumCPU
		row.GPUs = g.sumGPU
		row.MemoryMB = g.sumMem
		row.Runtime = g.maxRuntime
		row.TimeLimit = g.timeLimit
		row.ArrayCount = g.count
		row.ArrayStates = g.states
		row.ArrayThrottle = g.throttle
		// Pick a representative state for sorting/coloring.
		row.State = dominantState(g.states)
		// Nodes summary: count if multi-node, else show the single nodelist.
		if len(g.nodes) == 1 {
			for n := range g.nodes {
				row.Nodes = n
			}
		} else if len(g.nodes) > 1 {
			row.Nodes = fmt.Sprintf("%d nodes", len(g.nodes))
		}
		out = append(out, row)
	}
	return out
}

func dominantState(counts map[string]int) string {
	// Prefer RUNNING > PENDING > anything else.
	if counts["RUNNING"] > 0 {
		return "RUNNING"
	}
	if counts["PENDING"] > 0 {
		return "PENDING"
	}
	best := ""
	bestN := 0
	for s, n := range counts {
		if n > bestN {
			best = s
			bestN = n
		}
	}
	return best
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
