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
	usersSort string
	usersTop  int
)

var usersCmd = &cobra.Command{
	Use:     "users",
	Aliases: []string{"u"},
	Short:   "Per-user resource holdings (CPUs, GPUs, memory, oldest running job)",
	Long: `Aggregate jobs by user and show resources held. Helpful for answering
'who is holding the partition right now' and 'why is the queue full?'.

Slurm permission note: as a non-admin you only see your own jobs in some setups;
in that case this view will only show yourself. Ask an admin for cluster-wide visibility.`,
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
		rows := aggregate.Users(jobs, flagPartition, flagUser, usersSort, usersTop, time.Now())
		if flagJSON {
			return render.JSON(os.Stdout, rows)
		}
		lanids := make([]string, len(rows))
		for i, r := range rows {
			lanids[i] = r.User
		}
		render.PrewarmNames(lanids)
		render.RenderUsers(os.Stdout, rows)
		return nil
	},
}

func init() {
	usersCmd.Flags().StringVar(&usersSort, "sort", "cpus", "sort by: cpus|gpus|mem|jobs|age")
	usersCmd.Flags().IntVar(&usersTop, "top", 0, "show only the top N users (0 = all)")
	rootCmd.AddCommand(usersCmd)
}
