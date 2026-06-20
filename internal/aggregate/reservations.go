package aggregate

import (
	"sort"
	"time"

	"muster/internal/slurm"
)

type ReservationRow struct {
	Name      string        `json:"name"`
	State     string        `json:"state"`
	NodeCount int           `json:"node_count"`
	Nodes     string        `json:"nodes"`
	Partition string        `json:"partition,omitempty"`
	Users     string        `json:"users,omitempty"`
	Accounts  string        `json:"accounts,omitempty"`
	StartTime time.Time     `json:"start_time"`
	EndTime   time.Time     `json:"end_time"`
	Duration  time.Duration `json:"duration_ns"`
	StartsIn  time.Duration `json:"starts_in_ns,omitempty"`
	EndsIn    time.Duration `json:"ends_in_ns,omitempty"`
	TRES      string        `json:"tres,omitempty"`
}

// Reservations enriches slurm.Reservation values with derived fields (state,
// time-until-start/end, duration) and sorts them so active ones come first,
// then upcoming, then past.
func Reservations(items []slurm.Reservation, now time.Time) []ReservationRow {
	if now.IsZero() {
		now = time.Now()
	}
	out := make([]ReservationRow, 0, len(items))
	for _, r := range items {
		row := ReservationRow{
			Name:      r.Name,
			NodeCount: r.NodeCount,
			Nodes:     r.Nodes,
			Partition: r.Partition,
			Users:     r.Users,
			Accounts:  r.Accounts,
			StartTime: r.StartTime,
			EndTime:   r.EndTime,
			TRES:      r.TRES,
		}
		if !r.StartTime.IsZero() && !r.EndTime.IsZero() {
			row.Duration = r.EndTime.Sub(r.StartTime)
		}
		switch {
		case !r.StartTime.IsZero() && now.Before(r.StartTime):
			row.State = "upcoming"
			row.StartsIn = r.StartTime.Sub(now)
		case !r.EndTime.IsZero() && now.After(r.EndTime):
			row.State = "ended"
		default:
			row.State = "active"
			if !r.EndTime.IsZero() {
				row.EndsIn = r.EndTime.Sub(now)
			}
		}
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		order := func(s string) int {
			switch s {
			case "active":
				return 0
			case "upcoming":
				return 1
			default:
				return 2
			}
		}
		oi, oj := order(out[i].State), order(out[j].State)
		if oi != oj {
			return oi < oj
		}
		return out[i].StartTime.Before(out[j].StartTime)
	})
	return out
}
