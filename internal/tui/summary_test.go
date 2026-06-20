package tui

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/fatih/color"

	"muster/internal/slurm"
)

// TestRenderSummary_Live is a manual smoke test: when MUSTER_TUI_LIVE=1
// is set, fetches real Slurm data and prints the rendered summary panel.
// Skipped by default so `make test` stays hermetic.
func TestRenderSummary_Live(t *testing.T) {
	if os.Getenv("MUSTER_TUI_LIVE") == "" {
		t.Skip("set MUSTER_TUI_LIVE=1 to run; this hits live Slurm and dumps the summary panel")
	}

	color.NoColor = false // force ANSI so we can eyeball the output

	c := slurm.NewCLIClient()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	nodes, err := c.Nodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := c.Jobs(ctx, "gpu")
	if err != nil {
		t.Fatal(err)
	}

	jobsAll, err := c.Jobs(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println("=== with -p gpu ===")
	mDIL := &model{client: c, partition: "gpu", nodes: nodes, jobs: jobs, width: 120, height: 40}
	mDIL.recordSample()
	fmt.Println(mDIL.renderSummary(120))
	fmt.Println()

	fmt.Println("=== without -p (cluster-mode, per-partition cards) ===")
	mAll := &model{client: c, partition: "", nodes: nodes, jobs: jobsAll, width: 160, height: 40}
	mAll.recordSample()
	fmt.Println(mAll.renderSummary(160))
	fmt.Println()

	m := &model{
		client:    c,
		partition: "gpu",
		nodes:     nodes,
		jobs:      jobs,
		width:     120,
		height:    40,
	}
	// Seed the history with a synthetic curve so the sparkline renders
	// something interesting instead of a single bar.
	curve := []int{12, 18, 25, 33, 41, 47, 55, 62, 60, 58, 52, 45, 40, 38, 42, 50, 60, 67, 70, 72, 68, 62, 55}
	for _, v := range curve {
		m.history = append(m.history, historySample{cpuPct: v, gpuPct: v * 80 / 100, memPct: v + 20})
	}
	m.recordSample() // append one real sample too
	fmt.Println(m.renderSummary(120))
}
