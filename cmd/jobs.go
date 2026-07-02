package cmd

import (
	"context"
	"os"
	"time"

	"github.com/spf13/cobra"

	"muster/internal/aggregate"
	"muster/internal/render"
)

var (
	jobsSort string
	jobsTop  int
	jobsAll  bool
)

var jobsCmd = &cobra.Command{
	Use:     "jobs",
	Aliases: []string{"j", "top"},
	Short:   "Running jobs sorted by resource use - the 'what's eating resources' view",
	Long: `List individual jobs (running by default) sorted by resource use, so you
can quickly see which jobs are holding the most CPUs, GPUs, or memory.

Each row shows the job name, owner, partition, nodes, CPU/GPU/MEM allocation,
and how long it's been running. Use --all to include pending jobs.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		client, err := newClient()
		if err != nil {
			return err
		}
		jobs, err := client.Jobs(ctx, flagPartition)
		if err != nil {
			return err
		}
		rows := aggregate.Jobs(jobs, flagPartition, flagUser, jobsAll, jobsSort, jobsTop, time.Now())
		if flagJSON {
			return render.JSON(os.Stdout, rows)
		}
		lanids := make([]string, len(rows))
		for i, r := range rows {
			lanids[i] = r.User
		}
		render.PrewarmNames(lanids)
		render.RenderJobs(os.Stdout, rows)
		return nil
	},
}

func init() {
	jobsCmd.Flags().StringVar(&jobsSort, "sort", "cpus", "sort by: cpus|gpus|mem|runtime|user")
	jobsCmd.Flags().IntVar(&jobsTop, "top", 0, "show only the top N jobs (0 = all)")
	jobsCmd.Flags().BoolVar(&jobsAll, "all", false, "include pending jobs in addition to running")
	rootCmd.AddCommand(jobsCmd)
}
