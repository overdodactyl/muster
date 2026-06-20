// Package tui implements muster's interactive bubbletea dashboard.
package tui

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"muster/internal/aggregate"
	"muster/internal/render"
	"muster/internal/slurm"
)

const (
	refreshInterval    = 10 * time.Second
	historyRefreshEvery = 6 // refresh sacct every Nth tick (sacct is slow)
	defaultHistoryWindow = 24 * time.Hour
)

type tabIdx int

const (
	tabPartitions tabIdx = iota
	tabNodes
	tabJobs
	tabUsers
	tabQueue
	tabHistory
)

var tabNames = []string{"Partitions", "Nodes", "Jobs", "Users", "Queue", "History"}

// Run blocks until the user quits the TUI.
func Run(client slurm.Client, partition string) error {
	m := &model{
		client:    client,
		partition: partition,
		loading:   true,
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

type model struct {
	client    slurm.Client
	partition string

	tab    tabIdx
	width  int
	height int
	ticks  int

	nodes []slurm.Node
	jobs  []slurm.Job
	acct  []slurm.AcctJob

	history []historySample

	sortIndex map[tabIdx]int
	showHelp  bool

	loading   bool
	lastErr   error
	lastFetch time.Time
}

// sortOptions defines which sort keys cycle on `s` for each tab. Tabs not
// listed (Partitions, Nodes, History) have a single fixed sort.
var sortOptions = map[tabIdx][]string{
	tabJobs:   {"cpus", "gpus", "mem", "runtime", "user"},
	tabUsers:  {"cpus", "gpus", "mem", "jobs", "age"},
	tabQueue:  {"priority", "age", "user"},
}

func (m *model) currentSort() string {
	opts := sortOptions[m.tab]
	if len(opts) == 0 {
		return ""
	}
	if m.sortIndex == nil {
		return opts[0]
	}
	return opts[m.sortIndex[m.tab]%len(opts)]
}

func (m *model) cycleSort() {
	opts := sortOptions[m.tab]
	if len(opts) == 0 {
		return
	}
	if m.sortIndex == nil {
		m.sortIndex = map[tabIdx]int{}
	}
	m.sortIndex[m.tab] = (m.sortIndex[m.tab] + 1) % len(opts)
}

// historySample is a per-tick snapshot of the focused partition's
// utilization, recorded to feed the sparklines.
type historySample struct {
	when                   time.Time
	cpuPct, gpuPct, memPct int
}

const maxHistory = 60 // 60 ticks × 10s = 10 min of trend data

type dataMsg struct {
	nodes      []slurm.Node
	jobs       []slurm.Job
	acct       []slurm.AcctJob
	hasAcct    bool
	err        error
	when       time.Time
}

type tickMsg time.Time

func (m *model) Init() tea.Cmd {
	return tea.Batch(m.fetchCmd(true), tickEvery())
}

func tickEvery() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *model) fetchCmd(includeHistory bool) tea.Cmd {
	partition := m.partition
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		msg := dataMsg{when: time.Now()}

		nodes, err := client.Nodes(ctx)
		if err != nil {
			msg.err = err
			return msg
		}
		msg.nodes = nodes

		jobs, err := client.Jobs(ctx, partition)
		if err != nil {
			msg.err = err
			return msg
		}
		msg.jobs = jobs

		if includeHistory {
			acctCtx, acctCancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer acctCancel()
			if acct, err := client.Accounting(acctCtx, defaultHistoryWindow, partition); err == nil {
				msg.acct = acct
				msg.hasAcct = true
			}
		}

		return msg
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// Help overlay swallows any key (and dismisses on most keys).
		if m.showHelp {
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			}
			m.showHelp = false
			return m, nil
		}
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "?":
			m.showHelp = true
		case "r":
			m.loading = true
			return m, m.fetchCmd(true)
		case "s":
			m.cycleSort()
		case "tab", "right", "l":
			m.tab = (m.tab + 1) % tabIdx(len(tabNames))
		case "shift+tab", "left", "h":
			m.tab = (m.tab - 1 + tabIdx(len(tabNames))) % tabIdx(len(tabNames))
		case "1":
			m.tab = tabPartitions
		case "2":
			m.tab = tabNodes
		case "3":
			m.tab = tabJobs
		case "4":
			m.tab = tabUsers
		case "5":
			m.tab = tabQueue
		case "6":
			m.tab = tabHistory
		}

	case dataMsg:
		m.nodes = msg.nodes
		m.jobs = msg.jobs
		if msg.hasAcct {
			m.acct = msg.acct
		}
		m.lastErr = msg.err
		m.lastFetch = msg.when
		m.loading = false
		m.recordSample()

	case tickMsg:
		m.ticks++
		m.loading = true
		includeHistory := m.ticks%historyRefreshEvery == 0
		return m, tea.Batch(m.fetchCmd(includeHistory), tickEvery())
	}

	return m, nil
}

func (m *model) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading..."
	}

	header := m.renderHeader()
	summary := m.renderSummary(m.width)
	footer := m.renderFooter()

	used := lipgloss.Height(header) + lipgloss.Height(summary) + lipgloss.Height(footer) + 1
	contentHeight := m.height - used
	if contentHeight < 5 {
		contentHeight = 5
	}

	var content string
	if m.showHelp {
		content = m.renderHelp(contentHeight)
	} else {
		content = m.renderTab(contentHeight)
	}

	return lipgloss.JoinVertical(lipgloss.Top, header, summary, content, footer)
}

func (m *model) renderHelp(maxHeight int) string {
	body := strings.Join([]string{
		helpTitleStyle.Render("muster — keys"),
		"",
		helpSectionStyle.Render("Navigation"),
		"  " + helpKeyStyle.Render("1 – 6") + "       jump directly to tab",
		"  " + helpKeyStyle.Render("tab / ⇧tab") + "  next / previous tab",
		"  " + helpKeyStyle.Render("h / l") + "       previous / next tab",
		"",
		helpSectionStyle.Render("Actions"),
		"  " + helpKeyStyle.Render("r") + "           refresh now",
		"  " + helpKeyStyle.Render("s") + "           cycle sort key (jobs, users, queue)",
		"  " + helpKeyStyle.Render("?") + "           toggle this help",
		"",
		helpSectionStyle.Render("Exit"),
		"  " + helpKeyStyle.Render("q / esc") + "     quit",
		"",
		render.ColorFaint("press any key to return"),
	}, "\n")

	card := helpCardStyle.Render(body)
	return lipgloss.Place(m.width, maxHeight, lipgloss.Center, lipgloss.Center, card)
}

var (
	helpCardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(1, 3)
	helpTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("14"))
	helpSectionStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("245"))
	helpKeyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("11"))
)

func (m *model) renderHeader() string {
	title := titleStyle.Render(" muster ")

	var tabs []string
	for i, name := range tabNames {
		label := " " + name + " "
		if tabIdx(i) > 0 && m.partition != "" {
			label = " " + name + ":" + m.partition + " "
		}
		style := tabInactive
		if tabIdx(i) == m.tab {
			style = tabActive
		}
		tabs = append(tabs, style.Render(label))
	}
	bar := lipgloss.JoinHorizontal(lipgloss.Bottom, tabs...)
	return lipgloss.JoinHorizontal(lipgloss.Bottom, title, " ", bar)
}

func (m *model) renderTab(maxHeight int) string {
	var buf bytes.Buffer
	switch m.tab {
	case tabPartitions:
		rows := aggregate.Partitions(m.nodes, m.jobs, m.partition)
		render.RenderPartitions(&buf, rows)
	case tabNodes:
		rows := aggregate.Nodes(m.nodes, m.jobs, m.partition, nil, false, false)
		render.RenderNodes(&buf, rows, false)
	case tabJobs:
		sort := orDefault(m.currentSort(), "cpus")
		rows := aggregate.Jobs(m.jobs, m.partition, "", false, sort, 0, time.Now())
		render.RenderJobs(&buf, rows)
	case tabUsers:
		sort := orDefault(m.currentSort(), "cpus")
		rows := aggregate.Users(m.jobs, m.partition, "", sort, 0, time.Now())
		render.RenderUsers(&buf, rows)
	case tabQueue:
		sort := orDefault(m.currentSort(), "priority")
		rows := aggregate.Queue(m.jobs, m.partition, false, "", sort, time.Now())
		render.RenderQueue(&buf, rows)
	case tabHistory:
		rows := aggregate.History(m.acct, "user", m.partition, nil)
		render.RenderHistory(&buf, rows, "user")
	}
	body := strings.TrimRight(buf.String(), "\n")
	body = clipHeight(body, maxHeight)
	return contentStyle.Render(body)
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func clipHeight(s string, max int) string {
	if max <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= max {
		return s
	}
	clipped := strings.Join(lines[:max-1], "\n")
	hidden := len(lines) - (max - 1)
	return clipped + "\n" + lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf("…+%d more lines", hidden))
}

func (m *model) renderFooter() string {
	status := "ready"
	if m.loading {
		status = "loading…"
	} else if m.lastErr != nil {
		status = "error: " + m.lastErr.Error()
	} else if !m.lastFetch.IsZero() {
		status = fmt.Sprintf("last update %s", m.lastFetch.Format("15:04:05"))
	}
	help := "? help · q quit · r refresh · tab switch · s sort"
	left := footerStyle.Render(help)

	if sort := m.currentSort(); sort != "" {
		status = "sort: " + sort + "  ·  " + status
	}
	right := footerStyle.Render(status)

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("0")).
			Background(lipgloss.Color("14"))
	tabActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("63"))
	tabInactive = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Background(lipgloss.Color("236"))
	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))
	contentStyle = lipgloss.NewStyle().Padding(1, 1)
)
