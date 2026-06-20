package slurm

import "testing"

func TestExplainReason(t *testing.T) {
	if got := ExplainReason("BeginTime"); got != "Holding until scheduled start time" {
		t.Errorf("BeginTime explanation wrong: %q", got)
	}
	if got := ExplainReason("SomeNewSlurmReason"); got != "SomeNewSlurmReason" {
		t.Errorf("unknown code should pass through, got %q", got)
	}
	if got := ExplainReason(""); got == "" {
		t.Errorf("empty reason should be explained, got empty string")
	}
}
