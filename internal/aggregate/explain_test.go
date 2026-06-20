package aggregate

import "testing"

func TestExplain_PendingJobFitCheck(t *testing.T) {
	jobs := sampleJobs()
	// Find the Resources-blocked job (1003).
	var j *struct{}
	_ = j
	var target = jobs[2] // 1003: 16 CPUs, 32G mem
	report := Explain(target, sampleNodes())

	if report.RequiredCPU != 16 {
		t.Errorf("required cpus = %d, want 16", report.RequiredCPU)
	}
	if len(report.NodeFits) != 2 {
		t.Fatalf("gpu has 2 nodes; expected 2 fit entries, got %d", len(report.NodeFits))
	}
	// n01 has 12 idle CPUs (need 16) → can't fit
	// n02 has 32 idle CPUs (plenty) → fits
	for _, f := range report.NodeFits {
		switch f.Node {
		case "n01":
			if f.CanFit {
				t.Errorf("n01 has 12 free CPUs, job needs 16 — should not fit")
			}
		case "n02":
			if !f.CanFit {
				t.Errorf("n02 has 32 free CPUs and 60G mem — should fit; blockers=%v", f.Blockers)
			}
		}
	}
}

func TestExplain_GPURequirement(t *testing.T) {
	jobs := sampleJobs()
	// Synthesize a GPU-requiring pending job that needs an a100 (gpu nodes have it).
	target := jobs[2]
	target.GRESPerNode = "gres/gpu:1"
	report := Explain(target, sampleNodes())
	if report.RequiredGPU != 1 {
		t.Errorf("required gpus = %d, want 1", report.RequiredGPU)
	}
	// n01 has 1 free GPU (4 total - 3 used) → fits in GPU terms
	// n02 has 0 GPUs → blocked
	for _, f := range report.NodeFits {
		if f.Node == "n02" {
			foundBlock := false
			for _, b := range f.Blockers {
				if b == "no GPUs on this node" {
					foundBlock = true
				}
			}
			if !foundBlock {
				t.Errorf("n02 should report 'no GPUs on this node'; blockers=%v", f.Blockers)
			}
		}
	}
}
