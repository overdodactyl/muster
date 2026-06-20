package aggregate

import "testing"

func TestHistory_RollupByUser(t *testing.T) {
	rows := History(sampleAcctJobs(), "user", "gpu", nil)
	if len(rows) != 2 {
		t.Fatalf("expected 2 user rollups in gpu, got %d", len(rows))
	}
	// Alice: 8c × 1h + 16c × 2h = 40 cpu-hrs
	var alice *HistoryRow
	for i := range rows {
		if rows[i].Key == "alice" {
			alice = &rows[i]
			break
		}
	}
	if alice == nil || alice.CPUHours != 40.0 {
		t.Errorf("alice cpu-hours = %v, want 40", alice)
	}
	if alice.Jobs != 2 || alice.Completed != 2 {
		t.Errorf("alice jobs/completed wrong: %+v", *alice)
	}
}

func TestHistory_PartitionFilterExcludesElsewhere(t *testing.T) {
	rows := History(sampleAcctJobs(), "user", "gpu", nil)
	for _, r := range rows {
		if r.Key == "alice" && r.Jobs != 2 {
			t.Errorf("gpu should exclude the cpu job for alice; jobs=%d", r.Jobs)
		}
	}
}

func TestHistory_RollupByState(t *testing.T) {
	rows := History(sampleAcctJobs(), "state", "", nil)
	counts := map[string]int{}
	for _, r := range rows {
		counts[r.Key] = r.Jobs
	}
	if counts["COMPLETED"] != 3 || counts["FAILED"] != 1 {
		t.Errorf("rollup by state wrong: %+v", counts)
	}
}
