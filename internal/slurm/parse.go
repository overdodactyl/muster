package slurm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

func decodeJSON(b []byte, dst any) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || b[0] != '{' {
		return fmt.Errorf("slurm did not return JSON; rebuild with json-c support or use a 23.x+ cluster")
	}
	return json.Unmarshal(b, dst)
}

func parseScontrolNodes(b []byte) ([]Node, error) {
	var w scontrolNodeWire
	if err := decodeJSON(b, &w); err != nil {
		return nil, err
	}
	out := make([]Node, 0, len(w.Nodes))
	for _, n := range w.Nodes {
		idle := n.CPUs - n.AllocCPUs
		if n.IdleCPUs != nil {
			idle = *n.IdleCPUs
		}
		out = append(out, Node{
			Name:          n.Name,
			Partitions:    n.Partitions,
			State:         n.State,
			CPUs:          n.CPUs,
			AllocCPUs:     n.AllocCPUs,
			IdleCPUs:      idle,
			RealMemoryMB:  n.RealMemory,
			AllocMemoryMB: n.AllocMemory,
			FreeMemoryMB:  n.FreeMemory.Int(),
			GRESTotal:     n.GRES,
			GRESUsed:      n.GRESUsed,
			Reason:        n.Reason,
			CPULoad:       float64(n.CPULoad) / 100.0,
		})
	}
	return out, nil
}

func parseScontrolPartitions(b []byte) ([]Partition, error) {
	var w scontrolPartWire
	if err := decodeJSON(b, &w); err != nil {
		return nil, err
	}
	out := make([]Partition, 0, len(w.Partitions))
	for _, p := range w.Partitions {
		nodes, _ := ExpandHostlist(p.Nodes.Configured)
		state := ""
		if s, ok := p.State.(string); ok {
			state = s
		}
		out = append(out, Partition{
			Name:    p.Name,
			Nodes:   nodes,
			MaxTime: time.Duration(p.Maximums.Int()) * time.Minute,
			State:   state,
		})
	}
	return out, nil
}

func parseSqueueJobs(b []byte) ([]Job, error) {
	var w squeueWire
	if err := decodeJSON(b, &w); err != nil {
		return nil, err
	}
	out := make([]Job, 0, len(w.Jobs))
	for _, j := range w.Jobs {
		state := ""
		if len(j.JobState) > 0 {
			state = j.JobState[0]
		}
		gres := j.TresPerNode
		if gres == "" {
			gres = j.TresPerJob
		}
		out = append(out, Job{
			ID:          j.JobID,
			User:        j.UserName,
			Account:     j.Account,
			Name:        j.Name,
			Partition:   j.Partition,
			State:       state,
			Reason:      j.StateReason,
			Nodes:       j.Nodes,
			NodeCount:   j.NodeCount.Int(),
			CPUs:        j.CPUs.Int(),
			MemPerCPU:   j.MemoryPerCPU.Int(),
			MemPerNode:  j.MemoryPerNode.Int(),
			GRESPerNode: gres,
			GRESDetail:  j.GRESDetail,
			SubmitTime:  j.SubmitTime.Time(),
			StartTime:   j.StartTime.Time(),
			EndTime:     j.EndTime.Time(),
			Priority:    int64(j.Priority.Int()),
			TimeLimit:   time.Duration(j.TimeLimit.Int()) * time.Minute,
		})
	}
	return out, nil
}

func parseScontrolReservations(b []byte) ([]Reservation, error) {
	var w scontrolReservationWire
	if err := decodeJSON(b, &w); err != nil {
		return nil, err
	}
	out := make([]Reservation, 0, len(w.Reservations))
	for _, r := range w.Reservations {
		out = append(out, Reservation{
			Name:      r.Name,
			Nodes:     r.NodeList,
			NodeCount: r.NodeCount,
			Partition: r.Partition,
			StartTime: r.StartTime.Time(),
			EndTime:   r.EndTime.Time(),
			Users:     r.Users,
			Accounts:  r.Accounts,
			Flags:     r.Flags,
			TRES:      r.TRES,
		})
	}
	return out, nil
}

func parseSacctJobs(b []byte) ([]AcctJob, error) {
	var w sacctWire
	if err := decodeJSON(b, &w); err != nil {
		return nil, err
	}
	out := make([]AcctJob, 0, len(w.Jobs))
	for _, j := range w.Jobs {
		state := ""
		if len(j.State.Current) > 0 {
			state = j.State.Current[0]
		}
		elapsed := time.Duration(j.Time.Elapsed) * time.Second
		out = append(out, AcctJob{
			ID:         j.JobID,
			User:       j.User,
			Account:    j.Account,
			Partition:  j.Partition,
			State:      state,
			SubmitTime: unixTime(j.Time.Submission),
			StartTime:  unixTime(j.Time.Start),
			EndTime:    unixTime(j.Time.End),
			Elapsed:    elapsed,
			AllocTRES:  tresMap(j.Tres.Allocated),
			ExitCode:   j.ExitCode.ReturnCode.Int(),
		})
	}
	return out, nil
}

func unixTime(t int64) time.Time {
	if t <= 0 {
		return time.Time{}
	}
	return time.Unix(t, 0)
}

func tresMap(items []tresItem) map[string]int {
	if len(items) == 0 {
		return nil
	}
	out := make(map[string]int, len(items))
	for _, t := range items {
		key := t.Type
		if t.Name != "" {
			key = t.Type + "/" + t.Name
		}
		out[key] = t.Count.Int()
	}
	return out
}
