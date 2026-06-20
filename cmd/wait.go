package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"muster/internal/aggregate"
	"muster/internal/render"
)

var (
	waitUntil    string
	waitInterval time.Duration
	waitMax      time.Duration
	waitWebhook  string
	waitQuiet    bool
)

var waitCmd = &cobra.Command{
	Use:   "wait",
	Short: "Poll until a partition condition becomes true (e.g. a GPU frees up)",
	Long: `Poll the cluster until --until expression evaluates true. Useful for
'ping me when there's a free GPU'.

Expression format:  <partition>.<field> <op> <value>
  e.g.  gpu.gpu_free >= 1
        cpu.cpu_free > 100
        gpu.idle_nodes >= 1
        gpu.pending_jobs == 0

Fields:  cpu_free, cpu_alloc, cpu_total, gpu_free, gpu_alloc, gpu_total,
         mem_free_gb, mem_alloc_gb, idle_nodes, mixed_nodes, down_nodes,
         running_jobs, pending_jobs

Operators: >= <= == != > <

On success: rings the terminal bell, prints the satisfied condition, exits 0.
With --webhook <url>, additionally POSTs a small JSON payload to the URL
(Slack-compatible: {"text": "..."}).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if waitUntil == "" {
			return fmt.Errorf("--until is required")
		}
		cond, err := aggregate.ParseCond(waitUntil)
		if err != nil {
			return err
		}

		client, err := newClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigs
			fmt.Fprintln(os.Stderr, "\ninterrupted")
			cancel()
		}()

		deadline := time.Now().Add(waitMax)
		fmt.Fprintf(os.Stderr, "%s waiting for %s (poll every %s, timeout %s)\n",
			render.ColorCyan("muster wait"),
			render.Bold(waitUntil),
			waitInterval, waitMax)

		ticker := time.NewTicker(waitInterval)
		defer ticker.Stop()

		check := func() (bool, error) {
			cctx, ccancel := context.WithTimeout(ctx, 30*time.Second)
			defer ccancel()
			nodes, err := client.Nodes(cctx)
			if err != nil {
				return false, err
			}
			jobs, err := client.Jobs(cctx, cond.Partition)
			if err != nil {
				return false, err
			}
			parts := aggregate.Partitions(nodes, jobs, cond.Partition)
			current, ok, err := cond.Eval(parts)
			if err != nil {
				return false, err
			}
			if !waitQuiet {
				marker := "…"
				if ok {
					marker = render.ColorGreen("✓")
				}
				fmt.Fprintf(os.Stderr, "  [%s] %s.%s = %g  (need %s %g)  %s\n",
					time.Now().Format("15:04:05"),
					cond.Partition, cond.Field, current,
					cond.Op, cond.Value, marker)
			}
			if ok {
				return true, nil
			}
			return false, nil
		}

		// Immediate first check so the user sees state right away.
		if ok, err := check(); err != nil {
			return err
		} else if ok {
			return onSatisfied(cond)
		}

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				if time.Now().After(deadline) {
					return fmt.Errorf("timeout: condition %q not met within %s", waitUntil, waitMax)
				}
				ok, err := check()
				if err != nil {
					return err
				}
				if ok {
					return onSatisfied(cond)
				}
			}
		}
	},
}

func onSatisfied(c *aggregate.Cond) error {
	msg := fmt.Sprintf("muster wait: condition met — %s.%s %s %g", c.Partition, c.Field, c.Op, c.Value)
	// Terminal bell.
	fmt.Fprint(os.Stderr, "\a")
	fmt.Fprintln(os.Stderr, render.ColorGreen("✓ "+msg))
	if waitWebhook != "" {
		if err := postWebhook(waitWebhook, msg); err != nil {
			fmt.Fprintln(os.Stderr, render.ColorYellow("(webhook failed: "+err.Error()+")"))
		}
	}
	return nil
}

func postWebhook(url, text string) error {
	body, _ := json.Marshal(map[string]string{"text": text})
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

func init() {
	waitCmd.Flags().StringVar(&waitUntil, "until", "", "condition expression, e.g. 'gpu.gpu_free >= 1'")
	waitCmd.Flags().DurationVar(&waitInterval, "interval", 30*time.Second, "poll interval")
	waitCmd.Flags().DurationVar(&waitMax, "max", 24*time.Hour, "give up after this long")
	waitCmd.Flags().StringVar(&waitWebhook, "webhook", "", "POST {\"text\": ...} to this URL when condition met (Slack-compatible)")
	waitCmd.Flags().BoolVar(&waitQuiet, "quiet", false, "don't print per-poll progress to stderr")
	waitCmd.MarkFlagRequired("until")
	rootCmd.AddCommand(waitCmd)
}
