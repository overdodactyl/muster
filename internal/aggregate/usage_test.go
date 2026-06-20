package aggregate

import "testing"

func TestUsage_ComputesEfficiency(t *testing.T) {
	rows := Usage(sampleAcctJobs(), "gpu")
	// Both alice (2 jobs) and bob (1 failed job) consumed resources; both appear.
	if len(rows) != 2 {
		t.Fatalf("expected alice + bob (incl. failed) rollups, got %d", len(rows))
	}
	var a *UsageRow
	for i := range rows {
		if rows[i].User == "alice" {
			a = &rows[i]
		}
	}
	if a == nil {
		t.Fatal("alice missing")
	}
	// Requested: 8*1 + 16*2 = 40 cpu-hours
	// Used: 7.5 + 1 = 8.5 cpu-hours
	// Eff: 8.5/40 = 21.25%
	if a.CPUHoursReq != 40 {
		t.Errorf("requested cpu-hours = %v, want 40", a.CPUHoursReq)
	}
	if a.CPUHoursUsed != 8.5 {
		t.Errorf("used cpu-hours = %v, want 8.5", a.CPUHoursUsed)
	}
	if a.Efficiency < 21 || a.Efficiency > 22 {
		t.Errorf("efficiency = %v%%, want ~21.25", a.Efficiency)
	}
	// Worst-offender is the wasteful 2h job (uses 1/16 = ~3%, well below the good one's 7.5/8 = 94%)
	if a.WorstJobID != 9002 {
		t.Errorf("worst-job should be 9002, got %d", a.WorstJobID)
	}
}

func TestUsage_IncludesFailedJobs(t *testing.T) {
	// FAILED/TIMEOUT/CANCELLED jobs still consumed cluster time and count for
	// efficiency calculations.
	rows := Usage(sampleAcctJobs(), "")
	found := false
	for _, r := range rows {
		if r.User == "bob" {
			found = true
			if r.Jobs != 1 {
				t.Errorf("bob should have 1 job, got %d", r.Jobs)
			}
		}
	}
	if !found {
		t.Errorf("bob's failed job should still appear in usage")
	}
}
