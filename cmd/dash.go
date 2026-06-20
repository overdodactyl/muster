package cmd

import (
	"github.com/spf13/cobra"

	"muster/internal/tui"
)

var dashCmd = &cobra.Command{
	Use:     "dash",
	Aliases: []string{"d", "watch"},
	Short:   "Interactive auto-refreshing dashboard (TUI)",
	Long: `Open a tabbed full-screen dashboard with live-refreshing views of
partitions, nodes, users, queue, and recent history. Auto-refreshes every
10 seconds.

Keys: q/esc quit · r refresh · tab/shift-tab next/prev · 1-5 jump to tab.

Requires a TTY; pipe-friendly output uses the static subcommands.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		return tui.Run(client, flagPartition)
	},
}

func init() {
	rootCmd.AddCommand(dashCmd)
}
