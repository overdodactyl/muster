package slurm

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		states []string
		want   StateClass
	}{
		{[]string{"IDLE"}, StateIdle},
		{[]string{"MIXED"}, StateMixed},
		{[]string{"ALLOCATED"}, StateAlloc},
		{[]string{"DOWN"}, StateDown},
		{[]string{"DRAIN", "ALLOCATED"}, StateDrain},
		{[]string{"RESERVED", "IDLE"}, StateReserved},
		{[]string{}, StateUnknown},
		{[]string{"COMPLETING"}, StateAlloc},
	}
	for _, c := range cases {
		if got := Classify(c.states); got != c.want {
			t.Errorf("Classify(%v) = %v, want %v", c.states, got, c.want)
		}
	}
}
