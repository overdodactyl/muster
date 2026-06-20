package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"muster/internal/aggregate"
	"muster/internal/store"
)

var (
	recorderInterval time.Duration
	recorderPath     string
	recorderQuiet    bool
)

var recorderCmd = &cobra.Command{
	Use:   "recorder",
	Short: "Record cluster utilization samples to a JSONL file (for muster trend)",
	Long: `Long-running daemon that captures per-partition CPU/GPU/mem allocation
every --interval (default 60s) and appends a JSON sample line to
--path (default ~/.cache/muster/history.jsonl).

The 'muster trend' command reads this file to draw sparklines over
days/weeks rather than the in-dash 5-minute ring buffer.

Run in the background like the exporter:

  nohup muster recorder --quiet > /tmp/muster-recorder.log 2>&1 &
  systemctl --user start muster-recorder    # if you write a unit
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		path := recorderPath
		if path == "" {
			path = store.DefaultPath()
		}
		if !recorderQuiet {
			fmt.Fprintf(os.Stderr, "muster recorder writing to %s (interval=%s)\n", path, recorderInterval)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigs
			cancel()
		}()

		// Try to capture cluster name once for the record header.
		cluster := ""
		if cctx, ccancel := context.WithTimeout(ctx, 5*time.Second); true {
			if c, err := client.ClusterName(cctx); err == nil {
				cluster = c
			}
			ccancel()
		}

		ticker := time.NewTicker(recorderInterval)
		defer ticker.Stop()

		capture := func() error {
			cctx, ccancel := context.WithTimeout(ctx, 30*time.Second)
			defer ccancel()
			nodes, err := client.Nodes(cctx)
			if err != nil {
				return err
			}
			jobs, err := client.Jobs(cctx, "")
			if err != nil {
				return err
			}
			parts := aggregate.Partitions(nodes, jobs, "")
			s := store.Sample{
				At:      time.Now(),
				Cluster: cluster,
			}
			for _, p := range parts {
				s.Partitions = append(s.Partitions, store.PartitionSnap{
					Name:        p.Name,
					AllocCPUs:   p.AllocCPUs,
					TotalCPUs:   p.TotalCPUs,
					AllocGPUs:   p.AllocGPUs,
					TotalGPUs:   p.TotalGPUs,
					AllocMemMB:  p.AllocMemMB,
					TotalMemMB:  p.TotalMemMB,
					RunningJobs: p.RunningJobs,
					PendingJobs: p.PendingJobs,
				})
			}
			return store.Append(path, s)
		}
		if err := capture(); err != nil {
			fmt.Fprintln(os.Stderr, "recorder:", err)
		}

		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				if err := capture(); err != nil {
					fmt.Fprintln(os.Stderr, "recorder:", err)
				}
			}
		}
	},
}

func init() {
	recorderCmd.Flags().DurationVar(&recorderInterval, "interval", 60*time.Second, "how often to capture a sample")
	recorderCmd.Flags().StringVar(&recorderPath, "path", "", "history file path (default: ~/.cache/muster/history.jsonl, or $MUSTER_HISTORY)")
	recorderCmd.Flags().BoolVar(&recorderQuiet, "quiet", false, "suppress startup message")
	rootCmd.AddCommand(recorderCmd)
}
