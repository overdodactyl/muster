package aggregate

import (
	"time"

	"muster/internal/slurm"
)

// sampleNow is a fixed reference timestamp used by tests that compute
// durations against time.Now(). Keeping it deterministic makes assertions
// stable.
var sampleNow = time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)

// sampleNodes returns a small heterogeneous cluster:
//   - n01: gpu, mixed, 4 A100 GPUs (3 in use)
//   - n02: gpu, idle, no GPU
//   - n03: cpu, drain (excluded from healthy partitions)
func sampleNodes() []slurm.Node {
	return []slurm.Node{
		{
			Name:          "n01",
			Partitions:    []string{"gpu"},
			State:         []string{"MIXED"},
			CPUs:          32,
			AllocCPUs:     20,
			IdleCPUs:      12,
			RealMemoryMB:  64000,
			AllocMemoryMB: 40000,
			FreeMemoryMB:  20000,
			GRESTotal:     "gpu:a100:4",
			GRESUsed:      "gpu:a100:3(IDX:0-2)",
		},
		{
			Name:          "n02",
			Partitions:    []string{"gpu"},
			State:         []string{"IDLE"},
			CPUs:          32,
			AllocCPUs:     0,
			IdleCPUs:      32,
			RealMemoryMB:  64000,
			AllocMemoryMB: 0,
			FreeMemoryMB:  60000,
		},
		{
			Name:          "n03",
			Partitions:    []string{"cpu"},
			State:         []string{"DRAIN", "ALLOCATED"},
			CPUs:          16,
			AllocCPUs:     16,
			IdleCPUs:      0,
			RealMemoryMB:  32000,
			AllocMemoryMB: 32000,
			Reason:        "manual maintenance",
		},
	}
}

// sampleJobs returns a mix of running and pending jobs on the sample cluster.
func sampleJobs() []slurm.Job {
	return []slurm.Job{
		{
			ID: 1001, User: "alice", Account: "lab1", Name: "train", Partition: "gpu",
			State: "RUNNING", Nodes: "n01", NodeCount: 1, CPUs: 12,
			MemPerNode: 24000, GRESPerNode: "gres/gpu:1",
			GRESDetail: []string{"gpu:a100:1(IDX:0)"},
			StartTime:  sampleNow.Add(-2 * time.Hour),
			TimeLimit:  8 * time.Hour,
		},
		{
			ID: 1002, User: "alice", Account: "lab1", Name: "train2", Partition: "gpu",
			State: "RUNNING", Nodes: "n01", NodeCount: 1, CPUs: 8,
			MemPerNode: 16000, GRESPerNode: "gres/gpu:2",
			GRESDetail: []string{"gpu:a100:2(IDX:1-2)"},
			StartTime:  sampleNow.Add(-30 * time.Minute),
			TimeLimit:  2 * time.Hour,
		},
		{
			ID: 1003, User: "bob", Account: "lab2", Name: "blocked", Partition: "gpu",
			State: "PENDING", Reason: "Resources",
			NodeCount: 1, CPUs: 16, MemPerNode: 32000,
			SubmitTime: sampleNow.Add(-45 * time.Minute),
			Priority:   100,
			TimeLimit:  1 * time.Hour,
		},
		{
			ID: 1004, User: "carol", Account: "lab3", Name: "begin", Partition: "gpu",
			State: "PENDING", Reason: "BeginTime",
			NodeCount: 1, CPUs: 4, MemPerCPU: 4000,
			SubmitTime: sampleNow.Add(-1 * time.Hour),
			StartTime:  sampleNow.Add(2 * time.Hour),
			Priority:   50,
			TimeLimit:  30 * time.Minute,
		},
	}
}

func sampleAcctJobs() []slurm.AcctJob {
	return []slurm.AcctJob{
		{ // perfectly efficient: 8 cores × 1h, used 7.5h of CPU
			ID: 9001, Name: "good", User: "alice", Partition: "gpu",
			State: "COMPLETED", Elapsed: 1 * time.Hour,
			TotalCPU:  7*time.Hour + 30*time.Minute,
			AllocTRES: map[string]int{"cpu": 8, "mem": 32000},
		},
		{ // inefficient: 16 cores × 2h = 32 core-hours requested, used 1
			ID: 9002, Name: "wasteful", User: "alice", Partition: "gpu",
			State: "COMPLETED", Elapsed: 2 * time.Hour,
			TotalCPU:  1 * time.Hour,
			AllocTRES: map[string]int{"cpu": 16},
		},
		{ // failed, ignored by Usage but counted by History
			ID: 9003, Name: "boom", User: "bob", Partition: "gpu",
			State: "FAILED", Elapsed: 5 * time.Minute,
			TotalCPU:  30 * time.Second,
			AllocTRES: map[string]int{"cpu": 4},
		},
		// Different partition - should be excluded by partition filter.
		{
			ID: 9004, Name: "elsewhere", User: "alice", Partition: "cpu",
			State: "COMPLETED", Elapsed: 1 * time.Hour,
			TotalCPU:  2 * time.Hour, AllocTRES: map[string]int{"cpu": 4},
		},
	}
}
