package slurm

import (
	"os"
	"path/filepath"
	"testing"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("fixture %s not present: %v", name, err)
	}
	return b
}

func TestParseScontrolNodes_Fixture(t *testing.T) {
	b := loadFixture(t, "scontrol_nodes.json")
	nodes, err := parseScontrolNodes(b)
	if err != nil {
		t.Fatalf("parseScontrolNodes: %v", err)
	}
	if len(nodes) == 0 {
		t.Fatal("no nodes parsed")
	}
	// At least one gpu node should appear (gpu partition exists on this cluster).
	foundGPU := false
	for _, n := range nodes {
		for _, p := range n.Partitions {
			if p == "gpu" {
				foundGPU = true
				if n.CPUs <= 0 {
					t.Errorf("gpu node %s has non-positive CPU count %d", n.Name, n.CPUs)
				}
			}
		}
	}
	if !foundGPU {
		t.Error("no gpu nodes found in fixture")
	}
}

func TestParseScontrolPartitions_Fixture(t *testing.T) {
	b := loadFixture(t, "scontrol_partitions.json")
	parts, err := parseScontrolPartitions(b)
	if err != nil {
		t.Fatalf("parseScontrolPartitions: %v", err)
	}
	if len(parts) == 0 {
		t.Fatal("no partitions parsed")
	}
	names := map[string]bool{}
	for _, p := range parts {
		names[p.Name] = true
	}
	if !names["gpu"] {
		t.Errorf("expected gpu partition, got %v", names)
	}
}

func TestParseSqueueJobs_Fixture(t *testing.T) {
	b := loadFixture(t, "squeue.json")
	jobs, err := parseSqueueJobs(b)
	if err != nil {
		t.Fatalf("parseSqueueJobs: %v", err)
	}
	if len(jobs) == 0 {
		t.Fatal("no jobs parsed")
	}
	// Locate the compact-range row from the array fixture (job 200000).
	var pendingRange *Job
	runningTasks := 0
	for i := range jobs {
		j := &jobs[i]
		if j.ArrayJobID != 200000 {
			continue
		}
		if j.ArrayTaskString != "" {
			pendingRange = j
		} else if j.ArrayTaskID >= 0 {
			runningTasks++
		}
	}
	if runningTasks != 2 {
		t.Errorf("expected 2 running array tasks for 200000, got %d", runningTasks)
	}
	if pendingRange == nil {
		t.Fatal("no pending compact-range row parsed for array 200000")
	}
	if pendingRange.ArrayTaskString != "2-9%3" {
		t.Errorf("ArrayTaskString = %q, want %q", pendingRange.ArrayTaskString, "2-9%3")
	}
	if pendingRange.ArrayTaskCount != 8 {
		t.Errorf("ArrayTaskCount = %d, want 8", pendingRange.ArrayTaskCount)
	}
	if pendingRange.ArrayThrottle != 3 {
		t.Errorf("ArrayThrottle = %d, want 3", pendingRange.ArrayThrottle)
	}
	if !pendingRange.IsArrayTask() {
		t.Error("range row should report IsArrayTask() == true")
	}
}

func TestDecodeJSON_NonJSON(t *testing.T) {
	var dst any
	if err := decodeJSON([]byte("squeue: error: ...\n"), &dst); err == nil {
		t.Errorf("expected error for non-JSON input")
	}
}
