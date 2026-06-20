package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"muster/internal/render"
)

var (
	sshShell   string
	sshOverlap bool
)

var sshCmd = &cobra.Command{
	Use:   "ssh <jobid>",
	Short: "Drop into a shell inside a running job's allocation (srun --pty)",
	Long: `Attaches an interactive shell to a running job by exec-ing
'srun --jobid=<id> --pty <shell>'. Only works on RUNNING jobs you own.

This is the cluster-version of 'kubectl exec' — when you want to poke
around the runtime environment of a job (env vars, files written, GPU
visibility, module state) without ssh'ing the node directly.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		jobID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid jobid %q: %w", args[0], err)
		}

		// Pre-flight: confirm the job is running so the error message is clean
		// rather than 'srun: error: ...'.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		client, err := newClient()
		if err != nil {
			return err
		}
		d, err := client.JobDetail(ctx, jobID)
		if err != nil {
			return err
		}
		if d.State != "RUNNING" {
			return fmt.Errorf("job %d is %s, not RUNNING — can't attach", jobID, d.State)
		}

		fmt.Fprintf(os.Stderr, "%s job %d (%s) on %s — exit shell to detach\n",
			render.ColorFaint("# attaching to"),
			jobID, d.Name, d.Nodes)

		srun, err := exec.LookPath("srun")
		if err != nil {
			return fmt.Errorf("srun not found in PATH: %w", err)
		}
		args2 := []string{"srun", fmt.Sprintf("--jobid=%d", jobID), "--pty"}
		if sshOverlap {
			args2 = append(args2, "--overlap")
		}
		args2 = append(args2, sshShell)
		// Replace the muster process entirely with srun so it owns the TTY.
		return syscall.Exec(srun, args2, os.Environ())
	},
}

func init() {
	sshCmd.Flags().StringVar(&sshShell, "shell", "bash", "shell to launch inside the job allocation")
	sshCmd.Flags().BoolVar(&sshOverlap, "overlap", true, "pass --overlap to srun (lets the shell share the running job's resources)")
	rootCmd.AddCommand(sshCmd)
}
