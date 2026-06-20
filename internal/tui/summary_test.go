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

	m := &model{
		client:    c,
		partition: "gpu",
		nodes:     nodes,
		jobs:      jobs,
		width:     120,
		height:    40,
	}
	fmt.Println(m.renderSummary(120))
}
