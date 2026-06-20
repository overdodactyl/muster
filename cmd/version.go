package cmd

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/spf13/cobra"
)

var (
	buildVersion = "dev"
)

func SetVersion(v string) { buildVersion = v }

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print muster version and detected Slurm version",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("muster %s (%s/%s, %s)\n", buildVersion, runtime.GOOS, runtime.GOARCH, runtime.Version())

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		client, err := newClient()
		if err == nil {
			if sv, err := client.SlurmVersion(ctx); err == nil {
				fmt.Printf("slurm  %s\n", sv)
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
