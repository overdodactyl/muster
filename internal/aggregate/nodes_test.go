package aggregate

import (
	"testing"
)

func TestNodes_JoinsUsersFromRunningJobs(t *testing.T) {
	rows := Nodes(sampleNodes(), sampleJobs(), "gpu", nil, false, false)
	if len(rows) != 2 {
		t.Fatalf("expected 2 gpu nodes, got %d", len(rows))
	}
	n01 := rows[0]
	if n01.Name != "n01" {
		t.Fatalf("first row should be n01; got %s", n01.Name)
	}
	if len(n01.Users) != 1 || n01.Users[0] != "alice" {
		t.Errorf("n01 users = %v, want [alice]", n01.Users)
	}
	if n01.JobCount != 2 {
		t.Errorf("n01 job count = %d, want 2", n01.JobCount)
	}
}

func TestNodes_GPUOnlyFilter(t *testing.T) {
	rows := Nodes(sampleNodes(), sampleJobs(), "gpu", nil, true, false)
	if len(rows) != 1 || rows[0].Name != "n01" {
		t.Errorf("--gpu filter should leave only n01, got %+v", rows)
	}
}

func TestNodes_StateFilterCaseInsensitive(t *testing.T) {
	rows := Nodes(sampleNodes(), sampleJobs(), "gpu", []string{"idle"}, false, false)
	if len(rows) != 1 || rows[0].Name != "n02" {
		t.Errorf("state=idle should leave only n02, got %+v", rows)
	}
}
