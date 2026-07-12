package slurm

import (
	"context"
	"testing"
)

func TestFixtureClientReadsTestdata(t *testing.T) {
	c := NewFixtureClient("../../testdata")
	ctx := context.Background()

	nodes, err := c.Nodes(ctx)
	if err != nil {
		t.Fatalf("Nodes: %v", err)
	}
	if len(nodes) == 0 {
		t.Fatal("expected nodes from testdata/scontrol_nodes.json, got none")
	}

	parts, err := c.Partitions(ctx)
	if err != nil {
		t.Fatalf("Partitions: %v", err)
	}
	if len(parts) == 0 {
		t.Fatal("expected partitions from testdata/scontrol_partitions.json, got none")
	}

	all, err := c.Jobs(ctx, "")
	if err != nil {
		t.Fatalf("Jobs(all): %v", err)
	}
	if len(all) == 0 {
		t.Fatal("expected jobs from testdata/squeue.json, got none")
	}
	filtered, err := c.Jobs(ctx, "gpu")
	if err != nil {
		t.Fatalf("Jobs(gpu): %v", err)
	}
	if len(filtered) >= len(all) {
		t.Fatalf("partition filter did not narrow results: all=%d, filtered=%d", len(all), len(filtered))
	}
	for _, j := range filtered {
		if j.Partition != "gpu" {
			t.Errorf("job %d has partition %q, want gpu", j.ID, j.Partition)
		}
	}
}

func TestFixtureClientMissingFiles(t *testing.T) {
	c := NewFixtureClient(t.TempDir())
	ctx := context.Background()

	if n, err := c.Nodes(ctx); err != nil || n != nil {
		t.Errorf("Nodes on empty dir: (%v, %v), want (nil, nil)", n, err)
	}
	if p, err := c.Partitions(ctx); err != nil || p != nil {
		t.Errorf("Partitions on empty dir: (%v, %v), want (nil, nil)", p, err)
	}
	if j, err := c.Jobs(ctx, ""); err != nil || j != nil {
		t.Errorf("Jobs on empty dir: (%v, %v), want (nil, nil)", j, err)
	}
	if a, err := c.Accounting(ctx, 0, ""); err != nil || a != nil {
		t.Errorf("Accounting on empty dir: (%v, %v), want (nil, nil)", a, err)
	}
	if r, err := c.Reservations(ctx); err != nil || r != nil {
		t.Errorf("Reservations on empty dir: (%v, %v), want (nil, nil)", r, err)
	}
	if err := c.Cancel(ctx, 1); err != nil {
		t.Errorf("Cancel on empty dir: %v, want nil", err)
	}
	if u, err := c.JobGPUUtil(ctx, 1); err != nil || u != nil {
		t.Errorf("JobGPUUtil on empty dir: (%v, %v), want (nil, nil)", u, err)
	}
	if name, err := c.ClusterName(ctx); err != nil || name != "demo-cluster" {
		t.Errorf("ClusterName on empty dir: (%q, %v), want (\"demo-cluster\", nil)", name, err)
	}
}

func TestFixtureClientJobDetailFromSqueue(t *testing.T) {
	c := NewFixtureClient("../../testdata")
	ctx := context.Background()
	jobs, err := c.Jobs(ctx, "")
	if err != nil || len(jobs) == 0 {
		t.Fatalf("Jobs: %v (len=%d)", err, len(jobs))
	}
	want := jobs[0]
	d, err := c.JobDetail(ctx, want.ID)
	if err != nil {
		t.Fatalf("JobDetail(%d): %v", want.ID, err)
	}
	if d.ID != want.ID {
		t.Errorf("JobDetail returned ID %d, want %d", d.ID, want.ID)
	}
}
