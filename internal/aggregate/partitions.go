package aggregate

import (
	"sort"

	"muster/internal/slurm"
)

type StateCounts struct {
	Idle, Mixed, Alloc, Down, Drain, Reserved, Other int
}

type PartitionSummary struct {
	Name        string      `json:"name"`
	NodeCounts  StateCounts `json:"node_counts"`
	TotalNodes  int         `json:"total_nodes"`
	AllocCPUs   int         `json:"alloc_cpus"`
	IdleCPUs    int         `json:"idle_cpus"`
	TotalCPUs   int         `json:"total_cpus"`
	AllocGPUs   int         `json:"alloc_gpus"`
	TotalGPUs   int         `json:"total_gpus"`
	GPUModel    string      `json:"gpu_model,omitempty"`
	AllocMemMB  int         `json:"alloc_memory_mb"`
	FreeMemMB   int         `json:"free_memory_mb"`
	TotalMemMB  int         `json:"total_memory_mb"`
	RunningJobs int         `json:"running_jobs"`
	PendingJobs int         `json:"pending_jobs"`
}

// Partitions builds per-partition rollups. A node assigned to multiple
// partitions contributes its resources to each one (Slurm allows overlap).
// Pass partitionFilter="" to include all partitions.
func Partitions(nodes []slurm.Node, jobs []slurm.Job, partitionFilter string) []PartitionSummary {
	summaries := map[string]*PartitionSummary{}
	get := func(name string) *PartitionSummary {
		s, ok := summaries[name]
		if !ok {
			s = &PartitionSummary{Name: name}
			summaries[name] = s
		}
		return s
	}

	for _, n := range nodes {
		gres := slurm.ParseGRES(n.GRESTotal)
		gresUsed := slurm.ParseGRES(n.GRESUsed)
		nodeGPUs := slurm.GPUCount(gres)
		usedGPUs := slurm.GPUCount(gresUsed)
		model := slurm.GPUModel(gres)
		class := slurm.Classify(n.State)

		for _, p := range n.Partitions {
			if partitionFilter != "" && p != partitionFilter {
				continue
			}
			s := get(p)
			s.TotalNodes++
			switch class {
			case slurm.StateIdle:
				s.NodeCounts.Idle++
			case slurm.StateMixed:
				s.NodeCounts.Mixed++
			case slurm.StateAlloc:
				s.NodeCounts.Alloc++
			case slurm.StateDown:
				s.NodeCounts.Down++
			case slurm.StateDrain:
				s.NodeCounts.Drain++
			case slurm.StateReserved:
				s.NodeCounts.Reserved++
			default:
				s.NodeCounts.Other++
			}
			s.TotalCPUs += n.CPUs
			s.AllocCPUs += n.AllocCPUs
			s.IdleCPUs += n.IdleCPUs
			s.TotalMemMB += n.RealMemoryMB
			s.AllocMemMB += n.AllocMemoryMB
			s.FreeMemMB += n.FreeMemoryMB
			s.TotalGPUs += nodeGPUs
			s.AllocGPUs += usedGPUs
			if s.GPUModel == "" && model != "" {
				s.GPUModel = model
			}
		}
	}

	for _, j := range jobs {
		if partitionFilter != "" && j.Partition != partitionFilter {
			continue
		}
		s := get(j.Partition)
		switch j.State {
		case "RUNNING":
			s.RunningJobs++
		case "PENDING":
			s.PendingJobs++
		}
	}

	out := make([]PartitionSummary, 0, len(summaries))
	for _, s := range summaries {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
