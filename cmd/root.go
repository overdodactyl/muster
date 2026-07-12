package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"muster/internal/render"
)

var (
	flagPartition string
	flagJSON      bool
	flagNoColor   bool
	flagUser      string
	flagCluster   string
	flagTheme     string
	flagNames     bool
	flagFixtures  string
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
		switch flagTheme {
		case "light":
			render.SetTheme(render.ThemeLight)
		case "dark", "":
			render.SetTheme(render.ThemeDark)
		case "auto":
			// COLORFGBG is set by some terminals (rxvt, others) as "fg;bg".
			// Values >=15 in the bg slot generally mean a light background.
			fg, bg := parseColorFGBG(os.Getenv("COLORFGBG"))
			_ = fg
			if bg >= 10 {
				render.SetTheme(render.ThemeLight)
			} else {
				render.SetTheme(render.ThemeDark)
			}
		}
		render.SetNames(flagNames)
		// Cap table width to the terminal so narrow shells don't break the
		// layout. Skip when output isn't a TTY (piping); go-pretty's default
		// is fine there.
		if fd := int(os.Stdout.Fd()); term.IsTerminal(fd) {
			if w, _, err := term.GetSize(fd); err == nil && w > 0 {
				render.SetMaxWidth(w)
			}
		}
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&flagPartition, "partition", "p", "", "filter to a single partition (default: all)")
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "emit JSON instead of a table")
	rootCmd.PersistentFlags().BoolVar(&flagNoColor, "no-color", false, "disable colored output")
	rootCmd.PersistentFlags().StringVarP(&flagUser, "user", "u", "", "filter to a single user (where applicable)")
	rootCmd.PersistentFlags().StringVar(&flagCluster, "cluster", "", "Slurm cluster name (passed to -M); v1 supports a single cluster")
	rootCmd.PersistentFlags().StringVar(&flagTheme, "theme", "dark", "color theme: dark | light | auto (auto reads $COLORFGBG)")
	rootCmd.PersistentFlags().BoolVar(&flagNames, "names", false, "resolve lanids to real names via getent (dash: toggle with 'n')")
	rootCmd.PersistentFlags().StringVar(&flagFixtures, "fixtures", "", "read Slurm --json from this directory instead of shelling out (env: MUSTER_FIXTURES) — useful for demos and screenshots")
}

// parseColorFGBG splits the rxvt-style "fg;bg" env value into ints. Returns
// (0,0) for unparseable values.
func parseColorFGBG(s string) (int, int) {
	if s == "" {
		return 0, 0
	}
	var fg, bg int
	parts := strings.Split(s, ";")
	if len(parts) < 2 {
		return 0, 0
	}
	fmt.Sscanf(parts[0], "%d", &fg)
	fmt.Sscanf(parts[len(parts)-1], "%d", &bg)
	return fg, bg
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		// Cobra already printed "Error: ..." to stderr; just propagate the
		// exit code.
		os.Exit(1)
	}
}
