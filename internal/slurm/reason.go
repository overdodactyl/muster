package slurm

var reasonExplained = map[string]string{
	"Resources":                "Waiting for nodes to free up",
	"Priority":                 "Higher-priority jobs ahead in queue",
	"BeginTime":                "Holding until scheduled start time",
	"Dependency":               "Waiting on a dependent job to finish",
	"DependencyNeverSatisfied": "Dependency failed - job will never run",
	"JobHeldUser":              "Held by the submitting user (scontrol release)",
	"JobHeldAdmin":             "Held by an administrator",
	"QOSMaxJobsPerUserLimit":   "You've hit the per-user concurrent job limit",
	"QOSMaxCpuPerUserLimit":    "You've hit the per-user CPU limit",
	"QOSMaxGRESPerUser":        "You've hit the per-user GPU/GRES limit",
	"QOSMaxMemoryPerUser":      "You've hit the per-user memory limit",
	"AssocMaxJobsLimit":        "Account has hit its concurrent job limit",
	"AssocGrpCpuLimit":         "Account has hit its CPU limit",
	"JobArrayTaskLimit":        "Job array has hit its max-running-tasks limit",
	"PartitionTimeLimit":       "Requested time exceeds partition max",
	"PartitionNodeLimit":       "Requested nodes exceeds partition max",
	"ReqNodeNotAvail":          "Requested node is unavailable",
	"Reservation":              "Waiting for an active reservation",
	"NodeDown":                 "A requested node is down",
	"Licenses":                 "Waiting for a license",
	"None":                     "Eligible - scheduler hasn't picked it yet",
	"":                         "Eligible - scheduler hasn't picked it yet",
}

func ExplainReason(code string) string {
	if v, ok := reasonExplained[code]; ok {
		return v
	}
	return code
}
