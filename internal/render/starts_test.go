package render

import (
	"strings"
	"testing"
	"time"
)

// Strip ANSI color escapes so assertions can match raw text regardless
// of NO_COLOR / TTY detection.
func stripAnsi(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		if r == 0x1b {
			inEscape = true
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func TestFormatEstStart(t *testing.T) {
	now := time.Date(2026, 8, 2, 14, 30, 0, 0, time.Local)
	cases := []struct {
		name    string
		t       time.Time
		wantSub string
	}{
		{"zero", time.Time{}, "-"},
		{"in the past", now.Add(-5 * time.Minute), "now"},
		{"later today", now.Add(3 * time.Hour), "17:30"},
		{"tomorrow morning", now.Add(18 * time.Hour), "08:30"},
		{"far future", now.Add(30 * 24 * time.Hour), "Sep 1"},
	}
	for _, c := range cases {
		got := stripAnsi(formatEstStart(c.t, now))
		if !strings.Contains(got, c.wantSub) {
			t.Errorf("%s: formatEstStart(%v) = %q, want substring %q",
				c.name, c.t, got, c.wantSub)
		}
	}
}
