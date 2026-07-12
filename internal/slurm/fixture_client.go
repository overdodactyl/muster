package slurm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// NewFixtureClient returns a Client that reads Slurm --json fixture files
// from the given directory rather than shelling out. Missing files yield
// empty results without error, so a partial fixture set still boots.
//
// Recognized file names:
//
//	scontrol_nodes.json         Nodes()
//	scontrol_partitions.json    Partitions() + ClusterName()
//	squeue.json                 Jobs() + JobDetail()
//	sacct.json                  Accounting() + JobsByName()
//	scontrol_reservations.json  Reservations()
func NewFixtureClient(dir string) Client {
	return &fixtureClient{dir: dir}
}

type fixtureClient struct {
	dir string
}

func (c *fixtureClient) read(name string) ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(c.dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return b, err
}

func (c *fixtureClient) Nodes(ctx context.Context) ([]Node, error) {
	b, err := c.read("scontrol_nodes.json")
	if err != nil || b == nil {
		return nil, err
	}
	return parseScontrolNodes(b)
}

func (c *fixtureClient) Partitions(ctx context.Context) ([]Partition, error) {
	b, err := c.read("scontrol_partitions.json")
	if err != nil || b == nil {
		return nil, err
	}
	return parseScontrolPartitions(b)
}

func (c *fixtureClient) Jobs(ctx context.Context, partition string) ([]Job, error) {
	b, err := c.read("squeue.json")
	if err != nil || b == nil {
		return nil, err
	}
	jobs, err := parseSqueueJobs(b)
	if err != nil {
		return nil, err
	}
	if partition == "" {
		return jobs, nil
	}
	filtered := jobs[:0]
	for _, j := range jobs {
		if j.Partition == partition {
			filtered = append(filtered, j)
		}
	}
	return filtered, nil
}

func (c *fixtureClient) Accounting(ctx context.Context, since time.Duration, partition string) ([]AcctJob, error) {
	b, err := c.read("sacct.json")
	if err != nil || b == nil {
		return nil, err
	}
	acct, err := parseSacctJobs(b)
	if err != nil {
		return nil, err
	}
	cutoff := time.Time{}
	if since > 0 {
		cutoff = time.Now().Add(-since)
	}
	out := acct[:0]
	for _, a := range acct {
		if partition != "" && a.Partition != partition {
			continue
		}
		if !cutoff.IsZero() && !a.EndTime.IsZero() && a.EndTime.Before(cutoff) {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

func (c *fixtureClient) Reservations(ctx context.Context) ([]Reservation, error) {
	b, err := c.read("scontrol_reservations.json")
	if err != nil || b == nil {
		return nil, err
	}
	return parseScontrolReservations(b)
}

// Cancel is a no-op in fixture mode. A caller in demo mode isn't managing
// real jobs, and mutating the on-disk fixture would surprise the next reader.
func (c *fixtureClient) Cancel(ctx context.Context, jobID int64) error {
	return nil
}

// JobDetail synthesizes from the squeue fixture: same fields as Jobs plus
// empty stdout/stderr/cwd/command. Real deployments get these from
// `scontrol show job <id>` which the CLI client handles separately.
func (c *fixtureClient) JobDetail(ctx context.Context, jobID int64) (JobDetail, error) {
	jobs, err := c.Jobs(ctx, "")
	if err != nil {
		return JobDetail{}, err
	}
	for _, j := range jobs {
		if j.ID == jobID {
			return JobDetail{Job: j}, nil
		}
	}
	return JobDetail{}, nil
}

// JobEfficiency returns a plausible synthetic value for running jobs so the
// detail overlay doesn't render as all zeros in screenshots. Elapsed CPU
// scales with wall-clock runtime; RSS is a small fraction of the job's
// requested memory. Non-running or unknown jobs return zero.
func (c *fixtureClient) JobEfficiency(ctx context.Context, jobID int64) (JobEfficiency, error) {
	jobs, err := c.Jobs(ctx, "")
	if err != nil {
		return JobEfficiency{JobID: jobID}, nil
	}
	for _, j := range jobs {
		if j.ID != jobID || j.State != "RUNNING" || j.StartTime.IsZero() {
			continue
		}
		runtime := time.Since(j.StartTime)
		if runtime < 0 {
			runtime = 0
		}
		cores := j.CPUs
		if cores <= 0 {
			cores = 1
		}
		requestedMB := j.MemPerNode
		if requestedMB == 0 {
			requestedMB = j.MemPerCPU * cores
		}
		return JobEfficiency{
			JobID:    jobID,
			AveCPU:   time.Duration(float64(runtime) * float64(cores) * 0.72),
			AveRSSMB: requestedMB * 45 / 100,
			MaxRSSMB: requestedMB * 62 / 100,
		}, nil
	}
	return JobEfficiency{JobID: jobID}, nil
}

// JobGPUUtil returns nil in fixture mode — live nvidia-smi has no meaningful
// analogue. The dash gracefully hides the line when nil.
func (c *fixtureClient) JobGPUUtil(ctx context.Context, jobID int64) ([]GPUUtil, error) {
	return nil, nil
}

// JobsByName scans the sacct fixture for prior runs of a given name.
func (c *fixtureClient) JobsByName(ctx context.Context, name, partition string, since time.Duration) ([]AcctJob, error) {
	acct, err := c.Accounting(ctx, since, partition)
	if err != nil {
		return nil, err
	}
	out := acct[:0]
	for _, a := range acct {
		if a.Name == name {
			out = append(out, a)
		}
	}
	return out, nil
}

// ClusterName reads the meta block from any fixture that includes it. Falls
// back to "demo-cluster" so screenshots have a sensible label.
func (c *fixtureClient) ClusterName(ctx context.Context) (string, error) {
	for _, name := range []string{"scontrol_partitions.json", "scontrol_nodes.json", "squeue.json"} {
		b, err := c.read(name)
		if err != nil || b == nil {
			continue
		}
		if s, err := parseClusterName(b); err == nil && strings.TrimSpace(s) != "" {
			return s, nil
		}
	}
	return "demo-cluster", nil
}

// SlurmVersion returns a plausible fixed version for screenshots.
func (c *fixtureClient) SlurmVersion(ctx context.Context) (string, error) {
	return "slurm 24.05.4", nil
}
