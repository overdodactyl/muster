package cmd

import (
	"context"
	"os"
	"time"

	"github.com/spf13/cobra"

	"muster/internal/aggregate"
	"muster/internal/render"
)

var gpuCmd = &cobra.Command{
	Use:     "gpu",
	Aliases: []string{"gpus"},
	Short:   "Per-GPU allocation: which GPU index is held by which user/job",
	Long: `Walks every GPU-bearing node in scope and shows one row per physical GPU
index. Idle GPUs are flagged green; held GPUs show the user, job ID,
job name, and runtime.

The mapping comes from squeue's per-job gres_detail field (e.g.
'gpu:a100:1(IDX:2)'), so it reflects what Slurm actually scheduled.`,
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
		rows := aggregate.GPUs(nodes, jobs, flagPartition, time.Now())
		if flagJSON {
			return render.JSON(os.Stdout, rows)
		}
		lanids := make([]string, 0, len(rows))
		for _, r := range rows {
			if r.User != "" {
				lanids = append(lanids, r.User)
			}
		}
		render.PrewarmNames(lanids)
		render.RenderGPUs(os.Stdout, rows)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(gpuCmd)
}
