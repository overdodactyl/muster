package cmd

import (
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	flagPartition string
	flagJSON      bool
	flagNoColor   bool
	flagUser      string
	flagCluster   string
)

var rootCmd = &cobra.Command{
	Use:   "muster",
	Short: "Readable views over Slurm partitions, nodes, users, and queue",
	Long: `muster wraps sinfo/squeue/sacct/scontrol --json and presents
human-readable rollups: per-partition resource summaries, per-node detail
with who's running on each node, per-user resource holdings, pending-queue
insight with reason codes explained, and recent sacct history.`,
	SilenceUsage: true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if flagNoColor || flagJSON {
			color.NoColor = true
		}
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&flagPartition, "partition", "p", "", "filter to a single partition (default: all)")
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "emit JSON instead of a table")
	rootCmd.PersistentFlags().BoolVar(&flagNoColor, "no-color", false, "disable colored output")
	rootCmd.PersistentFlags().StringVarP(&flagUser, "user", "u", "", "filter to a single user (where applicable)")
	rootCmd.PersistentFlags().StringVar(&flagCluster, "cluster", "", "Slurm cluster name (passed to -M); v1 supports a single cluster")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		// Cobra already printed "Error: ..." to stderr; just propagate the
		// exit code.
		os.Exit(1)
	}
}
