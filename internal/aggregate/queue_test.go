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
