package cmd

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"muster/internal/aggregate"
	"muster/internal/render"
)

var (
	nodesStateFilter string
	nodesGPUOnly     bool
	nodesShowJobs    bool
)

var nodesCmd = &cobra.Command{
	Use:     "nodes",
	Aliases: []string{"n"},
	Short:   "Per-node detail: state, CPUs, memory, GPUs, and which users have jobs there",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		client, err := newClient()
		if err != nil {
			return err
		}
		nodes, err := client.Nodes(ctx)
		if err != nil {
			return err
		}
		jobs, err := client.Jobs(ctx, flagPartition)
		if err != nil {
			return err
		}
		var stateFilter []string
		if nodesStateFilter != "" {
			stateFilter = strings.Split(nodesStateFilter, ",")
		}
		rows := aggregate.Nodes(nodes, jobs, flagPartition, stateFilter, nodesGPUOnly, nodesShowJobs)
		if flagJSON {
			return render.JSON(os.Stdout, rows)
		}
		render.RenderNodes(os.Stdout, rows, nodesShowJobs)
		return nil
	},
}

func init() {
	nodesCmd.Flags().StringVar(&nodesStateFilter, "state", "", "comma-separated state filter (e.g. mixed,drain,down)")
	nodesCmd.Flags().BoolVar(&nodesGPUOnly, "gpu", false, "only show GPU-bearing nodes")
	nodesCmd.Flags().BoolVar(&nodesShowJobs, "show-jobs", false, "show user(jobid) pairs instead of unique users")
	rootCmd.AddCommand(nodesCmd)
}
