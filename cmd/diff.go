package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"muster/internal/render"
	"muster/internal/snapshot"
)

var diffCmd = &cobra.Command{
	Use:   "diff <old.json> [new.json]",
	Short: "Compare a previously-captured snapshot to the current state (or to another snapshot)",
	Long: `Loads a snapshot file written by 'muster snapshot' and reports what changed.

If only one path is given, the current live cluster state is compared against it.
If two paths are given, both files are loaded and compared.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		old, err := snapshot.Read(args[0])
		if err != nil {
			return err
		}

		var newer snapshot.File
		if len(args) == 2 {
			newer, err = snapshot.Read(args[1])
			if err != nil {
				return err
			}
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			client, err := newClient()
			if err != nil {
				return err
			}
			n, err := client.Nodes(ctx)
			if err != nil {
				return err
			}
			j, err := client.Jobs(ctx, "")
			if err != nil {
				return err
			}
			r, err := client.Reservations(ctx)
			if err != nil {
				return err
			}
			newer = snapshot.File{CapturedAt: time.Now(), Nodes: n, Jobs: j, Reservations: r}
		}

		d := snapshot.Compute(old, newer)
		if flagJSON {
			return render.JSON(os.Stdout, d)
		}
		printDiff(d)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(diffCmd)
}

func printDiff(d snapshot.Diff) {
	gap := d.NewAt.Sub(d.OldAt)
	fmt.Printf("%s comparing %s → %s (%s elapsed)\n\n",
		render.ColorCyan("muster diff"),
		d.OldAt.Format(time.RFC3339),
		d.NewAt.Format(time.RFC3339),
		render.HumanDuration(gap),
	)

	if len(d.JobsAdded) > 0 {
		fmt.Printf("%s  (%d)\n", render.ColorGreen("Jobs added"), len(d.JobsAdded))
		for _, j := range d.JobsAdded {
			fmt.Printf("  %d  %s  %s  [%s] %s\n",
				j.ID, padJobUser(j.User), padJobState(j.State), j.Partition, truncateName(j.Name, 28))
		}
		fmt.Println()
	}

	if len(d.JobsRemoved) > 0 {
		fmt.Printf("%s  (%d)\n", render.ColorRed("Jobs no longer present"), len(d.JobsRemoved))
		for _, j := range d.JobsRemoved {
			fmt.Printf("  %d  %s  was %s  [%s] %s\n",
				j.ID, padJobUser(j.User), j.State, j.Partition, truncateName(j.Name, 28))
		}
		fmt.Println()
	}

	if len(d.JobsChanged) > 0 {
		fmt.Printf("%s  (%d)\n", render.ColorYellow("Jobs with state change"), len(d.JobsChanged))
		for _, c := range d.JobsChanged {
			fmt.Printf("  %d  %s  %s → %s   %s\n",
				c.Job.ID, padJobUser(c.Job.User), c.OldState, c.NewState, truncateName(c.Job.Name, 28))
		}
		fmt.Println()
	}

	if len(d.NodesChanged) > 0 {
		fmt.Printf("%s  (%d)\n", render.ColorYellow("Node state changes"), len(d.NodesChanged))
		for _, c := range d.NodesChanged {
			fmt.Printf("  %s  %s → %s\n",
				c.Name, strings.Join(c.OldState, ","), strings.Join(c.NewState, ","))
		}
		fmt.Println()
	}

	if len(d.ReservationsAdded) > 0 || len(d.ReservationsRemoved) > 0 {
		if len(d.ReservationsAdded) > 0 {
			fmt.Printf("%s  (%d)\n", render.ColorGreen("Reservations added"), len(d.ReservationsAdded))
			for _, r := range d.ReservationsAdded {
				fmt.Printf("  %s  users=%s  nodes=%s\n", r.Name, r.Users, r.Nodes)
			}
		}
		if len(d.ReservationsRemoved) > 0 {
			fmt.Printf("%s  (%d)\n", render.ColorRed("Reservations removed"), len(d.ReservationsRemoved))
			for _, r := range d.ReservationsRemoved {
				fmt.Printf("  %s  users=%s  nodes=%s\n", r.Name, r.Users, r.Nodes)
			}
		}
		fmt.Println()
	}

	if len(d.JobsAdded)+len(d.JobsRemoved)+len(d.JobsChanged)+len(d.NodesChanged)+len(d.ReservationsAdded)+len(d.ReservationsRemoved) == 0 {
		fmt.Println(render.ColorFaint("(no changes)"))
	}
}

func padJobUser(s string) string {
	if len(s) >= 10 {
		return s
	}
	return s + strings.Repeat(" ", 10-len(s))
}

func padJobState(s string) string {
	if len(s) >= 8 {
		return s
	}
	return s + strings.Repeat(" ", 8-len(s))
}

func truncateName(s string, n int) string {
	if len(s) <= n {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

