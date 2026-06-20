package slurm

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type cliClient struct {
	sinfo, squeue, scontrol, sacct string
	timeout                        time.Duration
}

func NewCLIClient() Client {
	return &cliClient{
		sinfo:    "sinfo",
		squeue:   "squeue",
		scontrol: "scontrol",
		sacct:    "sacct",
		timeout:  30 * time.Second,
	}
}

func (c *cliClient) run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if _, ok := ctx.Deadline(); !ok && c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return out, nil
}

func (c *cliClient) SlurmVersion(ctx context.Context) (string, error) {
	out, err := c.run(ctx, c.sinfo, "--version")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (c *cliClient) Nodes(ctx context.Context) ([]Node, error) {
	out, err := c.run(ctx, c.scontrol, "show", "node", "--json")
	if err != nil {
		return nil, err
	}
	return parseScontrolNodes(out)
}

func (c *cliClient) Jobs(ctx context.Context, partition string) ([]Job, error) {
	args := []string{"--json"}
	if partition != "" {
		args = append(args, "-p", partition)
	}
	out, err := c.run(ctx, c.squeue, args...)
	if err != nil {
		return nil, err
	}
	return parseSqueueJobs(out)
}

func (c *cliClient) Partitions(ctx context.Context) ([]Partition, error) {
	out, err := c.run(ctx, c.scontrol, "show", "partition", "--json")
	if err != nil {
		return nil, err
	}
	return parseScontrolPartitions(out)
}

func (c *cliClient) Accounting(ctx context.Context, since time.Duration, partition string) ([]AcctJob, error) {
	if since <= 0 {
		since = 24 * time.Hour
	}
	start := fmt.Sprintf("now-%dminutes", int(since.Minutes()))
	args := []string{"--json", "--starttime", start, "--allusers"}
	if partition != "" {
		args = append(args, "-r", partition)
	}
	out, err := c.run(ctx, c.sacct, args...)
	if err != nil {
		return nil, err
	}
	return parseSacctJobs(out)
}
