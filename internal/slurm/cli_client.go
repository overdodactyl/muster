package slurm

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type cliClient struct {
	sinfo, squeue, scontrol, sacct, scancel, sstat string
	timeout                                        time.Duration
}

func NewCLIClient() Client {
	return &cliClient{
		sinfo:    "sinfo",
		squeue:   "squeue",
		scontrol: "scontrol",
		sacct:    "sacct",
		scancel:  "scancel",
		sstat:    "sstat",
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

func (c *cliClient) Cancel(ctx context.Context, jobID int64) error {
	_, err := c.run(ctx, c.scancel, fmt.Sprintf("%d", jobID))
	return err
}

func (c *cliClient) JobDetail(ctx context.Context, jobID int64) (JobDetail, error) {
	out, err := c.run(ctx, c.scontrol, "show", "job", fmt.Sprintf("%d", jobID), "--json")
	if err != nil {
		return JobDetail{}, err
	}
	return parseScontrolJobDetail(out)
}

// JobEfficiency calls `sstat` on the .batch step and parses cumulative CPU
// time + peak RSS. Returns zero-value (no error) if the step isn't present
// (interactive jobs, non-batch wrappers).
func (c *cliClient) JobEfficiency(ctx context.Context, jobID int64) (JobEfficiency, error) {
	out, err := c.run(ctx, c.sstat,
		fmt.Sprintf("--jobs=%d.batch", jobID),
		"--noheader",
		"--parsable2",
		"--format=JobID,AveCPU,MaxRSS",
	)
	if err != nil {
		// sstat returns non-zero when the .batch step is missing — treat as
		// "no live stats yet" rather than a hard error.
		return JobEfficiency{JobID: jobID}, nil
	}
	line := strings.TrimSpace(string(out))
	if line == "" {
		return JobEfficiency{JobID: jobID}, nil
	}
	parts := strings.Split(line, "|")
	if len(parts) < 3 {
		return JobEfficiency{JobID: jobID}, nil
	}
	return JobEfficiency{
		JobID:    jobID,
		AveCPU:   parseSlurmDuration(parts[1]),
		MaxRSSMB: parseSlurmBytes(parts[2]),
	}, nil
}

// parseSlurmDuration handles Slurm's "HH:MM:SS[.frac]" or "D-HH:MM:SS" duration
// strings. Returns 0 on parse failure.
func parseSlurmDuration(s string) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	var days int
	if i := strings.Index(s, "-"); i >= 0 {
		if d, err := strconv.Atoi(s[:i]); err == nil {
			days = d
		}
		s = s[i+1:]
	}
	// Strip subsecond fraction.
	if i := strings.Index(s, "."); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ":")
	var h, m, sec int
	switch len(parts) {
	case 3:
		h, _ = strconv.Atoi(parts[0])
		m, _ = strconv.Atoi(parts[1])
		sec, _ = strconv.Atoi(parts[2])
	case 2:
		m, _ = strconv.Atoi(parts[0])
		sec, _ = strconv.Atoi(parts[1])
	case 1:
		sec, _ = strconv.Atoi(parts[0])
	default:
		return 0
	}
	return time.Duration(days)*24*time.Hour +
		time.Duration(h)*time.Hour +
		time.Duration(m)*time.Minute +
		time.Duration(sec)*time.Second
}

// parseSlurmBytes handles Slurm's "1234K" / "1234M" / "1234G" suffixes and
// returns the value in megabytes.
func parseSlurmBytes(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	last := s[len(s)-1]
	mul := 0
	switch last {
	case 'K', 'k':
		mul = 0 // ÷1024 below
	case 'M', 'm':
		mul = 1
	case 'G', 'g':
		mul = 2
	case 'T', 't':
		mul = 3
	default:
		// Plain bytes
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			return int(v / 1024 / 1024)
		}
		return 0
	}
	v, err := strconv.ParseFloat(s[:len(s)-1], 64)
	if err != nil {
		return 0
	}
	switch mul {
	case 0:
		return int(v / 1024) // K -> MB
	case 1:
		return int(v) // M -> MB
	case 2:
		return int(v * 1024) // G -> MB
	case 3:
		return int(v * 1024 * 1024) // T -> MB
	}
	return 0
}

func (c *cliClient) Reservations(ctx context.Context) ([]Reservation, error) {
	out, err := c.run(ctx, c.scontrol, "show", "reservation", "--json")
	if err != nil {
		return nil, err
	}
	return parseScontrolReservations(out)
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
