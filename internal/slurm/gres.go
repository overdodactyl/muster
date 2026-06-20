package slurm

import (
	"strconv"
	"strings"
)

type GRES struct {
	Kind  string
	Model string
	Count int
	Index []int
}

// ParseGRES handles common Slurm GRES strings:
//
//	""                        -> nil
//	"gpu:4"                   -> kind=gpu, count=4
//	"gpu:a100:4"              -> kind=gpu, model=a100, count=4
//	"gpu:a100:3(IDX:0-2)"     -> ... plus index=[0,1,2]
//	"gpu:a100:4,nvme:1"       -> two entries
//
// Unparseable chunks are skipped silently so we don't crash on cluster-specific
// extensions we haven't seen.
func ParseGRES(s string) []GRES {
	s = strings.TrimSpace(s)
	if s == "" || s == "(null)" {
		return nil
	}
	var out []GRES
	for _, chunk := range splitTopLevel(s, ',') {
		g, ok := parseGRESChunk(chunk)
		if ok {
			out = append(out, g)
		}
	}
	return out
}

func parseGRESChunk(s string) (GRES, bool) {
	var g GRES
	idxOpen := strings.Index(s, "(")
	var head, tail string
	if idxOpen >= 0 {
		head = s[:idxOpen]
		tail = strings.TrimSuffix(s[idxOpen+1:], ")")
	} else {
		head = s
	}
	parts := strings.Split(strings.TrimSpace(head), ":")
	switch len(parts) {
	case 2:
		g.Kind = parts[0]
		n, err := strconv.Atoi(parts[1])
		if err != nil {
			return g, false
		}
		g.Count = n
	case 3:
		g.Kind = parts[0]
		g.Model = parts[1]
		n, err := strconv.Atoi(parts[2])
		if err != nil {
			return g, false
		}
		g.Count = n
	default:
		return g, false
	}
	if tail != "" {
		if rest, ok := strings.CutPrefix(tail, "IDX:"); ok {
			g.Index = expandIDX(rest)
		}
	}
	return g, true
}

func expandIDX(s string) []int {
	var out []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if lo, hi, ok := strings.Cut(part, "-"); ok {
			a, errA := strconv.Atoi(lo)
			b, errB := strconv.Atoi(hi)
			if errA != nil || errB != nil || a > b {
				continue
			}
			for i := a; i <= b; i++ {
				out = append(out, i)
			}
		} else if n, err := strconv.Atoi(part); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// splitTopLevel splits on sep but ignores separators inside (...) or [...].
func splitTopLevel(s string, sep rune) []string {
	var out []string
	depth := 0
	start := 0
	for i, r := range s {
		switch r {
		case '(', '[':
			depth++
		case ')', ']':
			if depth > 0 {
				depth--
			}
		case sep:
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	out = append(out, s[start:])
	return out
}

// GPUCount sums GPU counts across the parsed GRES entries.
func GPUCount(parsed []GRES) int {
	n := 0
	for _, g := range parsed {
		if g.Kind == "gpu" {
			n += g.Count
		}
	}
	return n
}

// GPUModel returns the first non-empty gpu model in the list, or "".
func GPUModel(parsed []GRES) string {
	for _, g := range parsed {
		if g.Kind == "gpu" && g.Model != "" {
			return g.Model
		}
	}
	return ""
}
