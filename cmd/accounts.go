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
	accountsSort string
	accountsTop  int
)

var accountsCmd = &cobra.Command{
	Use:     "accounts",
	Aliases: []string{"a", "labs"},
	Short:   "Per-account (lab) rollup: jobs and resources held by Slurm account",
	Long: `Aggregates jobs by Slurm account instead of by user. Useful when the
right question isn't 'who is on the cluster?' but 'which lab is using
its share?'

Each row shows distinct user count, running/pending jobs, CPUs/GPUs/mem
held, and the oldest running job's age.`,
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
		rows := aggregate.Accounts(jobs, flagPartition, accountsSort, accountsTop, time.Now())
		if flagJSON {
			return render.JSON(os.Stdout, rows)
		}
		render.RenderAccounts(os.Stdout, rows)
		return nil
	},
}

func init() {
	accountsCmd.Flags().StringVar(&accountsSort, "sort", "cpus", "sort by: cpus|gpus|mem|jobs|users|name")
	accountsCmd.Flags().IntVar(&accountsTop, "top", 0, "show only the top N accounts (0 = all)")
	rootCmd.AddCommand(accountsCmd)
}
