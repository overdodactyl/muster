package tui

import (
	"fmt"
	"os"
	"os/user"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"muster/internal/aggregate"
	"muster/internal/render"
)

// currentUser returns the username for the "You" card. Prefers $USER for speed.
func currentUser() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return ""
}

func (m *model) renderSummary(width int) string {
	parts := aggregate.Partitions(m.nodes, m.jobs, m.partition)
	var summary aggregate.PartitionSummary
	label := "All partitions"
	if m.partition != "" {
		label = m.partition
		if len(parts) > 0 {
			summary = parts[0]
		}
	} else {
		summary = aggregateAll(parts)
	}

	left := m.renderPartCard(label, summary)
	right := m.renderUserCard()

	gap := "   "
	if right == "" {
		return left
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, left, gap, right)
}

func (m *model) renderPartCard(label string, p aggregate.PartitionSummary) string {
	cpuPct := pct(p.AllocCPUs, p.TotalCPUs)
	gpuPct := pct(p.AllocGPUs, p.TotalGPUs)
	memPct := pct(p.AllocMemMB, p.TotalMemMB)

	gpuLabel := "-"
	if p.TotalGPUs > 0 {
		model := ""
		if p.GPUModel != "" {
			model = " " + p.GPUModel
		}
		gpuLabel = fmt.Sprintf("%d/%d%s", p.AllocGPUs, p.TotalGPUs, model)
	}

	lines := []string{
		cardTitleStyle.Render(label) + cardCountStyle.Render(fmt.Sprintf("  %d nodes  •  %d running / %d pending", p.TotalNodes, p.RunningJobs, p.PendingJobs)),
		fmt.Sprintf("%s  %s  %s", padRight("CPUs", 5), padRight(fmt.Sprintf("%d/%d", p.AllocCPUs, p.TotalCPUs), 12), render.Bar(p.AllocCPUs, p.TotalCPUs, 14)+render.ColorFaint(fmt.Sprintf(" %3d%%", cpuPct))),
		fmt.Sprintf("%s  %s  %s", padRight("GPUs", 5), padRight(gpuLabel, 12), render.Bar(p.AllocGPUs, p.TotalGPUs, 14)+render.ColorFaint(fmt.Sprintf(" %3d%%", gpuPct))),
		fmt.Sprintf("%s  %s  %s", padRight("Mem", 5), padRight(fmt.Sprintf("%s/%s", render.HumanMB(p.AllocMemMB), render.HumanMB(p.TotalMemMB)), 12), render.Bar(p.AllocMemMB, p.TotalMemMB, 14)+render.ColorFaint(fmt.Sprintf(" %3d%%", memPct))),
	}
	return cardStyle.Render(strings.Join(lines, "\n"))
}

func (m *model) renderUserCard() string {
	username := currentUser()
	if username == "" {
		return ""
	}
	rollups := aggregate.Users(m.jobs, m.partition, username, "cpus", 0, time.Now())
	var r aggregate.UserRollup
	if len(rollups) > 0 {
		r = rollups[0]
	}
	r.User = username

	totalJobs := r.Running + r.Pending
	state := render.ColorFaint("idle")
	if r.Running > 0 {
		state = render.ColorGreen(fmt.Sprintf("%d running", r.Running))
		if r.Pending > 0 {
			state += render.ColorFaint(fmt.Sprintf(" + %d pending", r.Pending))
		}
	} else if r.Pending > 0 {
		state = render.ColorYellow(fmt.Sprintf("%d pending", r.Pending))
	}

	oldest := "—"
	if r.OldestRunAge > 0 {
		oldest = render.HumanDuration(r.OldestRunAge)
	}

	lines := []string{
		cardTitleStyle.Render("You: "+username) + cardCountStyle.Render(fmt.Sprintf("  %d job%s", totalJobs, plural(totalJobs))),
		fmt.Sprintf("%s  %s", padRight("State", 7), state),
		fmt.Sprintf("%s  %d CPU   %d GPU   %s mem", padRight("Holding", 7), r.CPUsHeld, r.GPUsHeld, render.HumanMB(r.MemoryMBHeld)),
		fmt.Sprintf("%s  %s", padRight("Oldest", 7), oldest),
	}
	return cardStyle.Render(strings.Join(lines, "\n"))
}

// aggregateAll sums a list of partitions into a single cluster-wide summary.
func aggregateAll(parts []aggregate.PartitionSummary) aggregate.PartitionSummary {
	var out aggregate.PartitionSummary
	gpuModels := map[string]int{}
	for _, p := range parts {
		out.TotalNodes += p.TotalNodes
		out.NodeCounts.Idle += p.NodeCounts.Idle
		out.NodeCounts.Mixed += p.NodeCounts.Mixed
		out.NodeCounts.Alloc += p.NodeCounts.Alloc
		out.NodeCounts.Down += p.NodeCounts.Down
		out.NodeCounts.Drain += p.NodeCounts.Drain
		out.AllocCPUs += p.AllocCPUs
		out.TotalCPUs += p.TotalCPUs
		out.AllocGPUs += p.AllocGPUs
		out.TotalGPUs += p.TotalGPUs
		out.AllocMemMB += p.AllocMemMB
		out.TotalMemMB += p.TotalMemMB
		out.RunningJobs += p.RunningJobs
		out.PendingJobs += p.PendingJobs
		if p.GPUModel != "" {
			gpuModels[p.GPUModel]++
		}
	}
	if len(gpuModels) == 1 {
		for k := range gpuModels {
			out.GPUModel = k
		}
	}
	return out
}

func pct(num, denom int) int {
	if denom <= 0 {
		return 0
	}
	return num * 100 / denom
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

var (
	cardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)
	cardTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("14"))
	cardCountStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))
)
