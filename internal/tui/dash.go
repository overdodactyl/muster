// Package tui implements muster's interactive bubbletea dashboard.
package tui

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
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
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.Placeholder = "filter (name, user, reason…)"
	ti.CharLimit = 80
	ti.Width = 40

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))

	m := &model{
		client:      client,
		partition:   partition,
		loading:     true,
		filterInput: ti,
		spinner:     sp,
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

	nodesLoaded bool
	jobsLoaded  bool
	acctLoaded  bool
	acctInFlight bool

	history []historySample

	sortIndex map[tabIdx]int
	showHelp  bool

	filter      string
	filterMode  bool
	filterInput textinput.Model

	viewport      viewport.Model
	viewportReady bool

	spinner spinner.Model

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

// Fetch results: one message per Slurm resource. tea.Batch starts each
// underlying cobra in its own goroutine, so nodes/jobs/acct run in parallel.
type nodesMsg struct {
	nodes []slurm.Node
	err   error
	when  time.Time
}
type jobsMsg struct {
	jobs []slurm.Job
	err  error
	when time.Time
}
type acctMsg struct {
	acct []slurm.AcctJob
	err  error
	when time.Time
}

type tickMsg time.Time

func (m *model) Init() tea.Cmd {
	// Defer sacct until the user lands on the History tab or until the
	// periodic refresh (~1 min in) - it's the slow one.
	return tea.Batch(m.fetchNodesCmd(), m.fetchJobsCmd(), tickEvery(), m.spinner.Tick)
}

func tickEvery() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *model) fetchNodesCmd() tea.Cmd {
	c := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		n, err := c.Nodes(ctx)
		return nodesMsg{nodes: n, err: err, when: time.Now()}
	}
}

func (m *model) fetchJobsCmd() tea.Cmd {
	c := m.client
	p := m.partition
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		j, err := c.Jobs(ctx, p)
		return jobsMsg{jobs: j, err: err, when: time.Now()}
	}
}

func (m *model) fetchAcctCmd() tea.Cmd {
	c := m.client
	p := m.partition
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		a, err := c.Accounting(ctx, defaultHistoryWindow, p)
		return acctMsg{acct: a, err: err, when: time.Now()}
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeViewport()
		return m, nil

	case tea.KeyMsg:
		// Filter input swallows keys when active.
		if m.filterMode {
			switch msg.String() {
			case "esc":
				m.filterMode = false
				m.filterInput.SetValue(m.filter)
				m.filterInput.Blur()
				return m, nil
			case "enter":
				m.filterMode = false
				m.filter = strings.TrimSpace(m.filterInput.Value())
				m.filterInput.Blur()
				return m, nil
			}
			var cmd tea.Cmd
			m.filterInput, cmd = m.filterInput.Update(msg)
			return m, cmd
		}
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
			cmds := []tea.Cmd{m.fetchNodesCmd(), m.fetchJobsCmd()}
			if m.acctLoaded || m.tab == tabHistory {
				m.acctInFlight = true
				cmds = append(cmds, m.fetchAcctCmd())
			}
			return m, tea.Batch(cmds...)
		case "s":
			m.cycleSort()
		case "/":
			m.filterMode = true
			m.filterInput.SetValue(m.filter)
			m.filterInput.Focus()
			return m, textinput.Blink
		case "tab", "right", "l":
			m.tab = (m.tab + 1) % tabIdx(len(tabNames))
			m.viewport.GotoTop()
		case "shift+tab", "left", "h":
			m.tab = (m.tab - 1 + tabIdx(len(tabNames))) % tabIdx(len(tabNames))
			m.viewport.GotoTop()
		case "1":
			m.tab = tabPartitions
			m.viewport.GotoTop()
		case "2":
			m.tab = tabNodes
			m.viewport.GotoTop()
		case "3":
			m.tab = tabJobs
			m.viewport.GotoTop()
		case "4":
			m.tab = tabUsers
			m.viewport.GotoTop()
		case "5":
			m.tab = tabQueue
			m.viewport.GotoTop()
		case "6":
			m.tab = tabHistory
			m.viewport.GotoTop()
			if !m.acctLoaded && !m.acctInFlight {
				m.acctInFlight = true
				return m, m.fetchAcctCmd()
			}
		}
		// Delegate remaining keys (j/k/up/down/pgup/pgdn/home/end) to viewport.
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case nodesMsg:
		m.nodes = msg.nodes
		m.nodesLoaded = true
		if msg.err != nil {
			m.lastErr = msg.err
		} else {
			m.lastErr = nil
		}
		m.lastFetch = msg.when
		m.maybeFinishLoading()

	case jobsMsg:
		m.jobs = msg.jobs
		m.jobsLoaded = true
		if msg.err != nil {
			m.lastErr = msg.err
		} else {
			m.lastErr = nil
		}
		m.lastFetch = msg.when
		m.maybeFinishLoading()

	case acctMsg:
		m.acct = msg.acct
		m.acctLoaded = true
		m.acctInFlight = false
		if msg.err != nil {
			m.lastErr = msg.err
		}
		m.lastFetch = msg.when

	case tickMsg:
		m.ticks++
		m.loading = true
		cmds := []tea.Cmd{m.fetchNodesCmd(), m.fetchJobsCmd(), tickEvery()}
		// Periodic sacct refresh - only if user has already opened the
		// History tab, otherwise it's wasted work.
		if m.acctLoaded && m.ticks%historyRefreshEvery == 0 && !m.acctInFlight {
			m.acctInFlight = true
			cmds = append(cmds, m.fetchAcctCmd())
		}
		return m, tea.Batch(cmds...)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

// maybeFinishLoading flips m.loading off once both nodes and jobs have landed,
// and records a sparkline sample.
func (m *model) maybeFinishLoading() {
	if m.nodesLoaded && m.jobsLoaded {
		m.loading = false
		m.recordSample()
	}
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
	switch {
	case m.showHelp:
		content = m.renderHelp(contentHeight)
	case !m.nodesLoaded || !m.jobsLoaded:
		content = m.renderLoading(contentHeight)
	case m.tab == tabHistory && !m.acctLoaded:
		content = m.renderLoadingHistory(contentHeight)
	default:
		body := m.renderTabBody()
		m.viewport.Width = m.width
		m.viewport.Height = contentHeight
		m.viewport.SetContent(body)
		content = m.viewport.View()
		if !m.viewport.AtBottom() || m.viewport.YOffset > 0 {
			content = lipgloss.JoinVertical(lipgloss.Left, content, m.scrollHint())
		}
	}

	return lipgloss.JoinVertical(lipgloss.Top, header, summary, content, footer)
}

func (m *model) renderLoading(maxHeight int) string {
	what := []string{}
	if !m.nodesLoaded {
		what = append(what, "nodes (scontrol)")
	}
	if !m.jobsLoaded {
		what = append(what, "jobs (squeue)")
	}
	what = append(what, "")
	what = append(what, render.ColorFaint("first launch can take a few seconds"))

	body := strings.Join([]string{
		m.spinner.View() + "  " + lipgloss.NewStyle().Bold(true).Render("loading cluster state…"),
		"",
		render.ColorFaint("  • " + strings.Join(what[:len(what)-2], "\n  • ")),
		"",
		what[len(what)-1],
	}, "\n")
	return lipgloss.Place(m.width, maxHeight, lipgloss.Center, lipgloss.Center, body)
}

func (m *model) renderLoadingHistory(maxHeight int) string {
	body := strings.Join([]string{
		m.spinner.View() + "  " + lipgloss.NewStyle().Bold(true).Render("loading accounting history…"),
		"",
		render.ColorFaint("sacct --since 24h is the slowest call; takes 5-15s on a busy cluster"),
	}, "\n")
	return lipgloss.Place(m.width, maxHeight, lipgloss.Center, lipgloss.Center, body)
}

func (m *model) scrollHint() string {
	total := m.viewport.TotalLineCount()
	visible := m.viewport.VisibleLineCount()
	bottomLine := m.viewport.YOffset + visible
	if bottomLine > total {
		bottomLine = total
	}
	return render.ColorFaint(fmt.Sprintf("↑/↓ to scroll · lines %d-%d of %d", m.viewport.YOffset+1, bottomLine, total))
}

func (m *model) resizeViewport() {
	if !m.viewportReady {
		m.viewport = viewport.New(m.width, 10)
		m.viewportReady = true
	}
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
		"  " + helpKeyStyle.Render("/") + "           filter rows (name/user/reason)",
		"  " + helpKeyStyle.Render("↑ / ↓ / k / j") + "  scroll one line",
		"  " + helpKeyStyle.Render("pgup / pgdn") + " scroll one page",
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

// renderTabBody produces the full unscrolled table for the active tab.
// The caller wraps this in a viewport for scrolling.
func (m *model) renderTabBody() string {
	var buf bytes.Buffer
	f := strings.ToLower(m.filter)
	contains := func(parts ...string) bool {
		if f == "" {
			return true
		}
		for _, p := range parts {
			if strings.Contains(strings.ToLower(p), f) {
				return true
			}
		}
		return false
	}

	switch m.tab {
	case tabPartitions:
		rows := aggregate.Partitions(m.nodes, m.jobs, m.partition)
		if f != "" {
			filtered := rows[:0]
			for _, r := range rows {
				if contains(r.Name) {
					filtered = append(filtered, r)
				}
			}
			rows = filtered
		}
		render.RenderPartitions(&buf, rows)
	case tabNodes:
		rows := aggregate.Nodes(m.nodes, m.jobs, m.partition, nil, false, false)
		if f != "" {
			filtered := rows[:0]
			for _, r := range rows {
				if contains(append([]string{r.Name}, r.Users...)...) {
					filtered = append(filtered, r)
				}
			}
			rows = filtered
		}
		render.RenderNodes(&buf, rows, false)
	case tabJobs:
		sort := orDefault(m.currentSort(), "cpus")
		rows := aggregate.Jobs(m.jobs, m.partition, "", false, sort, 0, time.Now())
		if f != "" {
			filtered := rows[:0]
			for _, r := range rows {
				if contains(r.Name, r.User, r.Account, r.Nodes) {
					filtered = append(filtered, r)
				}
			}
			rows = filtered
		}
		render.RenderJobs(&buf, rows)
	case tabUsers:
		sort := orDefault(m.currentSort(), "cpus")
		rows := aggregate.Users(m.jobs, m.partition, "", sort, 0, time.Now())
		if f != "" {
			filtered := rows[:0]
			for _, r := range rows {
				if contains(r.User) {
					filtered = append(filtered, r)
				}
			}
			rows = filtered
		}
		render.RenderUsers(&buf, rows)
	case tabQueue:
		sort := orDefault(m.currentSort(), "priority")
		rows := aggregate.Queue(m.jobs, m.partition, false, "", sort, time.Now())
		if f != "" {
			filtered := rows[:0]
			for _, r := range rows {
				if contains(r.User, r.Name, r.Reason, r.ReasonHuman) {
					filtered = append(filtered, r)
				}
			}
			rows = filtered
		}
		render.RenderQueue(&buf, rows)
	case tabHistory:
		rows := aggregate.History(m.acct, "user", m.partition, nil)
		if f != "" {
			filtered := rows[:0]
			for _, r := range rows {
				if contains(r.Key) {
					filtered = append(filtered, r)
				}
			}
			rows = filtered
		}
		render.RenderHistory(&buf, rows, "user")
	}
	body := strings.TrimRight(buf.String(), "\n")
	return contentStyle.Render(body)
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}


func (m *model) renderFooter() string {
	if m.filterMode {
		hint := footerStyle.Render(" enter apply · esc cancel")
		return m.filterInput.View() + hint
	}

	status := "ready"
	if m.loading {
		status = "loading…"
	} else if m.lastErr != nil {
		status = "error: " + m.lastErr.Error()
	} else if !m.lastFetch.IsZero() {
		status = fmt.Sprintf("last update %s", m.lastFetch.Format("15:04:05"))
	}
	help := "? help · q quit · r refresh · tab switch · s sort · / filter"
	if m.filter != "" {
		help += "  ·  " + render.ColorYellow("filter: "+m.filter)
	}
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
