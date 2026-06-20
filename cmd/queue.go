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
	queueAll          bool
	queueReasonFilter string
	queueSort         string
)

var queueCmd = &cobra.Command{
	Use:     "queue",
	Aliases: []string{"q"},
	Short:   "Pending-job queue with reason codes explained in plain English",
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
		if flagUser != "" {
			filtered := jobs[:0]
			for _, j := range jobs {
				if j.User == flagUser {
					filtered = append(filtered, j)
				}
			}
			jobs = filtered
		}
		rows := aggregate.Queue(jobs, flagPartition, queueAll, queueReasonFilter, queueSort, time.Now())
		if flagJSON {
			return render.JSON(os.Stdout, rows)
		}
		render.RenderQueue(os.Stdout, rows)
		return nil
	},
}

func init() {
	queueCmd.Flags().BoolVar(&queueAll, "all", false, "include running jobs in addition to pending")
	queueCmd.Flags().StringVar(&queueReasonFilter, "reason", "", "filter by reason code substring (e.g. BeginTime, Resources)")
	queueCmd.Flags().StringVar(&queueSort, "sort", "priority", "sort by: priority|age|user")
	rootCmd.AddCommand(queueCmd)
}
