package slurm

import (
	"strconv"
	"strings"
)

// parseArrayTaskString counts the tasks represented by a Slurm compact
// array-task string and extracts the %N throttle. Slurm formats include:
//
//	"73-224%3"       -> 152 tasks, throttle 3
//	"1,3,5"          -> 3 tasks,   throttle 0
//	"1-3,7,10-12"    -> 7 tasks,   throttle 0
//	"1-100:2%4"      -> 51 tasks,  throttle 4  (step syntax)
//	""               -> 0 tasks,   throttle 0
//
// Malformed input yields (0, 0); callers should treat that as "unknown"
// rather than trusting a partial count.
func parseArrayTaskString(s string) (count int, throttle int) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0
	}
	if i := strings.LastIndex(s, "%"); i >= 0 {
		if n, err := strconv.Atoi(s[i+1:]); err == nil && n > 0 {
			throttle = n
		}
		s = s[:i]
	}
	total := 0
	for _, seg := range strings.Split(s, ",") {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		step := 1
		if i := strings.Index(seg, ":"); i >= 0 {
			if n, err := strconv.Atoi(seg[i+1:]); err == nil && n > 0 {
				step = n
			} else {
				return 0, 0
			}
			seg = seg[:i]
		}
		if dash := strings.Index(seg, "-"); dash >= 0 {
			lo, err1 := strconv.Atoi(seg[:dash])
			hi, err2 := strconv.Atoi(seg[dash+1:])
			if err1 != nil || err2 != nil || hi < lo {
				return 0, 0
			}
			total += (hi-lo)/step + 1
		} else {
			if _, err := strconv.Atoi(seg); err != nil {
				return 0, 0
			}
			total++
		}
	}
	return total, throttle
}
