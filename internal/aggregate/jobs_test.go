package aggregate

import (
	"testing"
	"time"

	"muster/internal/slurm"
)

func TestJobs_FiltersToRunningByDefault(t *testing.T) {
	rows := Jobs(sampleJobs(), "gpu", "", false, "cpus", 0, sampleNow)
	if len(rows) != 2 {
		t.Fatalf("expected 2 running jobs, got %d", len(rows))
	}
	if rows[0].CPUs != 12 || rows[1].CPUs != 8 {
		t.Errorf("default sort=cpus should be 12,8; got %d,%d", rows[0].CPUs, rows[1].CPUs)
	}
}

func TestJobs_IncludePendingAndSortByGPUs(t *testing.T) {
	rows := Jobs(sampleJobs(), "gpu", "", true, "gpus", 0, sampleNow)
	if len(rows) != 4 {
		t.Fatalf("expected 4 (2 running + 2 pending), got %d", len(rows))
	}
	// Highest GPU job is 1002 (2 GPUs)
	if rows[0].JobID != 1002 {
		t.Errorf("top GPU job should be 1002, got %d", rows[0].JobID)
	}
}

func TestJobs_UserFilter(t *testing.T) {
	rows := Jobs(sampleJobs(), "gpu", "bob", true, "cpus", 0, sampleNow)
	if len(rows) != 1 || rows[0].User != "bob" {
		t.Errorf("user=bob should yield 1 row, got %+v", rows)
	}
}

func TestJobs_TopN(t *testing.T) {
	rows := Jobs(sampleJobs(), "gpu", "", true, "cpus", 2, sampleNow)
	if len(rows) != 2 {
		t.Errorf("top=2 should yield 2 rows, got %d", len(rows))
	}
}

// sampleArrayJobs mirrors what parseSqueueJobs emits for a %3-throttled array
// with 2 running tasks and a compact pending range covering 8 more tasks.
func sampleArrayJobs() []slurm.Job {
	return []slurm.Job{
		{
			ID: 200001, User: "carol", Account: "labC", Name: "sweep", Partition: "gpu",
			State: "RUNNING", Nodes: "n01", NodeCount: 1, CPUs: 4,
			MemPerNode: 8000, GRESPerNode: "gres/gpu:1",
			StartTime:      sampleNow.Add(-10 * time.Minute),
			TimeLimit:      1 * time.Hour,
			ArrayJobID:     200000,
			ArrayTaskID:    0,
			ArrayTaskCount: 1,
			ArrayThrottle:  3,
		},
		{
			ID: 200002, User: "carol", Account: "labC", Name: "sweep", Partition: "gpu",
			State: "RUNNING", Nodes: "n01", NodeCount: 1, CPUs: 4,
			MemPerNode: 8000, GRESPerNode: "gres/gpu:1",
			StartTime:      sampleNow.Add(-5 * time.Minute),
			TimeLimit:      1 * time.Hour,
			ArrayJobID:     200000,
			ArrayTaskID:    1,
			ArrayTaskCount: 1,
			ArrayThrottle:  3,
		},
		{
			ID: 200000, User: "carol", Account: "labC", Name: "sweep", Partition: "gpu",
			State:  "PENDING",
			Reason: "JobArrayTaskLimit",
			NodeCount: 1, CPUs: 4, MemPerNode: 8000, GRESPerNode: "gres/gpu:1",
			SubmitTime:      sampleNow.Add(-20 * time.Minute),
			Priority:        900,
			TimeLimit:       1 * time.Hour,
			ArrayJobID:      200000,
			ArrayTaskID:     -1,
			ArrayTaskString: "2-9%3",
			ArrayTaskCount:  8,
			ArrayThrottle:   3,
		},
	}
}

func TestJobs_CollapsePendingRangeIntoArray(t *testing.T) {
	rows := Jobs(sampleArrayJobs(), "gpu", "", true, "cpus", 0, sampleNow)
	if len(rows) != 1 {
		t.Fatalf("expected 1 collapsed array row, got %d: %+v", len(rows), rows)
	}
	r := rows[0]
	if r.JobID != 200000 {
		t.Errorf("collapsed JobID = %d, want 200000", r.JobID)
	}
	if r.ArrayCount != 10 {
		t.Errorf("ArrayCount = %d, want 10 (2 running + 8 pending)", r.ArrayCount)
	}
	if r.ArrayStates["RUNNING"] != 2 || r.ArrayStates["PENDING"] != 8 {
		t.Errorf("ArrayStates = %+v, want RUNNING:2 PENDING:8", r.ArrayStates)
	}
	if r.ArrayThrottle != 3 {
		t.Errorf("ArrayThrottle = %d, want 3", r.ArrayThrottle)
	}
	// Resource sums = 2 running tasks + 1 sample from the range row (to
	// surface per-task allocation on pure-pending arrays). Not multiplied
	// by all 8 pending tasks.
	if r.CPUs != 12 {
		t.Errorf("CPUs = %d, want 12 (2 running + 1 range × 4)", r.CPUs)
	}
	if r.GPUs != 3 {
		t.Errorf("GPUs = %d, want 3", r.GPUs)
	}
	if r.MemoryMB != 24000 {
		t.Errorf("MemoryMB = %d, want 24000", r.MemoryMB)
	}
	if r.State != "RUNNING" {
		t.Errorf("dominant State = %q, want RUNNING", r.State)
	}
}

func TestJobs_PurePendingArrayShowsPerTaskResources(t *testing.T) {
	// Array with only a compact-range row (no running tasks yet). The
	// collapsed row should surface per-task allocation, not 0.
	jobs := []slurm.Job{
		{
			ID: 300000, User: "dave", Partition: "gpu", State: "PENDING",
			Reason:    "Resources",
			NodeCount: 1, CPUs: 8, MemPerNode: 32000, GRESPerNode: "gres/gpu:1",
			SubmitTime:      sampleNow.Add(-1 * time.Hour),
			ArrayJobID:      300000,
			ArrayTaskID:     -1,
			ArrayTaskString: "0-9",
			ArrayTaskCount:  10,
		},
	}
	rows := Jobs(jobs, "gpu", "", true, "cpus", 0, sampleNow)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	r := rows[0]
	if r.CPUs != 8 || r.GPUs != 1 || r.MemoryMB != 32000 {
		t.Errorf("pure-pending array should show per-task resources; got CPUs=%d GPUs=%d MemMB=%d",
			r.CPUs, r.GPUs, r.MemoryMB)
	}
	if r.ArrayCount != 10 {
		t.Errorf("ArrayCount = %d, want 10", r.ArrayCount)
	}
}

func TestJobs_ExpandPendingRangeAsSingleRow(t *testing.T) {
	rows := JobsCollapsed(sampleArrayJobs(), "gpu", "", true, true, "cpus", 0, sampleNow)
	if len(rows) != 3 {
		t.Fatalf("expected 3 expanded rows (2 running + 1 range), got %d", len(rows))
	}
	var rangeRow *JobRow
	for i := range rows {
		if rows[i].ArrayTaskString != "" {
			rangeRow = &rows[i]
		}
	}
	if rangeRow == nil {
		t.Fatal("no expanded range row found")
	}
	if rangeRow.ArrayTaskString != "2-9%3" || rangeRow.ArrayTaskCount != 8 {
		t.Errorf("range row = %+v", rangeRow)
	}
}
