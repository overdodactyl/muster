package aggregate

import (
	"fmt"
	"strconv"
	"strings"
)

// Cond is a parsed condition of the form `<partition>.<field> <op> <value>`,
// e.g., `gpu.gpu_free >= 1`. Used by `muster wait --until`.
type Cond struct {
	Partition string
	Field     string
	Op        string
	Value     float64
}

// Supported fields and how to extract them from a PartitionSummary.
var condFields = map[string]func(PartitionSummary) float64{
	"cpu_free":     func(p PartitionSummary) float64 { return float64(p.TotalCPUs - p.AllocCPUs) },
	"cpu_alloc":    func(p PartitionSummary) float64 { return float64(p.AllocCPUs) },
	"cpu_total":    func(p PartitionSummary) float64 { return float64(p.TotalCPUs) },
	"gpu_free":     func(p PartitionSummary) float64 { return float64(p.TotalGPUs - p.AllocGPUs) },
	"gpu_alloc":    func(p PartitionSummary) float64 { return float64(p.AllocGPUs) },
	"gpu_total":    func(p PartitionSummary) float64 { return float64(p.TotalGPUs) },
	"mem_free_gb":  func(p PartitionSummary) float64 { return float64(p.TotalMemMB-p.AllocMemMB) / 1024.0 },
	"mem_alloc_gb": func(p PartitionSummary) float64 { return float64(p.AllocMemMB) / 1024.0 },
	"idle_nodes":   func(p PartitionSummary) float64 { return float64(p.NodeCounts.Idle) },
	"mixed_nodes":  func(p PartitionSummary) float64 { return float64(p.NodeCounts.Mixed) },
	"down_nodes":   func(p PartitionSummary) float64 { return float64(p.NodeCounts.Down + p.NodeCounts.Drain) },
	"running_jobs": func(p PartitionSummary) float64 { return float64(p.RunningJobs) },
	"pending_jobs": func(p PartitionSummary) float64 { return float64(p.PendingJobs) },
}

// ParseCond parses an expression like "gpu.gpu_free >= 1" (whitespace optional).
func ParseCond(expr string) (*Cond, error) {
	expr = strings.TrimSpace(expr)
	// Longest-first so ">=" wins over ">".
	var op string
	var opIdx int = -1
	for _, candidate := range []string{">=", "<=", "==", "!=", ">", "<", "="} {
		if idx := strings.Index(expr, candidate); idx >= 0 {
			op = candidate
			opIdx = idx
			break
		}
	}
	if opIdx < 0 {
		return nil, fmt.Errorf("no comparison operator in %q (use >= <= == != > <)", expr)
	}
	left := strings.TrimSpace(expr[:opIdx])
	right := strings.TrimSpace(expr[opIdx+len(op):])
	if op == "=" {
		op = "=="
	}

	dot := strings.IndexByte(left, '.')
	if dot < 0 {
		return nil, fmt.Errorf("left side must be '<partition>.<field>', got %q", left)
	}
	c := &Cond{
		Partition: left[:dot],
		Field:     left[dot+1:],
		Op:        op,
	}
	if _, ok := condFields[c.Field]; !ok {
		var known []string
		for k := range condFields {
			known = append(known, k)
		}
		return nil, fmt.Errorf("unknown field %q; known: %s", c.Field, strings.Join(known, ", "))
	}
	v, err := strconv.ParseFloat(right, 64)
	if err != nil {
		return nil, fmt.Errorf("right side %q is not a number: %w", right, err)
	}
	c.Value = v
	return c, nil
}

// Eval evaluates the condition against the given partition summaries.
// Returns the current numeric value and whether the condition holds.
func (c *Cond) Eval(parts []PartitionSummary) (current float64, ok bool, err error) {
	var p *PartitionSummary
	for i := range parts {
		if parts[i].Name == c.Partition {
			p = &parts[i]
			break
		}
	}
	if p == nil {
		return 0, false, fmt.Errorf("partition %q not present in current cluster state", c.Partition)
	}
	getter := condFields[c.Field]
	current = getter(*p)
	switch c.Op {
	case ">=":
		ok = current >= c.Value
	case "<=":
		ok = current <= c.Value
	case "==":
		ok = current == c.Value
	case "!=":
		ok = current != c.Value
	case ">":
		ok = current > c.Value
	case "<":
		ok = current < c.Value
	}
	return current, ok, nil
}
