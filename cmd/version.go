package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	buildVersion = "dev"
)

func SetVersion(v string) { buildVersion = v }

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print muster version",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("muster %s (%s/%s, %s)\n", buildVersion, runtime.GOOS, runtime.GOARCH, runtime.Version())
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
