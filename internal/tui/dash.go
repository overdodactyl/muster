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
		cursor:      map[tabIdx]int{},
		rowCounts:   map[tabIdx]int{},
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
	meMode    bool

	filter      string
	filterMode  bool
	filterInput textinput.Model

	viewport      viewport.Model
	viewportReady bool

	cursor    map[tabIdx]int
	rowCounts map[tabIdx]int

	// Cache of the most-recently-rendered slice per tab, used by the
	// Enter-to-drill detail overlay to look up the selected row without
	// re-running the filter/sort pipeline.
	lastPartitions []aggregate.PartitionSummary
	lastNodes      []aggregate.NodeRow
	lastJobs       []aggregate.JobRow
	lastUsers      []aggregate.UserRollup
	lastQueue      []aggregate.QueueRow
	lastHistory    []aggregate.HistoryRow

	detailOpen  bool
	detailTitle string
	detailBody  string

	confirmCancelID    int64
	confirmCancelName  string
	confirmCancelOwner string
	confirmMode        bool

	cancelFlash    string
	cancelFlashErr bool

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
		// Filter input swallows keys when active. Filter is applied live as
		// the user types; Enter just exits the input (filter stays), Esc
		// clears the filter and exits.
		if m.filterMode {
			switch msg.String() {
			case "esc":
				m.filter = ""
				m.filterInput.SetValue("")
				m.filterInput.Blur()
				m.filterMode = false
				return m, nil
			case "enter":
				m.filterMode = false
				m.filterInput.Blur()
				return m, nil
			}
			var cmd tea.Cmd
			m.filterInput, cmd = m.filterInput.Update(msg)
			m.filter = strings.TrimSpace(m.filterInput.Value())
			m.viewport.GotoTop()
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
		// Detail overlay swallows any key (most dismiss).
		if m.detailOpen {
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			}
			m.detailOpen = false
			return m, nil
		}
		// Confirm-cancel modal.
		if m.confirmMode {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "y", "Y":
				id := m.confirmCancelID
				m.confirmMode = false
				return m, m.cancelJobCmd(id)
			default:
				m.confirmMode = false
				return m, nil
			}
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
		case "m":
			m.meMode = !m.meMode
			m.viewport.GotoTop()
		case "j", "down":
			m.moveCursor(1)
			return m, nil
		case "k", "up":
			m.moveCursor(-1)
			return m, nil
		case "g":
			m.cursor[m.tab] = 0
			m.viewport.GotoTop()
			return m, nil
		case "G":
			m.cursor[m.tab] = m.rowCounts[m.tab] - 1
			m.ensureCursorVisible()
			return m, nil
		case "enter":
			m.openDetail()
			return m, nil
		case "c":
			m.maybeOpenConfirmCancel()
			return m, nil
		case "/":
			m.filterMode = true
			m.filterInput.SetValue(m.filter)
			m.filterInput.Focus()
			return m, textinput.Blink
		case "tab", "right", "l":
			m.tab = (m.tab + 1) % tabIdx(len(tabNames))
			m.viewport.GotoTop()
			m.clampCursor()
		case "shift+tab", "left", "h":
			m.tab = (m.tab - 1 + tabIdx(len(tabNames))) % tabIdx(len(tabNames))
			m.viewport.GotoTop()
			m.clampCursor()
		case "1":
			m.tab = tabPartitions
			m.viewport.GotoTop()
			m.clampCursor()
		case "2":
			m.tab = tabNodes
			m.viewport.GotoTop()
			m.clampCursor()
		case "3":
			m.tab = tabJobs
			m.viewport.GotoTop()
			m.clampCursor()
		case "4":
			m.tab = tabUsers
			m.viewport.GotoTop()
			m.clampCursor()
		case "5":
			m.tab = tabQueue
			m.viewport.GotoTop()
			m.clampCursor()
		case "6":
			m.tab = tabHistory
			m.viewport.GotoTop()
			m.clampCursor()
			if !m.acctLoaded && !m.acctInFlight {
				m.acctInFlight = true
				return m, m.fetchAcctCmd()
			}
		}
		// pgup/pgdn/home/end still go to viewport for paging.
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

	case cancelResultMsg:
		if msg.err != nil {
			m.cancelFlash = fmt.Sprintf("failed to cancel job %d: %s", msg.id, msg.err.Error())
			m.cancelFlashErr = true
		} else {
			m.cancelFlash = fmt.Sprintf("cancelled job %d", msg.id)
			m.cancelFlashErr = false
		}
		// Immediately refresh jobs so the table updates.
		return m, m.fetchJobsCmd()
	}

	return m, nil
}

type cancelResultMsg struct {
	id  int64
	err error
}

func (m *model) cancelJobCmd(id int64) tea.Cmd {
	c := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := c.Cancel(ctx, id)
		return cancelResultMsg{id: id, err: err}
	}
}

// maybeOpenConfirmCancel checks if the cursor is on a cancellable job row
// (Jobs or Queue tab) AND that the job belongs to the current user. If both
// are true, it opens a y/N confirmation modal.
func (m *model) maybeOpenConfirmCancel() {
	cur := m.cursor[m.tab]
	var id int64
	var user string
	var name string
	switch m.tab {
	case tabJobs:
		if cur >= 0 && cur < len(m.lastJobs) {
			id = m.lastJobs[cur].JobID
			user = m.lastJobs[cur].User
			name = m.lastJobs[cur].Name
		}
	case tabQueue:
		if cur >= 0 && cur < len(m.lastQueue) {
			id = m.lastQueue[cur].JobID
			user = m.lastQueue[cur].User
			name = m.lastQueue[cur].Name
		}
	default:
		return
	}
	if id == 0 {
		return
	}
	if me := currentUser(); user != me {
		m.cancelFlash = fmt.Sprintf("refused: job %d belongs to %s, not %s", id, user, me)
		m.cancelFlashErr = true
		return
	}
	m.confirmCancelID = id
	m.confirmCancelOwner = user
	m.confirmCancelName = name
	m.confirmMode = true
}

// maybeFinishLoading flips m.loading off once both nodes and jobs have landed,
// and records a sparkline sample.
func (m *model) maybeFinishLoading() {
	if m.nodesLoaded && m.jobsLoaded {
		m.loading = false
		m.recordSample()
	}
}

func (m *model) moveCursor(delta int) {
	m.cursor[m.tab] += delta
	m.clampCursor()
	m.ensureCursorVisible()
}

func (m *model) clampCursor() {
	n := m.rowCounts[m.tab]
	if m.cursor[m.tab] < 0 || n == 0 {
		m.cursor[m.tab] = 0
	}
	if n > 0 && m.cursor[m.tab] >= n {
		m.cursor[m.tab] = n - 1
	}
}

// tableHeaderLines is the number of lines before the first data row in a
// go-pretty rounded-border table: top border + header + header separator.
const tableHeaderLines = 3

func (m *model) ensureCursorVisible() {
	n := m.rowCounts[m.tab]
	if n == 0 || m.viewport.Height == 0 {
		return
	}
	bodyLine := tableHeaderLines + m.cursor[m.tab]
	top := m.viewport.YOffset
	// One row below the bottom edge of what's visible (exclusive).
	visibleHeight := m.viewport.Height
	bottom := top + visibleHeight
	switch {
	case bodyLine < top:
		m.viewport.SetYOffset(bodyLine - 1)
	case bodyLine >= bottom:
		m.viewport.SetYOffset(bodyLine - visibleHeight + 1)
	}
	if m.viewport.YOffset < 0 {
		m.viewport.SetYOffset(0)
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
	case m.detailOpen:
		content = m.renderDetailOverlay(contentHeight)
	case m.confirmMode:
		content = m.renderConfirmCancel(contentHeight)
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

func (m *model) renderConfirmCancel(maxHeight int) string {
	body := strings.Join([]string{
		helpTitleStyle.Render(fmt.Sprintf("Cancel job %d?", m.confirmCancelID)),
		"",
		fmt.Sprintf("  %s  %s", render.ColorFaint("user:"), m.confirmCancelOwner),
		fmt.Sprintf("  %s  %s", render.ColorFaint("name:"), m.confirmCancelName),
		"",
		"  " + render.ColorYellow("y") + " confirm cancel    " + render.ColorYellow("n / esc") + " abort",
	}, "\n")
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("196")).
		Padding(1, 3)
	return lipgloss.Place(m.width, maxHeight, lipgloss.Center, lipgloss.Center, style.Render(body))
}

func (m *model) renderDetailOverlay(maxHeight int) string {
	body := strings.Join([]string{
		helpTitleStyle.Render(m.detailTitle),
		"",
		m.detailBody,
		"",
		render.ColorFaint("press any key to return"),
	}, "\n")
	card := helpCardStyle.Render(body)
	return lipgloss.Place(m.width, maxHeight, lipgloss.Center, lipgloss.Center, card)
}

func (m *model) openDetail() {
	cur := m.cursor[m.tab]
	if cur < 0 || cur >= m.rowCounts[m.tab] {
		return
	}

	switch m.tab {
	case tabPartitions:
		if cur < len(m.lastPartitions) {
			p := m.lastPartitions[cur]
			m.detailTitle = "Partition " + p.Name
			m.detailBody = renderPartitionDetail(p)
			m.detailOpen = true
		}
	case tabNodes:
		if cur < len(m.lastNodes) {
			n := m.lastNodes[cur]
			m.detailTitle = "Node " + n.Name
			m.detailBody = renderNodeDetail(n)
			m.detailOpen = true
		}
	case tabJobs:
		if cur < len(m.lastJobs) {
			j := m.lastJobs[cur]
			m.detailTitle = fmt.Sprintf("Job %d  %s / %s", j.JobID, j.User, j.Name)
			m.detailBody = renderJobDetail(j)
			m.detailOpen = true
		}
	case tabUsers:
		if cur < len(m.lastUsers) {
			u := m.lastUsers[cur]
			m.detailTitle = "User " + u.User
			m.detailBody = renderUserDetail(u)
			m.detailOpen = true
		}
	case tabQueue:
		if cur < len(m.lastQueue) {
			q := m.lastQueue[cur]
			m.detailTitle = fmt.Sprintf("Pending job %d  %s / %s", q.JobID, q.User, q.Name)
			m.detailBody = renderQueueDetail(q)
			m.detailOpen = true
		}
	case tabHistory:
		if cur < len(m.lastHistory) {
			h := m.lastHistory[cur]
			m.detailTitle = "History  " + h.Key
			m.detailBody = renderHistoryDetail(h)
			m.detailOpen = true
		}
	}
}

func detailLine(label, value string) string {
	return fmt.Sprintf("  %-14s  %s", render.ColorFaint(label), value)
}

func renderPartitionDetail(p aggregate.PartitionSummary) string {
	lines := []string{
		detailLine("nodes", fmt.Sprintf("%d total (%d idle, %d mixed, %d alloc, %d down/drain)",
			p.TotalNodes, p.NodeCounts.Idle, p.NodeCounts.Mixed, p.NodeCounts.Alloc, p.NodeCounts.Down+p.NodeCounts.Drain)),
		detailLine("cpus", fmt.Sprintf("%d alloc / %d total", p.AllocCPUs, p.TotalCPUs)),
	}
	if p.TotalGPUs > 0 {
		gpu := fmt.Sprintf("%d / %d", p.AllocGPUs, p.TotalGPUs)
		if p.GPUModel != "" {
			gpu += " " + p.GPUModel
		}
		lines = append(lines, detailLine("gpus", gpu))
	}
	lines = append(lines, detailLine("memory", fmt.Sprintf("%s alloc / %s total", render.HumanMB(p.AllocMemMB), render.HumanMB(p.TotalMemMB))))
	lines = append(lines, detailLine("jobs", fmt.Sprintf("%d running, %d pending", p.RunningJobs, p.PendingJobs)))
	return strings.Join(lines, "\n")
}

func renderNodeDetail(n aggregate.NodeRow) string {
	users := strings.Join(n.Users, ", ")
	if users == "" {
		users = "(none)"
	}
	lines := []string{
		detailLine("partition", n.Partition),
		detailLine("state", strings.Join(n.State, ", ")),
		detailLine("cpus", fmt.Sprintf("%d alloc / %d idle / %d total", n.CPUsAlloc, n.CPUsIdle, n.CPUsTotal)),
		detailLine("memory", fmt.Sprintf("%s used / %s total (%s free)", render.HumanMB(n.MemAllocMB), render.HumanMB(n.MemTotalMB), render.HumanMB(n.MemFreeMB))),
	}
	if n.GPUsTotal > 0 {
		gpu := fmt.Sprintf("%d / %d", n.GPUsAlloc, n.GPUsTotal)
		if n.GPUModel != "" {
			gpu += " " + n.GPUModel
		}
		lines = append(lines, detailLine("gpus", gpu))
	}
	lines = append(lines, detailLine("users", users))
	if n.Reason != "" {
		lines = append(lines, detailLine("reason", n.Reason))
	}
	return strings.Join(lines, "\n")
}

func renderJobDetail(j aggregate.JobRow) string {
	lines := []string{
		detailLine("state", j.State),
		detailLine("account", j.Account),
		detailLine("partition", j.Partition),
		detailLine("nodes", orDefault(j.Nodes, "(not yet allocated)")),
		detailLine("cpus", fmt.Sprintf("%d", j.CPUs)),
	}
	if j.GPUs > 0 {
		lines = append(lines, detailLine("gpus", fmt.Sprintf("%d", j.GPUs)))
	}
	lines = append(lines,
		detailLine("memory", render.HumanMB(j.MemoryMB)),
		detailLine("runtime", render.HumanDuration(j.Runtime)),
		detailLine("time limit", render.HumanDuration(j.TimeLimit)),
	)
	return strings.Join(lines, "\n")
}

func renderUserDetail(u aggregate.UserRollup) string {
	return strings.Join([]string{
		detailLine("running", fmt.Sprintf("%d jobs", u.Running)),
		detailLine("pending", fmt.Sprintf("%d jobs", u.Pending)),
		detailLine("cpus held", fmt.Sprintf("%d", u.CPUsHeld)),
		detailLine("gpus held", fmt.Sprintf("%d", u.GPUsHeld)),
		detailLine("memory", render.HumanMB(u.MemoryMBHeld)),
		detailLine("oldest run", render.HumanDuration(u.OldestRunAge)),
	}, "\n")
}

func renderQueueDetail(q aggregate.QueueRow) string {
	lines := []string{
		detailLine("partition", q.Partition),
		detailLine("priority", fmt.Sprintf("%d", q.Priority)),
		detailLine("cpus", fmt.Sprintf("%d", q.CPUs)),
	}
	if q.GPUs > 0 {
		lines = append(lines, detailLine("gpus", fmt.Sprintf("%d", q.GPUs)))
	}
	lines = append(lines,
		detailLine("memory", render.HumanMB(q.MemoryMB)),
		detailLine("time limit", render.HumanDuration(q.TimeLimit)),
		detailLine("reason", q.Reason),
		detailLine("explained", q.ReasonHuman),
	)
	if !q.SubmitTime.IsZero() {
		lines = append(lines, detailLine("submitted", q.SubmitTime.Format("2006-01-02 15:04")+" ("+render.HumanDuration(q.SubmitAge)+" ago)"))
	}
	if !q.EligibleStart.IsZero() {
		lines = append(lines, detailLine("eligible", q.EligibleStart.Format("2006-01-02 15:04")))
	}
	return strings.Join(lines, "\n")
}

func renderHistoryDetail(h aggregate.HistoryRow) string {
	return strings.Join([]string{
		detailLine("jobs", fmt.Sprintf("%d total", h.Jobs)),
		detailLine("completed", fmt.Sprintf("%d", h.Completed)),
		detailLine("failed", fmt.Sprintf("%d", h.Failed)),
		detailLine("timeout", fmt.Sprintf("%d", h.Timeout)),
		detailLine("cancelled", fmt.Sprintf("%d", h.Cancelled)),
		detailLine("cpu-hours", fmt.Sprintf("%.1f", h.CPUHours)),
		detailLine("gpu-hours", fmt.Sprintf("%.1f", h.GPUHours)),
	}, "\n")
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
		"  " + helpKeyStyle.Render("m") + "           toggle Me mode (your jobs only)",
		"  " + helpKeyStyle.Render("enter") + "       open detail for the selected row",
		"  " + helpKeyStyle.Render("c") + "           cancel selected job (own jobs only)",
		"  " + helpKeyStyle.Render("↑ / ↓ / k / j") + "  move selection cursor",
		"  " + helpKeyStyle.Render("g / G") + "       jump to first / last row",
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

	me := ""
	if m.meMode {
		me = currentUser()
	}
	hasUser := func(target string, users []string) bool {
		if target == "" {
			return true
		}
		for _, u := range users {
			if u == target {
				return true
			}
		}
		return false
	}

	rowCount := 0
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
		m.lastPartitions = rows
		rowCount = len(rows)
		render.RenderPartitions(&buf, rows)
	case tabNodes:
		rows := aggregate.Nodes(m.nodes, m.jobs, m.partition, nil, false, false)
		filtered := rows[:0]
		for _, r := range rows {
			if me != "" && !hasUser(me, r.Users) {
				continue
			}
			if !contains(append([]string{r.Name}, r.Users...)...) {
				continue
			}
			filtered = append(filtered, r)
		}
		m.lastNodes = filtered
		rowCount = len(filtered)
		render.RenderNodes(&buf, filtered, false)
	case tabJobs:
		sort := orDefault(m.currentSort(), "cpus")
		rows := aggregate.Jobs(m.jobs, m.partition, me, false, sort, 0, time.Now())
		if f != "" {
			filtered := rows[:0]
			for _, r := range rows {
				if contains(r.Name, r.User, r.Account, r.Nodes) {
					filtered = append(filtered, r)
				}
			}
			rows = filtered
		}
		m.lastJobs = rows
		rowCount = len(rows)
		render.RenderJobs(&buf, rows)
	case tabUsers:
		sort := orDefault(m.currentSort(), "cpus")
		rows := aggregate.Users(m.jobs, m.partition, me, sort, 0, time.Now())
		if f != "" {
			filtered := rows[:0]
			for _, r := range rows {
				if contains(r.User) {
					filtered = append(filtered, r)
				}
			}
			rows = filtered
		}
		m.lastUsers = rows
		rowCount = len(rows)
		render.RenderUsers(&buf, rows)
	case tabQueue:
		sort := orDefault(m.currentSort(), "priority")
		rows := aggregate.Queue(m.jobs, m.partition, false, "", sort, time.Now())
		filtered := rows[:0]
		for _, r := range rows {
			if me != "" && r.User != me {
				continue
			}
			if !contains(r.User, r.Name, r.Reason, r.ReasonHuman) {
				continue
			}
			filtered = append(filtered, r)
		}
		m.lastQueue = filtered
		rowCount = len(filtered)
		render.RenderQueue(&buf, filtered)
	case tabHistory:
		rows := aggregate.History(m.acct, "user", m.partition, nil)
		filtered := rows[:0]
		for _, r := range rows {
			if me != "" && r.Key != me {
				continue
			}
			if !contains(r.Key) {
				continue
			}
			filtered = append(filtered, r)
		}
		m.lastHistory = filtered
		rowCount = len(filtered)
		render.RenderHistory(&buf, filtered, "user")
	}

	m.rowCounts[m.tab] = rowCount
	m.clampCursor()

	body := strings.TrimRight(buf.String(), "\n")
	if rowCount > 0 {
		body = highlightDataRow(body, m.cursor[m.tab])
	}
	return body
}

// highlightDataRow draws a soft-background tint plus a leading ▶ marker on
// the Nth data row of a go-pretty rendered table (k9s / lazygit pattern).
// All other lines get a leading space so the table stays horizontally aligned
// across the cursor and non-cursor rows.
//
// Header is the first `│`-bearing line; border lines (╭ ├ ╰) lack `│` and are
// shifted right by a space too.
func highlightDataRow(body string, rowIdx int) string {
	lines := strings.Split(body, "\n")
	dataIdx := -1
	for i, line := range lines {
		if !strings.Contains(line, "│") {
			// border line — just shift right by one space for alignment
			lines[i] = " " + line
			continue
		}
		if dataIdx == -1 {
			lines[i] = " " + line // header line
			dataIdx = 0
			continue
		}
		if dataIdx == rowIdx {
			lines[i] = cursorRowStyle.Render(cursorMarker + line)
		} else {
			lines[i] = " " + line
		}
		dataIdx++
	}
	return strings.Join(lines, "\n")
}

var (
	cursorRowStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("238"))
	cursorMarker = lipgloss.NewStyle().
			Foreground(lipgloss.Color("14")).
			Bold(true).
			Render("▶")
)

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}


func (m *model) renderFooter() string {
	if m.filterMode {
		hint := footerStyle.Render(" enter done · esc clear")
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
	help := "? help · q quit · r refresh · tab · s sort · / filter · m me · c cancel"
	if m.meMode {
		help += "  ·  " + render.ColorYellow("Me mode")
	}
	if m.filter != "" {
		help += "  ·  " + render.ColorYellow("filter: "+m.filter)
	}
	if m.cancelFlash != "" {
		flashed := m.cancelFlash
		if m.cancelFlashErr {
			flashed = render.ColorRed(flashed)
		} else {
			flashed = render.ColorGreen(flashed)
		}
		help += "  ·  " + flashed
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
)
