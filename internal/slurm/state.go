package slurm

import "strings"

type StateClass int

const (
	StateUnknown StateClass = iota
	StateIdle
	StateMixed
	StateAlloc
	StateReserved
	StateDrain
	StateDown
)

func (s StateClass) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateMixed:
		return "mixed"
	case StateAlloc:
		return "alloc"
	case StateReserved:
		return "reserved"
	case StateDrain:
		return "drain"
	case StateDown:
		return "down"
	default:
		return "unknown"
	}
}

// Classify reduces Slurm's flag list to a single canonical state.
// Priority: DOWN > DRAIN/DRAINED/DRAINING > FAIL/NOT_RESPONDING/MAINTENANCE >
// RESERVED > MIXED > ALLOCATED > IDLE.
func Classify(states []string) StateClass {
	has := make(map[string]bool, len(states))
	for _, s := range states {
		has[strings.ToUpper(s)] = true
	}
	switch {
	case has["DOWN"], has["FAIL"], has["NOT_RESPONDING"], has["FAILING"]:
		return StateDown
	case has["DRAIN"], has["DRAINED"], has["DRAINING"]:
		return StateDrain
	case has["MAINT"], has["MAINTENANCE"]:
		return StateDrain
	case has["RESERVED"]:
		return StateReserved
	case has["MIXED"]:
		return StateMixed
	case has["ALLOCATED"], has["COMPLETING"]:
		return StateAlloc
	case has["IDLE"]:
		return StateIdle
	default:
		return StateUnknown
	}
}
