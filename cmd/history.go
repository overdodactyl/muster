package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"muster/internal/aggregate"
	"muster/internal/render"
)

var (
	histSince string
	histBy    string
	histState string
)

var historyCmd = &cobra.Command{
	Use:     "history",
	Aliases: []string{"h"},
	Short:   "Recent sacct rollup of CPU-hours, GPU-hours, and job outcomes",
	Long: `Aggregate finished jobs from Slurm accounting (sacct) over a time window.
Helpful for 'who used what this week' and 'how many jobs failed'.

Slurm permission note: non-admin users see only their own jobs.
Long windows are slow - default is 24h.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dur, err := parseSince(histSince)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		client, err := newClient()
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "fetching accounting history...")
		jobs, err := client.Accounting(ctx, dur, flagPartition)
		if err != nil {
			return err
		}
		var states []string
		if histState != "" {
			states = strings.Split(histState, ",")
		}
		rows := aggregate.History(jobs, histBy, flagPartition, states)
		if flagJSON {
			return render.JSON(os.Stdout, rows)
		}
		render.RenderHistory(os.Stdout, rows, histBy)
		return nil
	},
}

// parseSince accepts "24h", "30m", "7d", "2h30m" etc. The "d" suffix isn't
// supported by time.ParseDuration so we translate it to hours.
func parseSince(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 24 * time.Hour, nil
	}
	if strings.HasSuffix(s, "d") {
		var days int
		_, err := fmt.Sscanf(s, "%dd", &days)
		if err != nil {
			return 0, fmt.Errorf("bad --since value %q: %w", s, err)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("bad --since value %q: use 30m / 24h / 7d", s)
	}
	return d, nil
}

func init() {
	historyCmd.Flags().StringVar(&histSince, "since", "24h", "time window: 30m, 4h, 7d, etc.")
	historyCmd.Flags().StringVar(&histBy, "by", "user", "rollup key: user|account|state|partition")
	historyCmd.Flags().StringVar(&histState, "state", "", "filter to specific final states (comma-separated, e.g. FAILED,TIMEOUT)")
	rootCmd.AddCommand(historyCmd)
}
