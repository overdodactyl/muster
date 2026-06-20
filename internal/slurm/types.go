package slurm

import "time"

type Node struct {
	Name          string
	Partitions    []string
	State         []string
	CPUs          int
	AllocCPUs     int
	IdleCPUs      int
	RealMemoryMB  int
	AllocMemoryMB int
	FreeMemoryMB  int
	GRESTotal     string
	GRESUsed      string
	Reason        string
	CPULoad       float64
}

type Job struct {
	ID          int64
	User        string
	Account     string
	Name        string
	Partition   string
	State       string
	Reason      string
	Nodes       string
	NodeCount   int
	CPUs        int
	MemPerCPU   int
	MemPerNode  int
	GRESPerNode string
	GRESDetail  []string // per-node assigned GRES, e.g. ["gpu:a100:1(IDX:0)"]
	SubmitTime  time.Time
	StartTime   time.Time
	EndTime     time.Time
	Priority    int64
	TimeLimit   time.Duration
}

// JobDetail is the extra info available via `scontrol show job <id>` that
// squeue's listing doesn't surface: stdout/stderr paths, working dir,
// command, etc.
type JobDetail struct {
	Job
	StandardOutput          string
	StandardError           string
	CurrentWorkingDirectory string
	Command                 string
}

type Partition struct {
	Name    string
	Nodes   []string
	MaxTime time.Duration
	State   string
}

type Reservation struct {
	Name      string
	Nodes     string
	NodeCount int
	Partition string
	StartTime time.Time
	EndTime   time.Time
	Users     string
	Accounts  string
	Flags     []string
	TRES      string
}

type AcctJob struct {
	ID         int64
	Name       string
	User       string
	Account    string
	Partition  string
	State      string
	SubmitTime time.Time
	StartTime  time.Time
	EndTime    time.Time
	Elapsed    time.Duration
	TotalCPU   time.Duration // user+system CPU time consumed across all cores
	AllocTRES  map[string]int
	ExitCode   int
}
