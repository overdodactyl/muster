package slurm

// All wire structs include only fields we actually use; encoding/json silently
// ignores everything else. Fields confirmed against Slurm 24.11.0 JSON output.

// scontrol show node --json
type scontrolNodeWire struct {
	Nodes []scontrolNode `json:"nodes"`
}

type scontrolNode struct {
	Name         string   `json:"name"`
	State        []string `json:"state"`
	Partitions   []string `json:"partitions"`
	CPUs         int      `json:"cpus"`
	AllocCPUs    int      `json:"alloc_cpus"`
	IdleCPUs     *int     `json:"idle_cpus"`
	RealMemory   int      `json:"real_memory"`
	AllocMemory  int      `json:"alloc_memory"`
	FreeMemory   slurmNum `json:"free_mem"`
	GRES         string   `json:"gres"`
	GRESUsed     string   `json:"gres_used"`
	Reason       string   `json:"reason"`
	CPULoad      int      `json:"cpu_load"`
}

// scontrol show partition --json
type scontrolPartWire struct {
	Partitions []scontrolPart `json:"partitions"`
}

type scontrolPart struct {
	Name     string `json:"name"`
	State    any    `json:"state"`
	Nodes    struct {
		Configured string `json:"configured"`
	} `json:"nodes"`
	Maximums slurmNum `json:"maximums"`
}

// squeue --json
type squeueWire struct {
	Jobs []squeueJob `json:"jobs"`
}

type squeueJob struct {
	JobID                   int64    `json:"job_id"`
	UserName                string   `json:"user_name"`
	Account                 string   `json:"account"`
	Name                    string   `json:"name"`
	Partition               string   `json:"partition"`
	JobState                []string `json:"job_state"`
	StateReason             string   `json:"state_reason"`
	Nodes                   string   `json:"nodes"`
	NodeCount               slurmNum `json:"node_count"`
	CPUs                    slurmNum `json:"cpus"`
	MemoryPerCPU            slurmNum `json:"memory_per_cpu"`
	MemoryPerNode           slurmNum `json:"memory_per_node"`
	TresPerNode             string   `json:"tres_per_node"`
	TresPerJob              string   `json:"tres_per_job"`
	GRESDetail              []string `json:"gres_detail"`
	SubmitTime              slurmNum `json:"submit_time"`
	StartTime               slurmNum `json:"start_time"`
	EndTime                 slurmNum `json:"end_time"`
	Priority                slurmNum `json:"priority"`
	TimeLimit               slurmNum `json:"time_limit"`
	StandardOutput          string   `json:"standard_output"`
	StandardError           string   `json:"standard_error"`
	CurrentWorkingDirectory string   `json:"current_working_directory"`
	Command                 string   `json:"command"`
}

// sacct --json
type sacctWire struct {
	Jobs []sacctJob `json:"jobs"`
}

type sacctJob struct {
	JobID     int64  `json:"job_id"`
	Name      string `json:"name"`
	User      string `json:"user"`
	Account   string `json:"account"`
	Partition string `json:"partition"`
	State     struct {
		Current []string `json:"current"`
	} `json:"state"`
	Time struct {
		Submission int64 `json:"submission"`
		Start      int64 `json:"start"`
		End        int64 `json:"end"`
		Elapsed    int64 `json:"elapsed"`
		Total      struct {
			Seconds int64 `json:"seconds"`
		} `json:"total"`
	} `json:"time"`
	Tres struct {
		Allocated []tresItem `json:"allocated"`
		Requested []tresItem `json:"requested"`
	} `json:"tres"`
	ExitCode struct {
		ReturnCode slurmNum `json:"return_code"`
	} `json:"exit_code"`
}

type tresItem struct {
	Type  string   `json:"type"`
	Name  string   `json:"name"`
	Count slurmNum `json:"count"`
}

// scontrol show reservation --json
type scontrolReservationWire struct {
	Reservations []scontrolReservation `json:"reservations"`
}

type scontrolReservation struct {
	Name      string   `json:"name"`
	NodeList  string   `json:"node_list"`
	NodeCount int      `json:"node_count"`
	Partition string   `json:"partition"`
	StartTime slurmNum `json:"start_time"`
	EndTime   slurmNum `json:"end_time"`
	Users     string   `json:"users"`
	Accounts  string   `json:"accounts"`
	Flags     []string `json:"flags"`
	TRES      string   `json:"tres"`
}
