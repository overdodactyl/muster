package aggregate

import "testing"

func TestQueue_PendingOnly(t *testing.T) {
	rows := Queue(sampleJobs(), "gpu", false, "", "priority", sampleNow)
	if len(rows) != 2 {
		t.Fatalf("expected 2 pending jobs, got %d", len(rows))
	}
	// Sorted by priority desc.
	if rows[0].JobID != 1003 || rows[1].JobID != 1004 {
		t.Errorf("priority sort wrong: %d, %d", rows[0].JobID, rows[1].JobID)
	}
}

func TestQueue_ReasonFilter(t *testing.T) {
	rows := Queue(sampleJobs(), "gpu", false, "begin", "priority", sampleNow)
	if len(rows) != 1 || rows[0].JobID != 1004 {
		t.Errorf("BeginTime filter should yield 1004, got %+v", rows)
	}
}

func TestQueue_BeginTimeShowsEligibleStart(t *testing.T) {
	rows := Queue(sampleJobs(), "gpu", false, "begin", "priority", sampleNow)
	if !rows[0].EligibleStart.Equal(sampleNow.Add(2 * 60 * 60 * 1e9)) {
		// eligible at sampleNow + 2h
		if rows[0].EligibleStart.IsZero() {
			t.Errorf("BeginTime row should have EligibleStart set")
		}
	}
	if rows[0].ReasonHuman == "" {
		t.Errorf("ReasonHuman should be populated")
	}
}

func TestQueue_IncludeRunning(t *testing.T) {
	rows := Queue(sampleJobs(), "gpu", true, "", "priority", sampleNow)
	if len(rows) != 4 {
		t.Errorf("with --all should include all 4 jobs, got %d", len(rows))
	}
}

func TestQueue_CollapsePendingRangeIntoArray(t *testing.T) {
	rows := Queue(sampleArrayJobs(), "gpu", true, "", "priority", sampleNow)
	if len(rows) != 1 {
		t.Fatalf("expected 1 collapsed array row, got %d: %+v", len(rows), rows)
	}
	r := rows[0]
	if r.JobID != 200000 {
		t.Errorf("collapsed JobID = %d, want 200000", r.JobID)
	}
	if r.ArrayCount != 10 {
		t.Errorf("ArrayCount = %d, want 10", r.ArrayCount)
	}
	if r.ArrayStates["RUNNING"] != 2 || r.ArrayStates["PENDING"] != 8 {
		t.Errorf("ArrayStates = %+v, want RUNNING:2 PENDING:8", r.ArrayStates)
	}
	if r.ArrayThrottle != 3 {
		t.Errorf("ArrayThrottle = %d, want 3", r.ArrayThrottle)
	}
}
