package aggregate

import "testing"

func TestGPUs_AttributesIndicesToJobs(t *testing.T) {
	rows := GPUs(sampleNodes(), sampleJobs(), "gpu", sampleNow)
	if len(rows) != 4 {
		t.Fatalf("n01 has 4 GPUs; expected 4 rows, got %d", len(rows))
	}
	// Rows are sorted by (node, index). Indices 0/1/2 are in use, 3 is idle.
	used := map[int]string{0: "alice", 1: "alice", 2: "alice"}
	for i := 0; i < 4; i++ {
		got := rows[i]
		if got.Index != i {
			t.Errorf("row %d should have Index=%d, got %d", i, i, got.Index)
		}
		if owner, ok := used[i]; ok {
			if !got.InUse || got.User != owner {
				t.Errorf("idx %d should be in use by %s; got InUse=%v user=%s", i, owner, got.InUse, got.User)
			}
		} else {
			if got.InUse {
				t.Errorf("idx %d should be idle; got user=%s", i, got.User)
			}
		}
	}
}

func TestGPUs_SkipsNodesWithoutGPUs(t *testing.T) {
	rows := GPUs(sampleNodes(), sampleJobs(), "gpu", sampleNow)
	for _, r := range rows {
		if r.Node != "n01" {
			t.Errorf("only n01 has GPUs; got row on %s", r.Node)
		}
	}
}
