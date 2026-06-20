package aggregate

import (
	"sort"
	"strings"
	"time"

	"muster/internal/slurm"
)

type AccountRollup struct {
	Account      string        `json:"account"`
	Users        int           `json:"users"`
	Running      int           `json:"running"`
	Pending      int           `json:"pending"`
	CPUsHeld     int           `json:"cpus_held"`
	GPUsHeld     int           `json:"gpus_held"`
	MemoryMBHeld int           `json:"memory_mb_held"`
	OldestRunAge time.Duration `json:"oldest_running_age_ns"`
}

// Accounts rolls jobs up by Slurm account ("lab"). Same metrics as Users()
// but the cluster question shifts from 'who is hogging this?' to 'which lab
// is over its share?'.
func Accounts(jobs []slurm.Job, partition string, sortBy string, top int, now time.Time) []AccountRollup {
	if now.IsZero() {
		now = time.Now()
	}
	byAcct := map[string]*AccountRollup{}
	users := map[string]map[string]bool{} // account -> set of users
	for _, j := range jobs {
		if partition != "" && j.Partition != partition {
			continue
		}
		acct := j.Account
		if acct == "" {
			acct = "(none)"
		}
		a, ok := byAcct[acct]
		if !ok {
			a = &AccountRollup{Account: acct}
			byAcct[acct] = a
			users[acct] = map[string]bool{}
		}
		users[acct][j.User] = true
		switch j.State {
		case "RUNNING":
			a.Running++
			a.CPUsHeld += j.CPUs
			a.GPUsHeld += jobGPUs(j)
			a.MemoryMBHeld += jobMemory(j)
			if !j.StartTime.IsZero() {
				age := now.Sub(j.StartTime)
				if age > a.OldestRunAge {
					a.OldestRunAge = age
				}
			}
		case "PENDING":
			a.Pending++
		}
	}
	out := make([]AccountRollup, 0, len(byAcct))
	for k, a := range byAcct {
		a.Users = len(users[k])
		out = append(out, *a)
	}
	sortAccounts(out, sortBy)
	if top > 0 && len(out) > top {
		out = out[:top]
	}
	return out
}

func sortAccounts(rows []AccountRollup, by string) {
	switch strings.ToLower(by) {
	case "gpus":
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].GPUsHeld > rows[j].GPUsHeld })
	case "mem":
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].MemoryMBHeld > rows[j].MemoryMBHeld })
	case "jobs":
		sort.SliceStable(rows, func(i, j int) bool {
			return (rows[i].Running + rows[i].Pending) > (rows[j].Running + rows[j].Pending)
		})
	case "users":
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].Users > rows[j].Users })
	case "name", "account":
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].Account < rows[j].Account })
	default: // "cpus"
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].CPUsHeld > rows[j].CPUsHeld })
	}
}
