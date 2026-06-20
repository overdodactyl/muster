package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"muster/internal/snapshot"
)

var snapshotCmd = &cobra.Command{
	Use:   "snapshot [path]",
	Short: "Capture current cluster state (nodes, jobs, reservations) to a JSON file",
	Long: `Writes a JSON snapshot of the live cluster state. Use 'muster diff <file>'
later to see what changed between then and now.

Default filename when [path] omitted: muster-snapshot-YYYYMMDD-HHMMSS.json
in the current directory.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := ""
		if len(args) > 0 {
			path = args[0]
		}
		if path == "" {
			path = fmt.Sprintf("muster-snapshot-%s.json", time.Now().Format("20060102-150405"))
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		client, err := newClient()
		if err != nil {
			return err
		}

		nodes, err := client.Nodes(ctx)
		if err != nil {
			return err
		}
		jobs, err := client.Jobs(ctx, "")
		if err != nil {
			return err
		}
		res, err := client.Reservations(ctx)
		if err != nil {
			return err
		}

		f := snapshot.File{
			CapturedAt:   time.Now(),
			Nodes:        nodes,
			Jobs:         jobs,
			Reservations: res,
		}
		if err := snapshot.Write(path, f); err != nil {
			return err
		}
		fmt.Printf("wrote %s: %d nodes, %d jobs, %d reservations\n",
			path, len(nodes), len(jobs), len(res))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(snapshotCmd)
}
