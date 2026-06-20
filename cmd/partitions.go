package cmd

import (
	"context"
	"os"
	"time"

	"github.com/spf13/cobra"

	"muster/internal/aggregate"
	"muster/internal/render"
)

var partitionsCmd = &cobra.Command{
	Use:     "partitions",
	Aliases: []string{"parts", "p"},
	Short:   "Per-partition rollup of nodes, CPUs, GPUs, memory, and job counts",
	Long: `Show a one-row-per-partition summary: node states (idle/mixed/alloc/down),
total vs allocated CPUs and GPUs, memory in use, and counts of running and pending jobs.`,
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
		rows := aggregate.Partitions(nodes, jobs, flagPartition)
		if flagJSON {
			return render.JSON(os.Stdout, rows)
		}
		render.RenderPartitions(os.Stdout, rows)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(partitionsCmd)
}
