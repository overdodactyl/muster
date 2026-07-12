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

	// Array-job fields. Non-zero ArrayJobID + non-negative ArrayTaskID =>
	// this Job is one task of an array submitted with sbatch --array.
	ArrayJobID  int64
	ArrayTaskID int // -1 when not an array task

	// Dependency is the raw squeue 'dependency' field, e.g.
	// 'afterok:9300291_*(unfulfilled)'. Empty when none.
	Dependency string
}

// IsArrayTask reports whether this job belongs to a Slurm job array.
func (j Job) IsArrayTask() bool {
	return j.ArrayJobID != 0 && j.ArrayTaskID >= 0
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

// JobEfficiency is live CPU + memory utilization for a running job, queried
// via `sstat`. Zero values mean the call returned nothing (job not stepping,
// not running, or no permission).
type JobEfficiency struct {
	JobID    int64
	AveCPU   time.Duration // wall-clock cumulative CPU time used (across tasks)
	AveRSSMB int           // running-average resident set size in MB ("current")
	MaxRSSMB int           // peak resident set size in MB
}

// GPUUtil is one nvidia-smi sample from inside a running job's allocation.
type GPUUtil struct {
	Index      int // GPU index on the node
	UtilGPUPct int // 0..100, GPU compute utilization
	MemUsedMB  int
	MemTotalMB int
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
