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

var usageSince string

var usageCmd = &cobra.Command{
	Use:   "usage",
	Short: "Efficiency report - CPU-hours requested vs CPU-hours actually used",
	Long: `Walks completed sacct jobs over the given window and computes per-user
efficiency: CPU-hours requested (alloc_cpu × elapsed) vs CPU-hours actually
used (sum of user+system CPU time across all cores).

Low efficiency means cores were sitting idle - common pattern is asking
for 32 cores but only using 1. Worst-offender column flags the worst job
per user. Excludes jobs shorter than 5 minutes (noisy startup failures).

sacct visibility: non-admin users only see their own jobs.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dur, err := parseUsageSince(usageSince)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
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
		rows := aggregate.Usage(jobs, flagPartition)
		if flagJSON {
			return render.JSON(os.Stdout, rows)
		}
		lanids := make([]string, len(rows))
		for i, r := range rows {
			lanids[i] = r.User
		}
		render.PrewarmNames(lanids)
		render.RenderUsage(os.Stdout, rows)
		return nil
	},
}

func parseUsageSince(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 7 * 24 * time.Hour, nil
	}
	if strings.HasSuffix(s, "d") {
		var days int
		_, err := fmt.Sscanf(s, "%dd", &days)
		if err != nil {
			return 0, fmt.Errorf("bad --since %q", s)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("bad --since %q: use 30m / 24h / 7d", s)
	}
	return d, nil
}

func init() {
	usageCmd.Flags().StringVar(&usageSince, "since", "7d", "time window: 30m, 24h, 7d, etc.")
	rootCmd.AddCommand(usageCmd)
}
