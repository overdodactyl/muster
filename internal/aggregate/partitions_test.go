package aggregate

import "testing"

func TestPartitions_AggregatesAcrossNodesAndJobs(t *testing.T) {
	rows := Partitions(sampleNodes(), sampleJobs(), "")
	if len(rows) != 2 {
		t.Fatalf("expected 2 partitions (gpu + cpu), got %d", len(rows))
	}

	var dil, cpu *PartitionSummary
	for i := range rows {
		switch rows[i].Name {
		case "gpu":
			dil = &rows[i]
		case "cpu":
			cpu = &rows[i]
		}
	}
	if dil == nil || cpu == nil {
		t.Fatalf("missing partition; got names %+v", rows)
	}

	// gpu: 2 nodes, 64 total CPU, 20 alloc, 1 mixed + 1 idle, 4 GPUs total, 3 alloc
	if dil.TotalNodes != 2 {
		t.Errorf("gpu total nodes = %d, want 2", dil.TotalNodes)
	}
	if dil.TotalCPUs != 64 || dil.AllocCPUs != 20 {
		t.Errorf("gpu CPUs alloc/total = %d/%d, want 20/64", dil.AllocCPUs, dil.TotalCPUs)
	}
	if dil.NodeCounts.Idle != 1 || dil.NodeCounts.Mixed != 1 {
		t.Errorf("gpu node counts wrong: %+v", dil.NodeCounts)
	}
	if dil.TotalGPUs != 4 || dil.AllocGPUs != 3 {
		t.Errorf("gpu GPUs alloc/total = %d/%d, want 3/4", dil.AllocGPUs, dil.TotalGPUs)
	}
	if dil.GPUModel != "a100" {
		t.Errorf("gpu GPU model = %q, want a100", dil.GPUModel)
	}
	if dil.RunningJobs != 2 || dil.PendingJobs != 2 {
		t.Errorf("gpu jobs running/pending = %d/%d, want 2/2", dil.RunningJobs, dil.PendingJobs)
	}

	// cpu: 1 drained node, no jobs in sample
	if cpu.NodeCounts.Drain != 1 {
		t.Errorf("cpu should have 1 drain node, got %+v", cpu.NodeCounts)
	}
	if cpu.RunningJobs != 0 || cpu.PendingJobs != 0 {
		t.Errorf("cpu should have no jobs in sample")
	}
}

func TestPartitions_FilterRestrictsToOne(t *testing.T) {
	rows := Partitions(sampleNodes(), sampleJobs(), "gpu")
	if len(rows) != 1 || rows[0].Name != "gpu" {
		t.Fatalf("filter to gpu did not work: %+v", rows)
	}
}
