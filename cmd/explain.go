package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"muster/internal/aggregate"
	"muster/internal/render"
	"muster/internal/slurm"
)

var explainCmd = &cobra.Command{
	Use:   "explain <jobid>",
	Short: "Why isn't my job running? Per-node fit check against the partition",
	Long: `Looks up a pending job and reports what's blocking it on each candidate
node: not enough CPUs, no free GPU, GPU model mismatch, node drained, etc.

For BeginTime / Resources / Priority pending reasons, the eligible-start
timestamp (from start_time in squeue) is shown when populated.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		jobID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid jobid %q: %w", args[0], err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		client, err := newClient()
		if err != nil {
			return err
		}
		jobs, err := client.Jobs(ctx, "")
		if err != nil {
			return err
		}

		var found *slurm.Job
		for i := range jobs {
			if jobs[i].ID == jobID {
				found = &jobs[i]
				break
			}
		}
		if found == nil {
			return fmt.Errorf("job %d not found in squeue (may be completed; try sacct)", jobID)
		}

		nodes, err := client.Nodes(ctx)
		if err != nil {
			return err
		}

		report := aggregate.Explain(*found, nodes)
		if flagJSON {
			return render.JSON(os.Stdout, report)
		}
		printExplain(os.Stdout, report)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(explainCmd)
}

func printExplain(w *os.File, r aggregate.ExplainReport) {
	j := r.Job
	fmt.Fprintf(w, "%s %d (%s / %s)\n",
		render.Bold("Job"), j.ID, render.ColorCyan(j.User), j.Name)
	fmt.Fprintf(w, "  State:      %s\n", colorState(j.State))
	// Slurm sets state_reason to "None" for running jobs; only the pending
	// reason explainer is meaningful when the job is actually waiting.
	if j.State == "PENDING" && j.Reason != "" && j.Reason != "None" {
		fmt.Fprintf(w, "  Reason:     %s — %s\n", j.Reason, r.ReasonHuman)
	}
	fmt.Fprintf(w, "  Partition:  %s\n", j.Partition)
	if j.State == "RUNNING" && !j.StartTime.IsZero() {
		runtime := time.Since(j.StartTime)
		fmt.Fprintf(w, "  Nodes:      %s\n", j.Nodes)
		fmt.Fprintf(w, "  Started:    %s  (running %s)\n",
			j.StartTime.Format("2006-01-02 15:04"), render.HumanDuration(runtime))
		if j.TimeLimit > 0 {
			remaining := j.TimeLimit - runtime
			if remaining > 0 {
				fmt.Fprintf(w, "  Time left:  %s  (limit %s)\n",
					render.HumanDuration(remaining), render.HumanDuration(j.TimeLimit))
			} else {
				fmt.Fprintf(w, "  Time left:  %s  (over limit; scheduler will TIMEOUT)\n",
					render.ColorRed("0"))
			}
		}
	}

	reqs := []string{fmt.Sprintf("%d CPUs", j.CPUs)}
	if r.RequiredGPU > 0 {
		g := fmt.Sprintf("%d GPU", r.RequiredGPU)
		if r.GPUModel != "" {
			g = fmt.Sprintf("%d %s GPU", r.RequiredGPU, r.GPUModel)
		}
		reqs = append(reqs, g)
	}
	if r.RequiredMem > 0 {
		reqs = append(reqs, render.HumanMB(r.RequiredMem)+" mem/node")
	}
	if j.TimeLimit > 0 {
		reqs = append(reqs, render.HumanDuration(j.TimeLimit)+" time")
	}
	fmt.Fprintf(w, "  Requested:  %s\n", strings.Join(reqs, "  ·  "))

	if !j.SubmitTime.IsZero() {
		age := time.Since(j.SubmitTime)
		fmt.Fprintf(w, "  Submitted:  %s  (%s ago)\n",
			j.SubmitTime.Format("2006-01-02 15:04"),
			render.HumanDuration(age))
	}
	if j.State == "PENDING" && !j.StartTime.IsZero() {
		eta := time.Until(j.StartTime)
		var when string
		if eta > 0 {
			when = "in " + render.HumanDuration(eta)
		} else {
			when = "past — scheduler likely about to dispatch"
		}
		fmt.Fprintf(w, "  Estimated start: %s  (%s)\n",
			j.StartTime.Format("2006-01-02 15:04"),
			when)
	}

	if j.State != "PENDING" {
		fmt.Fprintf(w, "\n%s\n", render.ColorFaint("(job is "+j.State+"; no fit check needed)"))
		return
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "%s\n", render.Bold(fmt.Sprintf("Per-node fit check (%s)", j.Partition)))
	if len(r.NodeFits) == 0 {
		fmt.Fprintln(w, "  (no candidate nodes — partition empty?)")
		return
	}

	fits := 0
	for _, f := range r.NodeFits {
		marker := render.ColorRed("✗")
		if f.CanFit {
			marker = render.ColorGreen("✓")
			fits++
		}
		gpu := ""
		if f.GPUTotal > 0 {
			gpu = fmt.Sprintf("  GPU %d/%d %s", f.GPUFree, f.GPUTotal, f.GPUModel)
		}
		fmt.Fprintf(w, "  %s  %-12s  %-8s  CPU %d free  Mem %s free%s\n",
			marker, render.ColorCyan(f.Node), f.State, f.CPUFree, render.HumanMB(f.MemFree), gpu)
		for _, b := range f.Blockers {
			fmt.Fprintf(w, "        %s\n", render.ColorYellow("• "+b))
		}
	}

	fmt.Fprintln(w)
	if fits > 0 {
		fmt.Fprintf(w, "%s %d/%d nodes could fit this job right now\n",
			render.ColorGreen("→"), fits, len(r.NodeFits))
	} else {
		fmt.Fprintf(w, "%s 0/%d nodes can fit this job right now\n",
			render.ColorRed("→"), len(r.NodeFits))
	}
}

func colorState(s string) string {
	switch s {
	case "RUNNING":
		return render.ColorGreen(s)
	case "PENDING":
		return render.ColorYellow(s)
	default:
		return s
	}
}
