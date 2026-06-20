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

	loading   bool
	lastErr   error
	lastFetch time.Time
}

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
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "r":
			m.loading = true
			return m, m.fetchCmd(true)
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
	footer := m.renderFooter()

	contentHeight := m.height - lipgloss.Height(header) - lipgloss.Height(footer) - 1
	if contentHeight < 5 {
		contentHeight = 5
	}
	content := m.renderTab(contentHeight)

	return lipgloss.JoinVertical(lipgloss.Top, header, content, footer)
}

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
		rows := aggregate.Jobs(m.jobs, m.partition, "", false, "cpus", 0, time.Now())
		render.RenderJobs(&buf, rows)
	case tabUsers:
		rows := aggregate.Users(m.jobs, m.partition, "", "cpus", 0, time.Now())
		render.RenderUsers(&buf, rows)
	case tabQueue:
		rows := aggregate.Queue(m.jobs, m.partition, false, "", "priority", time.Now())
		render.RenderQueue(&buf, rows)
	case tabHistory:
		rows := aggregate.History(m.acct, "user", m.partition, nil)
		render.RenderHistory(&buf, rows, "user")
	}
	body := strings.TrimRight(buf.String(), "\n")
	body = clipHeight(body, maxHeight)
	return contentStyle.Render(body)
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
	help := "q quit · r refresh · tab/⇧tab switch · 1-6 jump"
	left := footerStyle.Render(help)
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
