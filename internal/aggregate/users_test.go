package aggregate

import (
	"testing"
)

func TestUsers_RollupAndSort(t *testing.T) {
	rows := Users(sampleJobs(), "gpu", "", "cpus", 0, sampleNow)
	if len(rows) != 3 {
		t.Fatalf("expected 3 users (alice, bob, carol), got %d", len(rows))
	}

	// Alice has 2 running jobs holding 12+8=20 CPUs and 1+2=3 GPUs.
	var alice *UserRollup
	for i := range rows {
		if rows[i].User == "alice" {
			alice = &rows[i]
			break
		}
	}
	if alice == nil {
		t.Fatal("alice missing")
	}
	if alice.Running != 2 || alice.CPUsHeld != 20 || alice.GPUsHeld != 3 {
		t.Errorf("alice rollup wrong: %+v", *alice)
	}
	// Oldest running age should be 2h (1001 started 2h before sampleNow).
	if alice.OldestRunAge < (2 * 60 * 60 * 1e9) {
		t.Errorf("alice oldest run age = %v, want >= 2h", alice.OldestRunAge)
	}
}

func TestUsers_PendingOnlyShowsAsPending(t *testing.T) {
	rows := Users(sampleJobs(), "gpu", "carol", "cpus", 0, sampleNow)
	if len(rows) != 1 {
		t.Fatalf("carol should yield 1 row, got %d", len(rows))
	}
	if rows[0].Running != 0 || rows[0].Pending != 1 {
		t.Errorf("carol should be 0 R / 1 PD, got %+v", rows[0])
	}
}
