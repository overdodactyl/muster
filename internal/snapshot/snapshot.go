// Package snapshot persists and compares cluster state captures.
package snapshot

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"muster/internal/slurm"
)

const SchemaVersion = 1

type File struct {
	SchemaVersion int                 `json:"schema_version"`
	CapturedAt    time.Time           `json:"captured_at"`
	Cluster       string              `json:"cluster,omitempty"`
	Nodes         []slurm.Node        `json:"nodes"`
	Jobs          []slurm.Job         `json:"jobs"`
	Reservations  []slurm.Reservation `json:"reservations"`
}

func Write(path string, f File) error {
	if f.SchemaVersion == 0 {
		f.SchemaVersion = SchemaVersion
	}
	if f.CapturedAt.IsZero() {
		f.CapturedAt = time.Now()
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

func Read(path string) (File, error) {
	var f File
	b, err := os.ReadFile(path)
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return f, fmt.Errorf("parse snapshot %s: %w", path, err)
	}
	if f.SchemaVersion != SchemaVersion {
		return f, fmt.Errorf("snapshot %s has schema_version %d, expected %d", path, f.SchemaVersion, SchemaVersion)
	}
	return f, nil
}

// Diff is the structured difference between two snapshots.
type Diff struct {
	OldAt time.Time
	NewAt time.Time

	JobsAdded   []slurm.Job
	JobsRemoved []slurm.Job
	JobsChanged []JobStateChange

	NodesChanged []NodeStateChange

	ReservationsAdded   []slurm.Reservation
	ReservationsRemoved []slurm.Reservation
}

type JobStateChange struct {
	Job      slurm.Job
	OldState string
	NewState string
}

type NodeStateChange struct {
	Name     string
	OldState []string
	NewState []string
}

func Compute(old, new File) Diff {
	d := Diff{OldAt: old.CapturedAt, NewAt: new.CapturedAt}

	oldJobs := indexJobs(old.Jobs)
	newJobs := indexJobs(new.Jobs)
	for id, j := range newJobs {
		if _, ok := oldJobs[id]; !ok {
			d.JobsAdded = append(d.JobsAdded, *j)
		} else if oldJobs[id].State != j.State {
			d.JobsChanged = append(d.JobsChanged, JobStateChange{
				Job: *j, OldState: oldJobs[id].State, NewState: j.State,
			})
		}
	}
	for id, j := range oldJobs {
		if _, ok := newJobs[id]; !ok {
			d.JobsRemoved = append(d.JobsRemoved, *j)
		}
	}

	oldNodes := indexNodes(old.Nodes)
	newNodes := indexNodes(new.Nodes)
	for name, n := range newNodes {
		o, ok := oldNodes[name]
		if !ok {
			continue
		}
		if !stateEqual(o.State, n.State) {
			d.NodesChanged = append(d.NodesChanged, NodeStateChange{
				Name: name, OldState: o.State, NewState: n.State,
			})
		}
	}

	oldRes := indexReservations(old.Reservations)
	newRes := indexReservations(new.Reservations)
	for name, r := range newRes {
		if _, ok := oldRes[name]; !ok {
			d.ReservationsAdded = append(d.ReservationsAdded, *r)
		}
	}
	for name, r := range oldRes {
		if _, ok := newRes[name]; !ok {
			d.ReservationsRemoved = append(d.ReservationsRemoved, *r)
		}
	}

	return d
}

func indexJobs(jobs []slurm.Job) map[int64]*slurm.Job {
	m := make(map[int64]*slurm.Job, len(jobs))
	for i := range jobs {
		m[jobs[i].ID] = &jobs[i]
	}
	return m
}

func indexNodes(nodes []slurm.Node) map[string]*slurm.Node {
	m := make(map[string]*slurm.Node, len(nodes))
	for i := range nodes {
		m[nodes[i].Name] = &nodes[i]
	}
	return m
}

func indexReservations(res []slurm.Reservation) map[string]*slurm.Reservation {
	m := make(map[string]*slurm.Reservation, len(res))
	for i := range res {
		m[res[i].Name] = &res[i]
	}
	return m
}

func stateEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	return strings.Join(a, ",") == strings.Join(b, ",")
}
