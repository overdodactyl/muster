// Package store persists per-partition utilization samples to disk so that
// trend views can reach back days/weeks rather than the 5-minute in-memory
// ring buffer in the dash. Format is JSONL: one Sample per line, easy to
// grep / jq / rotate.
package store

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const SchemaVersion = 1

// DefaultPath is the well-known location for the history file. Caller can
// override via cmd flag.
func DefaultPath() string {
	if v := os.Getenv("MUSTER_HISTORY"); v != "" {
		return v
	}
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".cache", "muster", "history.jsonl")
	}
	return "muster-history.jsonl"
}

// Sample is one snapshot of cluster utilization at a moment in time.
type Sample struct {
	SchemaVersion int             `json:"schema_version"`
	At            time.Time       `json:"at"`
	Cluster       string          `json:"cluster,omitempty"`
	Partitions    []PartitionSnap `json:"partitions"`
}

type PartitionSnap struct {
	Name        string `json:"name"`
	AllocCPUs   int    `json:"alloc_cpus"`
	TotalCPUs   int    `json:"total_cpus"`
	AllocGPUs   int    `json:"alloc_gpus"`
	TotalGPUs   int    `json:"total_gpus"`
	AllocMemMB  int    `json:"alloc_mem_mb"`
	TotalMemMB  int    `json:"total_mem_mb"`
	RunningJobs int    `json:"running_jobs"`
	PendingJobs int    `json:"pending_jobs"`
}

// Append serializes one Sample as a JSON line to the file. Creates parent
// directories as needed. Concurrent-writer-safe because each line write is
// atomic for sizes below PIPE_BUF.
func Append(path string, s Sample) error {
	if s.SchemaVersion == 0 {
		s.SchemaVersion = SchemaVersion
	}
	if s.At.IsZero() {
		s.At = time.Now()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

// Read returns all samples in the file with At ≥ since. Order is by time
// ascending. Skips lines that fail to parse instead of erroring (assume
// the file is best-effort historical log).
func Read(path string, since time.Time) ([]Sample, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Sample
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var s Sample
		if err := json.Unmarshal(line, &s); err != nil {
			continue
		}
		if since.IsZero() || !s.At.Before(since) {
			out = append(out, s)
		}
	}
	if err := sc.Err(); err != nil {
		return out, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, nil
}

// PartitionSeries reduces a sample stream to per-partition time series of
// allocated-share percentages (cpu / gpu / mem). Used by `muster trend` to
// render sparklines.
type Series struct {
	Times []time.Time
	CPU   []int
	GPU   []int
	Mem   []int
}

func PartitionSeries(samples []Sample, partition string) Series {
	var s Series
	for _, sm := range samples {
		for _, p := range sm.Partitions {
			if p.Name != partition {
				continue
			}
			s.Times = append(s.Times, sm.At)
			s.CPU = append(s.CPU, pct(p.AllocCPUs, p.TotalCPUs))
			s.GPU = append(s.GPU, pct(p.AllocGPUs, p.TotalGPUs))
			s.Mem = append(s.Mem, pct(p.AllocMemMB, p.TotalMemMB))
		}
	}
	return s
}

// PartitionNames returns the unique set of partition names seen across the
// given samples, sorted alphabetically.
func PartitionNames(samples []Sample) []string {
	seen := map[string]bool{}
	for _, s := range samples {
		for _, p := range s.Partitions {
			seen[p.Name] = true
		}
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func pct(num, denom int) int {
	if denom <= 0 {
		return 0
	}
	return num * 100 / denom
}

// Downsample groups samples into `width` equal-time buckets and averages.
// Returns one value per bucket; empty buckets get -1 so callers can skip them.
func Downsample(values []int, times []time.Time, width int) []int {
	if width <= 0 || len(values) == 0 {
		return nil
	}
	if len(values) <= width {
		return values
	}
	start := times[0]
	end := times[len(times)-1]
	if !end.After(start) {
		return values
	}
	span := end.Sub(start)
	bucketDur := span / time.Duration(width)
	if bucketDur <= 0 {
		return values
	}
	out := make([]int, width)
	counts := make([]int, width)
	for i, v := range values {
		idx := int(times[i].Sub(start) / bucketDur)
		if idx >= width {
			idx = width - 1
		}
		out[idx] += v
		counts[idx]++
	}
	for i := range out {
		if counts[i] == 0 {
			out[i] = -1
			continue
		}
		out[i] /= counts[i]
	}
	// Forward-fill empty buckets so the sparkline stays continuous.
	last := 0
	for i, v := range out {
		if v < 0 {
			out[i] = last
		} else {
			last = v
		}
	}
	return out
}

// SampleStats returns (min, max, avg) for a series, ignoring negative
// sentinel values used by Downsample for empty buckets.
func SampleStats(values []int) (mn, mx, avg int) {
	if len(values) == 0 {
		return 0, 0, 0
	}
	mn, mx = 100, 0
	sum, n := 0, 0
	for _, v := range values {
		if v < 0 {
			continue
		}
		if v < mn {
			mn = v
		}
		if v > mx {
			mx = v
		}
		sum += v
		n++
	}
	if n == 0 {
		return 0, 0, 0
	}
	avg = sum / n
	return
}

func ParseSince(s string) (time.Duration, error) {
	if s == "" {
		return 7 * 24 * time.Hour, nil
	}
	if len(s) > 1 && s[len(s)-1] == 'd' {
		var d int
		_, err := fmt.Sscanf(s, "%dd", &d)
		if err != nil {
			return 0, err
		}
		return time.Duration(d) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
