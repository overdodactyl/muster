package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"muster/internal/render"
)

var (
	logsFollow    bool
	logsStderr    bool
	logsTailLines int
)

var logsCmd = &cobra.Command{
	Use:   "logs <jobid>",
	Short: "Print the stdout (or stderr) of a Slurm job; -f tails live",
	Long: `Reads the job's standard_output (or standard_error with --stderr) path
from 'scontrol show job <id>' and prints it. With --follow / -f the output
is tailed live like 'tail -f'. Interactive jobs that don't write to a file
(zsh, [RStudio Launcher], etc.) print a clear message instead of erroring.

This is the "what is my job actually doing right now?" answer — no more
hunting through the experiment directory for the matching .out file.`,
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
		d, err := client.JobDetail(ctx, jobID)
		if err != nil {
			return err
		}

		path := d.StandardOutput
		label := "stdout"
		if logsStderr {
			path = d.StandardError
			label = "stderr"
		}
		if path == "" {
			fmt.Fprintf(os.Stderr, "job %d (%s) has no %s file — looks like an interactive session (%s).\n",
				jobID, render.ColorCyan(d.Name), label, d.Command)
			return nil
		}

		fmt.Fprintf(os.Stderr, "%s %s\n", render.ColorFaint("# tail of"), path)
		if logsFollow {
			return tailFollow(path, logsTailLines)
		}
		return tailOnce(path, logsTailLines)
	},
}

func tailOnce(path string, n int) error {
	if n <= 0 {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(os.Stdout, f)
		return err
	}
	// Use the system tail for simplicity (handles large files efficiently).
	c := exec.Command("tail", "-n", fmt.Sprintf("%d", n), path)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func tailFollow(path string, n int) error {
	args := []string{"--retry", "-n", fmt.Sprintf("%d", n), "-f", path}
	c := exec.Command("tail", args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Start(); err != nil {
		return err
	}
	// Forward Ctrl+C to the tail process so it cleans up.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigs
		_ = c.Process.Signal(syscall.SIGTERM)
	}()
	return c.Wait()
}

func init() {
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "follow the file as new lines are written (like tail -f)")
	logsCmd.Flags().BoolVar(&logsStderr, "stderr", false, "show stderr instead of stdout")
	logsCmd.Flags().IntVarP(&logsTailLines, "lines", "n", 100, "show the last N lines (0 = whole file)")
	rootCmd.AddCommand(logsCmd)
}
