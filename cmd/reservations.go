package cmd

import (
	"context"
	"os"
	"time"

	"github.com/spf13/cobra"

	"muster/internal/aggregate"
	"muster/internal/render"
)

var reservationsCmd = &cobra.Command{
	Use:     "reservations",
	Aliases: []string{"resv", "reserve"},
	Short:   "List active and upcoming Slurm reservations",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		client, err := newClient()
		if err != nil {
			return err
		}
		res, err := client.Reservations(ctx)
		if err != nil {
			return err
		}
		rows := aggregate.Reservations(res, time.Now())
		if flagPartition != "" {
			filtered := rows[:0]
			for _, r := range rows {
				if r.Partition == "" || r.Partition == flagPartition {
					filtered = append(filtered, r)
				}
			}
			rows = filtered
		}
		if flagJSON {
			return render.JSON(os.Stdout, rows)
		}
		render.RenderReservations(os.Stdout, rows)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(reservationsCmd)
}
