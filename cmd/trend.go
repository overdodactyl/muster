package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"muster/internal/render"
	"muster/internal/store"
)

var (
	trendSince string
	trendPath  string
	trendWidth int
)

var trendCmd = &cobra.Command{
	Use:   "trend",
	Short: "Plot partition utilization sparklines from the recorder's history file",
	Long: `Reads ~/.cache/muster/history.jsonl (or --path / $MUSTER_HISTORY) and
draws per-partition CPU/GPU/memory utilization sparklines over the
given window. Compared to the dash sparklines (5 minutes in-memory),
this reaches back as far as the recorder has run.

The recorder must be running (or have been running): see 'muster recorder'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dur, err := store.ParseSince(trendSince)
		if err != nil {
			return err
		}
		path := trendPath
		if path == "" {
			path = store.DefaultPath()
		}
		since := time.Now().Add(-dur)
		samples, err := store.Read(path, since)
		if err != nil {
			return fmt.Errorf("read history: %w (is the recorder running?)", err)
		}
		if len(samples) == 0 {
			fmt.Fprintln(os.Stderr, "no samples in window — try `muster recorder` first")
			return nil
		}

		fmt.Printf("%s  window: last %s · %d samples\n",
			render.Bold("muster trend"), trendSince, len(samples))

		names := store.PartitionNames(samples)
		if flagPartition != "" {
			names = []string{flagPartition}
		}
		for _, name := range names {
			series := store.PartitionSeries(samples, name)
			if len(series.CPU) == 0 {
				continue
			}
			cpu := store.Downsample(series.CPU, series.Times, trendWidth)
			gpu := store.Downsample(series.GPU, series.Times, trendWidth)
			mem := store.Downsample(series.Mem, series.Times, trendWidth)

			cpuMin, cpuMax, cpuAvg := store.SampleStats(cpu)
			gpuMin, gpuMax, gpuAvg := store.SampleStats(gpu)
			memMin, memMax, memAvg := store.SampleStats(mem)

			fmt.Println()
			fmt.Println(render.Bold(name))
			fmt.Printf("  CPU%%  %s   %s\n",
				fmt.Sprintf("min %2d  avg %2d  max %3d", cpuMin, cpuAvg, cpuMax),
				render.Sparkline(cpu, trendWidth))
			fmt.Printf("  GPU%%  %s   %s\n",
				fmt.Sprintf("min %2d  avg %2d  max %3d", gpuMin, gpuAvg, gpuMax),
				render.Sparkline(gpu, trendWidth))
			fmt.Printf("  MEM%%  %s   %s\n",
				fmt.Sprintf("min %2d  avg %2d  max %3d", memMin, memAvg, memMax),
				render.Sparkline(mem, trendWidth))
		}
		return nil
	},
}

func init() {
	trendCmd.Flags().StringVar(&trendSince, "since", "7d", "time window: 1h, 24h, 7d, ...")
	trendCmd.Flags().StringVar(&trendPath, "path", "", "history file (default: ~/.cache/muster/history.jsonl)")
	trendCmd.Flags().IntVar(&trendWidth, "width", 60, "sparkline width in chars")
	rootCmd.AddCommand(trendCmd)
}
