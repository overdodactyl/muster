package slurm

import (
	"fmt"
	"strconv"
	"strings"
)

// ExpandHostlist expands a Slurm hostlist expression into individual node names.
//
//	"node013a"                 -> [node013a]
//	"node[013-014]a"           -> [node013a, node014a]
//	"node[013-014,016]a"       -> [node013a, node014a, node016a]
//	"nodeA,nodeB"                -> [nodeA, nodeB]
//	"prefix[01-03],other[1-2]"   -> [prefix01..prefix03, other1, other2]
//
// Padding is preserved from the lower bound of each range.
func ExpandHostlist(expr string) ([]string, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, nil
	}
	var out []string
	for _, segment := range splitTopLevel(expr, ',') {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		expanded, err := expandSegment(segment)
		if err != nil {
			return nil, err
		}
		out = append(out, expanded...)
	}
	return out, nil
}

func expandSegment(s string) ([]string, error) {
	open := strings.Index(s, "[")
	if open < 0 {
		return []string{s}, nil
	}
	close := strings.Index(s, "]")
	if close < open {
		return nil, fmt.Errorf("unbalanced brackets in %q", s)
	}
	prefix := s[:open]
	suffix := s[close+1:]
	body := s[open+1 : close]

	var out []string
	for _, part := range strings.Split(body, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if lo, hi, ok := strings.Cut(part, "-"); ok {
			width := len(lo)
			a, errA := strconv.Atoi(lo)
			b, errB := strconv.Atoi(hi)
			if errA != nil || errB != nil {
				return nil, fmt.Errorf("bad range %q in %q", part, s)
			}
			if a > b {
				return nil, fmt.Errorf("descending range %q in %q", part, s)
			}
			for i := a; i <= b; i++ {
				out = append(out, fmt.Sprintf("%s%0*d%s", prefix, width, i, suffix))
			}
		} else {
			out = append(out, prefix+part+suffix)
		}
	}
	return out, nil
}
