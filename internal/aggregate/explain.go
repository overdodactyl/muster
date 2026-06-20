package aggregate

import (
	"fmt"
	"sort"
	"strings"

	"muster/internal/slurm"
)

type ExplainReport struct {
	Job          slurm.Job
	ReasonHuman  string
	RequiredCPU  int    // per-node
	RequiredMem  int    // per-node, MB
	RequiredGPU  int    // per-node
	GPUModel     string // requested gpu model if any
	NodeFits     []NodeFit
}

type NodeFit struct {
	Node     string
	State    string
	CPUFree  int
	MemFree  int
	GPUFree  int
	GPUTotal int
	GPUModel string
	CanFit   bool
	Blockers []string
}

// Explain computes a per-node fit report for the given job against the nodes
// in its partition. Used to answer "why isn't this running?" — surfaces the
// concrete blockers per candidate node.
func Explain(job slurm.Job, nodes []slurm.Node) ExplainReport {
	r := ExplainReport{
		Job:         job,
		ReasonHuman: slurm.ExplainReason(job.Reason),
	}

	// Per-node requirements.
	r.RequiredCPU = job.CPUs
	if job.NodeCount > 1 {
		r.RequiredCPU = job.CPUs / job.NodeCount
	}
	switch {
	case job.MemPerNode > 0:
		r.RequiredMem = job.MemPerNode
	case job.MemPerCPU > 0:
		r.RequiredMem = job.MemPerCPU * r.RequiredCPU
	}
	r.RequiredGPU, r.GPUModel = parseGPURequirement(job.GRESPerNode)

	// Walk candidate nodes (those in this job's partition).
	for _, n := range nodes {
		inPart := false
		for _, p := range n.Partitions {
			if p == job.Partition {
				inPart = true
				break
			}
		}
		if !inPart {
			continue
		}

		fit := NodeFit{
			Node:    n.Name,
			State:   slurm.Classify(n.State).String(),
			CPUFree: n.IdleCPUs,
			MemFree: n.RealMemoryMB - n.AllocMemoryMB,
		}
		gres := slurm.ParseGRES(n.GRESTotal)
		gresUsed := slurm.ParseGRES(n.GRESUsed)
		fit.GPUTotal = slurm.GPUCount(gres)
		fit.GPUFree = fit.GPUTotal - slurm.GPUCount(gresUsed)
		fit.GPUModel = slurm.GPUModel(gres)

		class := slurm.Classify(n.State)
		switch class {
		case slurm.StateDown:
			fit.Blockers = append(fit.Blockers, "node is DOWN")
		case slurm.StateDrain:
			fit.Blockers = append(fit.Blockers, "node is DRAIN/DRAINED")
		case slurm.StateReserved:
			fit.Blockers = append(fit.Blockers, "node is RESERVED")
		}
		if fit.CPUFree < r.RequiredCPU {
			fit.Blockers = append(fit.Blockers,
				fmt.Sprintf("only %d free CPUs (need %d)", fit.CPUFree, r.RequiredCPU))
		}
		if r.RequiredMem > 0 && fit.MemFree < r.RequiredMem {
			fit.Blockers = append(fit.Blockers,
				fmt.Sprintf("only %d MB free mem (need %d MB)", fit.MemFree, r.RequiredMem))
		}
		if r.RequiredGPU > 0 {
			if fit.GPUTotal == 0 {
				fit.Blockers = append(fit.Blockers, "no GPUs on this node")
			} else if fit.GPUFree < r.RequiredGPU {
				fit.Blockers = append(fit.Blockers,
					fmt.Sprintf("only %d free GPUs (need %d)", fit.GPUFree, r.RequiredGPU))
			} else if r.GPUModel != "" && fit.GPUModel != r.GPUModel {
				fit.Blockers = append(fit.Blockers,
					fmt.Sprintf("GPU model mismatch: node has %s, job wants %s", fit.GPUModel, r.GPUModel))
			}
		}
		fit.CanFit = len(fit.Blockers) == 0
		r.NodeFits = append(r.NodeFits, fit)
	}

	// Sort: fitting nodes first, then by node name.
	sort.SliceStable(r.NodeFits, func(i, j int) bool {
		if r.NodeFits[i].CanFit != r.NodeFits[j].CanFit {
			return r.NodeFits[i].CanFit
		}
		return r.NodeFits[i].Node < r.NodeFits[j].Node
	})
	return r
}

// parseGPURequirement reads tres_per_node-style strings ("gres/gpu:1",
// "gpu:a100:2") and returns (count, model). Returns 0 if no GPU is required.
func parseGPURequirement(s string) (count int, model string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, ""
	}
	for _, chunk := range strings.Split(s, ",") {
		chunk = strings.TrimSpace(chunk)
		chunk = strings.TrimPrefix(chunk, "gres/")
		gres := slurm.ParseGRES(chunk)
		for _, g := range gres {
			if g.Kind == "gpu" {
				return g.Count, g.Model
			}
		}
	}
	return 0, ""
}
