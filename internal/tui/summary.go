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

func (m *model) recordSample() {
	parts := aggregate.Partitions(m.nodes, m.jobs, m.partition)
	var p aggregate.PartitionSummary
	if m.partition != "" && len(parts) > 0 {
		p = parts[0]
	} else {
		p = aggregateAll(parts)
	}

	// Current user's share of the focused partition right now.
	var myCPU, myGPU, myMem int
	if u := currentUser(); u != "" {
		rollups := aggregate.Users(m.jobs, m.partition, u, "cpus", 0, time.Now())
		if len(rollups) > 0 {
			r := rollups[0]
			myCPU = r.CPUsHeld
			myGPU = r.GPUsHeld
			myMem = r.MemoryMBHeld
		}
	}

	m.history = append(m.history, historySample{
		when:     time.Now(),
		cpuPct:   pct(p.AllocCPUs, p.TotalCPUs),
		gpuPct:   pct(p.AllocGPUs, p.TotalGPUs),
		memPct:   pct(p.AllocMemMB, p.TotalMemMB),
		myCPUPct: pct(myCPU, p.TotalCPUs),
		myGPUPct: pct(myGPU, p.TotalGPUs),
		myMemPct: pct(myMem, p.TotalMemMB),
	})
	if len(m.history) > maxHistory {
		m.history = m.history[len(m.history)-maxHistory:]
	}
}

func (m *model) sparkSeries(get func(historySample) int) []int {
	out := make([]int, len(m.history))
	for i, s := range m.history {
		out[i] = get(s)
	}
	return out
}

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
	user := m.renderUserCard()
	gap := "  "

	// Cluster-mode (no -p): one compact card per partition, side-by-side.
	if m.partition == "" {
		if len(parts) == 0 {
			return user
		}
		cards := make([]string, 0, len(parts)+2)
		for _, p := range parts {
			cards = append(cards, renderCompactPartCard(p))
			cards = append(cards, gap)
		}
		if user != "" {
			cards = append(cards, user)
		} else if len(cards) > 0 && cards[len(cards)-1] == gap {
			cards = cards[:len(cards)-1]
		}
		return lipgloss.JoinHorizontal(lipgloss.Top, cards...)
	}

	// -p NAME mode: the full card with sparklines + user card.
	var summary aggregate.PartitionSummary
	if len(parts) > 0 {
		summary = parts[0]
	}
	left := m.renderPartCard(m.partition, summary)
	if user == "" {
		return left
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, left, gap, user)
}

// renderCompactPartCard is the cluster-mode card: no sparkline, smaller bars,
// designed to fit several side-by-side. ~26 chars wide including borders.
func renderCompactPartCard(p aggregate.PartitionSummary) string {
	cpuPct := pct(p.AllocCPUs, p.TotalCPUs)
	gpuPct := pct(p.AllocGPUs, p.TotalGPUs)
	memPct := pct(p.AllocMemMB, p.TotalMemMB)

	gpuRow := render.ColorFaint(padRight("GPU", 4)) + render.ColorFaint("  —")
	if p.TotalGPUs > 0 {
		model := ""
		if p.GPUModel != "" {
			model = " " + p.GPUModel
		}
		gpuRow = fmt.Sprintf("%s  %s %s  %s",
			padRight("GPU", 4),
			render.Bar(p.AllocGPUs, p.TotalGPUs, 8),
			fmt.Sprintf("%3d%%", gpuPct),
			render.ColorFaint(fmt.Sprintf("%d/%d%s", p.AllocGPUs, p.TotalGPUs, model)),
		)
	}

	lines := []string{
		cardTitleStyle.Render(p.Name) + cardCountStyle.Render(fmt.Sprintf("  %dn · %dR/%dPD", p.TotalNodes, p.RunningJobs, p.PendingJobs)),
		fmt.Sprintf("%s  %s %s  %s",
			padRight("CPU", 4),
			render.Bar(p.AllocCPUs, p.TotalCPUs, 8),
			fmt.Sprintf("%3d%%", cpuPct),
			render.ColorFaint(fmt.Sprintf("%d/%d", p.AllocCPUs, p.TotalCPUs)),
		),
		gpuRow,
		fmt.Sprintf("%s  %s %s  %s",
			padRight("Mem", 4),
			render.Bar(p.AllocMemMB, p.TotalMemMB, 8),
			fmt.Sprintf("%3d%%", memPct),
			render.ColorFaint(fmt.Sprintf("%s/%s", render.HumanMB(p.AllocMemMB), render.HumanMB(p.TotalMemMB))),
		),
	}
	return cardStyle.Render(strings.Join(lines, "\n"))
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

	cpuSpark := render.Sparkline(m.sparkSeries(func(s historySample) int { return s.cpuPct }), sparkWidth)
	gpuSpark := render.Sparkline(m.sparkSeries(func(s historySample) int { return s.gpuPct }), sparkWidth)
	memSpark := render.Sparkline(m.sparkSeries(func(s historySample) int { return s.memPct }), sparkWidth)

	lines := []string{
		cardTitleStyle.Render(label) + cardCountStyle.Render(fmt.Sprintf("  %d nodes  •  %d running / %d pending", p.TotalNodes, p.RunningJobs, p.PendingJobs)),
		formatMetricLine("CPUs", fmt.Sprintf("%d/%d", p.AllocCPUs, p.TotalCPUs), p.AllocCPUs, p.TotalCPUs, cpuPct, cpuSpark),
		formatMetricLine("GPUs", gpuLabel, p.AllocGPUs, p.TotalGPUs, gpuPct, gpuSpark),
		formatMetricLine("Mem", fmt.Sprintf("%s/%s", render.HumanMB(p.AllocMemMB), render.HumanMB(p.TotalMemMB)), p.AllocMemMB, p.TotalMemMB, memPct, memSpark),
	}
	return cardStyle.Render(strings.Join(lines, "\n"))
}

const sparkWidth = 30

// formatMetricLine builds one row of the partition card:
//
//	CPUs   168/352      ████▊░░░░░░░░░  47%   ▁▂▃▄▅▆▇█▇▆▅▄▃▂
func formatMetricLine(label, count string, used, total, pct int, spark string) string {
	return fmt.Sprintf("%s  %s  %s%s  %s",
		padRight(label, 5),
		padRight(count, 12),
		render.Bar(used, total, 14),
		render.ColorFaint(fmt.Sprintf(" %3d%%", pct)),
		spark,
	)
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

	const userSparkW = 16
	cpuSpark := render.Sparkline(m.sparkSeries(func(s historySample) int { return s.myCPUPct }), userSparkW)
	gpuSpark := render.Sparkline(m.sparkSeries(func(s historySample) int { return s.myGPUPct }), userSparkW)
	memSpark := render.Sparkline(m.sparkSeries(func(s historySample) int { return s.myMemPct }), userSparkW)

	lines := []string{
		cardTitleStyle.Render("You: "+username) + cardCountStyle.Render(fmt.Sprintf("  %d job%s", totalJobs, plural(totalJobs))),
		fmt.Sprintf("%s  %s", padRight("State", 7), state),
		fmt.Sprintf("%s  %d CPU %s   %d GPU %s   %s mem %s",
			padRight("Holding", 7), r.CPUsHeld, cpuSpark, r.GPUsHeld, gpuSpark, render.HumanMB(r.MemoryMBHeld), memSpark),
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
