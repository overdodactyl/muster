package aggregate

import "testing"

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
