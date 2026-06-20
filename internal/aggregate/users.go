package aggregate

import (
	"sort"
	"strings"
	"time"

	"muster/internal/slurm"
)

type UserRollup struct {
	User         string        `json:"user"`
	Running      int           `json:"running"`
	Pending      int           `json:"pending"`
	CPUsHeld     int           `json:"cpus_held"`
	GPUsHeld     int           `json:"gpus_held"`
	MemoryMBHeld int           `json:"memory_mb_held"`
	OldestRunAge time.Duration `json:"oldest_running_age_ns"`
}

// Users rolls jobs up by user for the given partition (empty = all).
// sortBy: "cpus" (default), "gpus", "mem", "jobs", "age".
// top: 0 = all; positive = truncate.
func Users(jobs []slurm.Job, partition, user string, sortBy string, top int, now time.Time) []UserRollup {
	if now.IsZero() {
		now = time.Now()
	}
	byUser := map[string]*UserRollup{}
	for _, j := range jobs {
		if partition != "" && j.Partition != partition {
			continue
		}
		if user != "" && j.User != user {
			continue
		}
		u, ok := byUser[j.User]
		if !ok {
			u = &UserRollup{User: j.User}
			byUser[j.User] = u
		}
		gpus := jobGPUs(j)
		mem := jobMemory(j)
		switch j.State {
		case "RUNNING":
			u.Running++
			u.CPUsHeld += j.CPUs
			u.GPUsHeld += gpus
			u.MemoryMBHeld += mem
			if !j.StartTime.IsZero() {
				age := now.Sub(j.StartTime)
				if age > u.OldestRunAge {
					u.OldestRunAge = age
				}
			}
		case "PENDING":
			u.Pending++
		}
	}
	out := make([]UserRollup, 0, len(byUser))
	for _, u := range byUser {
		out = append(out, *u)
	}
	sortUsers(out, sortBy)
	if top > 0 && len(out) > top {
		out = out[:top]
	}
	return out
}

func sortUsers(rows []UserRollup, by string) {
	switch strings.ToLower(by) {
	case "gpus":
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].GPUsHeld > rows[j].GPUsHeld })
	case "mem":
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].MemoryMBHeld > rows[j].MemoryMBHeld })
	case "jobs":
		sort.SliceStable(rows, func(i, j int) bool {
			return (rows[i].Running + rows[i].Pending) > (rows[j].Running + rows[j].Pending)
		})
	case "age":
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].OldestRunAge > rows[j].OldestRunAge })
	default: // "cpus" or any unknown
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].CPUsHeld > rows[j].CPUsHeld })
	}
}

// jobGPUs parses tres_per_node or tres_per_job ("gres/gpu:1") and multiplies by nodes.
func jobGPUs(j slurm.Job) int {
	g := parseGresGPUSpec(j.GRESPerNode)
	if g == 0 {
		return 0
	}
	n := j.NodeCount
	if n <= 0 {
		n = 1
	}
	return g * n
}

func parseGresGPUSpec(s string) int {
	// Handle "gres/gpu:1", "gpu:1", "gres/gpu:a100:2"
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	for _, chunk := range strings.Split(s, ",") {
		chunk = strings.TrimSpace(chunk)
		chunk = strings.TrimPrefix(chunk, "gres/")
		gres := slurm.ParseGRES(chunk)
		if n := slurm.GPUCount(gres); n > 0 {
			return n
		}
	}
	return 0
}

// jobMemory returns total job memory in MB.
func jobMemory(j slurm.Job) int {
	if j.MemPerNode > 0 {
		n := j.NodeCount
		if n <= 0 {
			n = 1
		}
		return j.MemPerNode * n
	}
	if j.MemPerCPU > 0 {
		return j.MemPerCPU * j.CPUs
	}
	return 0
}
