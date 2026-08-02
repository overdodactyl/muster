package render

import (
	"strings"
	"testing"
)

func TestFormatNodeState(t *testing.T) {
	cases := []struct {
		name    string
		states  []string
		class   string
		want    string
		notWant string
	}{
		{"plain mixed", []string{"MIXED"}, "mixed", "mixed", "·"},
		{"mixed planned", []string{"MIXED", "PLANNED"}, "mixed", "mixed·planned", ""},
		{"idle powered_down", []string{"IDLE", "POWERED_DOWN"}, "idle", "idle·powered_down", ""},
		{"drain with reboot", []string{"DRAIN", "REBOOT_REQUESTED"}, "drain", "drain·reboot_requested", ""},
		{"only class name", []string{}, "idle", "idle", "·"},
	}
	for _, c := range cases {
		got := stripAnsi(formatNodeState(c.states, c.class))
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: got %q, want substring %q", c.name, got, c.want)
		}
		if c.notWant != "" && strings.Contains(got, c.notWant) {
			t.Errorf("%s: got %q, should not contain %q", c.name, got, c.notWant)
		}
	}
}
