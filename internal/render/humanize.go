package render

import (
	"fmt"
	"time"
)

// HumanMB formats megabytes as "12G", "256G", "1.5T".
func HumanMB(mb int) string {
	if mb <= 0 {
		return "0"
	}
	g := float64(mb) / 1024.0
	if g < 1024 {
		if g >= 100 {
			return fmt.Sprintf("%.0fG", g)
		}
		return fmt.Sprintf("%.1fG", g)
	}
	return fmt.Sprintf("%.1fT", g/1024.0)
}

// HumanDuration formats a duration as "1d2h", "5h12m", "12m", "45s".
func HumanDuration(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		m := int(d.Minutes()) - h*60
		if m == 0 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh%dm", h, m)
	}
	days := int(d.Hours()) / 24
	h := int(d.Hours()) - days*24
	if h == 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dd%dh", days, h)
}
