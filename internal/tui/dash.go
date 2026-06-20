// Package tui implements muster's interactive bubbletea dashboard.
package tui

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
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
	tabAccounts
	tabQueue
	tabHistory
)

var tabNames = []string{"Partitions", "Nodes", "Jobs", "Users", "Accounts", "Queue", "History"}

// Run blocks until the user quits the TUI.
func Run(client slurm.Client, partition string) error {
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.Placeholder = "filter (name, user, reason…)"
	ti.CharLimit = 80
	ti.Width = 40

	lti := textinput.New()
	lti.Prompt = "/ "
	lti.Placeholder = "search in log (n/N jump, esc clear)"
	lti.CharLimit = 200
	lti.Width = 50

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))

	m := &model{
		client:         client,
		partition:      partition,
		loading:        true,
		filterInput:    ti,
		logSearchInput: lti,
		spinner:        sp,
		cursor:         map[tabIdx]int{},
		rowCounts:      map[tabIdx]int{},
	}
	p := tea.NewProgram(m,
		tea.WithAltScreen(),
		tea.WithMouseAllMotion(), // capture mouse incl. wheel so scrolling stays inside the dash
	)
	_, err := p.Run()
	return err
}

type model struct {
	client      slurm.Client
	partition   string
	clusterName string // populated by clusterMsg on startup

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
	lastAccounts   []aggregate.AccountRollup
	lastQueue      []aggregate.QueueRow
	lastHistory    []aggregate.HistoryRow

	detailOpen     bool
	detailTitle    string
	detailBody     string
	detailJobID    int64    // non-zero when the detail is a job/queue row
	detailLogPath  string   // resolved stdout path (for header display)
	detailCWD      string   // job's working directory
	detailCommand  string   // job's command/script path
	detailLogs     []string // captured stdout lines, refreshed each tick while open
	detailLogErr   error
	detailEff      slurm.JobEfficiency // live cpu/mem stats from sstat
	detailViewport viewport.Model // scrollable log viewer (jobs/queue only)
	detailVPReady  bool

	// Per-detail CPU history: cleared on each new openDetail; updated when
	// effMsg lands. Powers the live sparkline in the metadata block.
	detailEffSeries []int

	// History-of-job-name comparison for the open detail.
	detailHistoryRuns []slurm.AcctJob

	// nvidia-smi snapshot for the currently-detailed job (GPU jobs only),
	// refreshed every 3rd dash tick to keep srun overhead bounded.
	detailGPU []slurm.GPUUtil

	// In-log search (active only while the log viewer is open).
	logSearch       string
	logSearchMode   bool
	logSearchInput  textinput.Model
	logMatchLines   []int // wrapped-line indices that contain a match
	logMatchCursor  int   // index into logMatchLines for n/N navigation

	confirmCancelID    int64
	confirmCancelName  string
	confirmCancelOwner string
	confirmMode        bool
	bulkCancelIDs      []int64 // non-empty when confirm is a multi-select cancel

	cancelFlash    string
	cancelFlashErr bool

	spinner spinner.Model

	// Job-flash bookkeeping: lastSeenJobs is the set of IDs from the prior
	// jobsMsg; jobIsNew is the set that appeared in the most recent one. A
	// job stays "new" only until the next refresh fires.
	lastSeenJobs map[int64]bool
	jobIsNew     map[int64]bool

	// Space-toggled job selection set (shared across Jobs and Queue tabs
	// since both navigate job IDs). Cleared after a bulk operation.
	selectedJobs map[int64]bool

	// arrayDrill is non-zero when the user has pressed Enter on a collapsed
	// array row; only that array's tasks are shown (expanded). Esc clears.
	arrayDrill int64

	loading   bool
	lastErr   error
	lastFetch time.Time
}

// sortOptions defines which sort keys cycle on `s` for each tab. Tabs not
// listed (Partitions, Nodes, History) have a single fixed sort.
var sortOptions = map[tabIdx][]string{
	tabJobs:     {"cpus", "gpus", "mem", "runtime", "user"},
	tabUsers:    {"cpus", "gpus", "mem", "jobs", "age"},
	tabAccounts: {"cpus", "gpus", "mem", "jobs", "users", "name"},
	tabQueue:    {"priority", "age", "user"},
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
// utilization plus the current user's share of it. Both feed sparklines
// in the partition card and the user card respectively.
type historySample struct {
	when                         time.Time
	cpuPct, gpuPct, memPct       int // partition: alloc share of total
	myCPUPct, myGPUPct, myMemPct int // current user's share of partition total
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
	// Fire sacct in parallel with nodes/jobs at startup. It's the slow one
	// (~20s) so it lands well after the UI is interactive, but if the user
	// visits History it'll already be there. Marking acctInFlight so a
	// concurrent switchTab(tabHistory) doesn't kick off a second fetch.
	m.acctInFlight = true
	return tea.Batch(
		m.fetchNodesCmd(),
		m.fetchJobsCmd(),
		m.fetchAcctCmd(),
		m.fetchClusterCmd(),
		tickEvery(),
		m.spinner.Tick,
	)
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
		// Tell the render layer about the new width so go-pretty wraps cells
		// to fit instead of overflowing into the scrollback.
		render.SetMaxWidth(msg.Width)
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
		// Detail overlay key handling.
		if m.detailOpen {
			// Search-input mode swallows keystrokes (esc clears, enter exits).
			if m.logSearchMode {
				switch msg.String() {
				case "esc":
					m.logSearch = ""
					m.logSearchInput.SetValue("")
					m.logSearchInput.Blur()
					m.logSearchMode = false
					m.logMatchLines = nil
					m.logMatchCursor = 0
					m.updateDetailViewportContent()
					return m, nil
				case "enter":
					m.logSearchMode = false
					m.logSearchInput.Blur()
					return m, nil
				}
				var cmd tea.Cmd
				m.logSearchInput, cmd = m.logSearchInput.Update(msg)
				m.logSearch = strings.TrimSpace(m.logSearchInput.Value())
				m.logMatchCursor = 0
				m.updateDetailViewportContent()
				m.jumpToCurrentMatch()
				return m, cmd
			}
			switch msg.String() {
			case "esc", "q":
				m.detailOpen = false
				return m, nil
			case "ctrl+c":
				return m, tea.Quit
			case "/":
				if m.detailJobID > 0 {
					m.logSearchMode = true
					m.logSearchInput.SetValue(m.logSearch)
					m.logSearchInput.Focus()
					return m, textinput.Blink
				}
			case "n":
				if m.detailJobID > 0 && len(m.logMatchLines) > 0 {
					m.logMatchCursor = (m.logMatchCursor + 1) % len(m.logMatchLines)
					m.jumpToCurrentMatch()
					return m, nil
				}
			case "N":
				if m.detailJobID > 0 && len(m.logMatchLines) > 0 {
					m.logMatchCursor = (m.logMatchCursor - 1 + len(m.logMatchLines)) % len(m.logMatchLines)
					m.jumpToCurrentMatch()
					return m, nil
				}
			}
			if m.detailJobID > 0 {
				var cmd tea.Cmd
				m.detailViewport, cmd = m.detailViewport.Update(msg)
				return m, cmd
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
				m.confirmMode = false
				if len(m.bulkCancelIDs) > 0 {
					ids := m.bulkCancelIDs
					m.bulkCancelIDs = nil
					m.selectedJobs = nil
					return m, m.cancelManyCmd(ids)
				}
				id := m.confirmCancelID
				return m, m.cancelJobCmd(id)
			default:
				m.confirmMode = false
				m.bulkCancelIDs = nil
				return m, nil
			}
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc":
			// Tiered: leave array-drill first, then quit if not drilling.
			if m.arrayDrill != 0 {
				m.arrayDrill = 0
				m.viewport.GotoTop()
				return m, nil
			}
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
		case "p", "P":
			m.cyclePartition(msg.String() == "P")
			return m, nil
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
			return m, m.openDetail()
		case " ":
			m.toggleSelection()
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
			return m, m.switchTab((m.tab + 1) % tabIdx(len(tabNames)))
		case "shift+tab", "left", "h":
			return m, m.switchTab((m.tab - 1 + tabIdx(len(tabNames))) % tabIdx(len(tabNames)))
		case "1":
			return m, m.switchTab(tabPartitions)
		case "2":
			return m, m.switchTab(tabNodes)
		case "3":
			return m, m.switchTab(tabJobs)
		case "4":
			return m, m.switchTab(tabUsers)
		case "5":
			return m, m.switchTab(tabAccounts)
		case "6":
			return m, m.switchTab(tabQueue)
		case "7":
			return m, m.switchTab(tabHistory)
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
		// Compute which job IDs are new since the previous refresh BEFORE
		// overwriting m.jobs, so the next render can mark them.
		seenNow := make(map[int64]bool, len(msg.jobs))
		newIDs := map[int64]bool{}
		for _, j := range msg.jobs {
			seenNow[j.ID] = true
			if m.lastSeenJobs != nil && !m.lastSeenJobs[j.ID] {
				newIDs[j.ID] = true
			}
		}
		// On the very first refresh, nothing is "new" — only flash after we
		// have a baseline to compare against.
		if m.lastSeenJobs == nil {
			newIDs = nil
		}
		m.jobIsNew = newIDs
		m.lastSeenJobs = seenNow

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
		// Periodic sacct refresh.
		if m.acctLoaded && m.ticks%historyRefreshEvery == 0 && !m.acctInFlight {
			m.acctInFlight = true
			cmds = append(cmds, m.fetchAcctCmd())
		}
		// If a job detail overlay is open, refresh its log tail + sstat too.
		if m.detailOpen && m.detailJobID > 0 {
			cmds = append(cmds,
				m.fetchLogTailCmd(m.detailJobID),
				m.fetchEfficiencyCmd(m.detailJobID),
			)
			// nvidia-smi via srun is expensive (spawns a step) — only every
			// 3rd tick (~30s) to keep scheduler overhead bounded.
			if m.ticks%3 == 0 {
				gpuJob := false
				for _, j := range m.lastJobs {
					if j.JobID == m.detailJobID && j.GPUs > 0 {
						gpuJob = true
						break
					}
				}
				if gpuJob {
					cmds = append(cmds, m.fetchGPUUtilCmd(m.detailJobID))
				}
			}
		}
		return m, tea.Batch(cmds...)

	case clusterMsg:
		if msg.err == nil && msg.name != "" {
			m.clusterName = msg.name
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.MouseMsg:
		// Handle wheel directly: bubbles/viewport's internal handler only
		// fires on Action==Press, but some terminals send wheel events with
		// other actions (Motion, etc.). When a job detail overlay is open,
		// the wheel scrolls the log viewport instead of the main table.
		target := &m.viewport
		if m.detailOpen && m.detailJobID > 0 {
			target = &m.detailViewport
		}
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			target.LineUp(3)
		case tea.MouseButtonWheelDown:
			target.LineDown(3)
		}
		return m, nil

	case logTailMsg:
		if msg.jobID == m.detailJobID && m.detailOpen {
			m.detailLogs = msg.lines
			m.detailLogErr = msg.err
			m.detailLogPath = msg.path
			m.detailCWD = msg.cwd
			m.detailCommand = msg.command
			m.updateDetailViewportContent()
		}
		return m, nil

	case effMsg:
		if msg.jobID == m.detailJobID && m.detailOpen {
			m.detailEff = msg.eff
			if pct, ok := m.cpuPercentFromEff(msg.eff); ok {
				m.detailEffSeries = append(m.detailEffSeries, pct)
				if len(m.detailEffSeries) > detailEffMaxSamples {
					m.detailEffSeries = m.detailEffSeries[len(m.detailEffSeries)-detailEffMaxSamples:]
				}
			}
		}
		return m, nil

	case jobHistoryMsg:
		if msg.jobID == m.detailJobID && m.detailOpen && msg.err == nil {
			m.detailHistoryRuns = msg.runs
		}
		return m, nil

	case gpuUtilMsg:
		if msg.jobID == m.detailJobID && m.detailOpen && msg.err == nil {
			m.detailGPU = msg.samples
		}
		return m, nil

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

	case bulkCancelResultMsg:
		if msg.firstErr != nil && msg.succeeded < msg.requested {
			m.cancelFlash = fmt.Sprintf("cancelled %d of %d (first error: %s)",
				msg.succeeded, msg.requested, msg.firstErr.Error())
			m.cancelFlashErr = true
		} else {
			m.cancelFlash = fmt.Sprintf("cancelled %d job%s", msg.succeeded, plural(msg.succeeded))
			m.cancelFlashErr = false
		}
		return m, m.fetchJobsCmd()
	}

	return m, nil
}

type cancelResultMsg struct {
	id  int64
	err error
}

type logTailMsg struct {
	jobID   int64
	path    string
	cwd     string
	command string
	lines   []string
	err     error
}

type effMsg struct {
	jobID int64
	eff   slurm.JobEfficiency
	err   error
}

type clusterMsg struct {
	name string
	err  error
}

type jobHistoryMsg struct {
	jobID int64
	runs  []slurm.AcctJob
	err   error
}

type gpuUtilMsg struct {
	jobID   int64
	samples []slurm.GPUUtil
	err     error
}

func (m *model) fetchGPUUtilCmd(jobID int64) tea.Cmd {
	c := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		s, err := c.JobGPUUtil(ctx, jobID)
		return gpuUtilMsg{jobID: jobID, samples: s, err: err}
	}
}

func (m *model) fetchJobHistoryCmd(jobID int64, jobName, partition string) tea.Cmd {
	c := m.client
	return func() tea.Msg {
		if jobName == "" {
			return jobHistoryMsg{jobID: jobID}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		runs, err := c.JobsByName(ctx, jobName, partition, 30*24*time.Hour)
		return jobHistoryMsg{jobID: jobID, runs: runs, err: err}
	}
}

func (m *model) fetchClusterCmd() tea.Cmd {
	c := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		n, err := c.ClusterName(ctx)
		return clusterMsg{name: n, err: err}
	}
}

func (m *model) fetchEfficiencyCmd(jobID int64) tea.Cmd {
	c := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		e, err := c.JobEfficiency(ctx, jobID)
		return effMsg{jobID: jobID, eff: e, err: err}
	}
}

// detailLogMaxLines caps how many trailing lines we hold for the in-overlay
// log viewer. Big enough to scroll meaningfully, small enough that NFS reads
// of huge .out files stay snappy. For complete files, `muster logs <id> -n 0`.
const detailLogMaxLines = 2000

// fetchLogTailCmd reads the last N lines of a job's stdout via scontrol +
// `tail -n N <path>`. Also passes through the job's cwd and command so the
// detail overlay can display them. Results land as logTailMsg in Update.
func (m *model) fetchLogTailCmd(jobID int64) tea.Cmd {
	c := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		d, err := c.JobDetail(ctx, jobID)
		if err != nil {
			return logTailMsg{jobID: jobID, err: err}
		}
		msg := logTailMsg{
			jobID:   jobID,
			cwd:     d.CurrentWorkingDirectory,
			command: d.Command,
		}
		path := d.StandardOutput
		if path == "" {
			msg.err = fmt.Errorf("interactive session — no stdout file")
			return msg
		}
		msg.path = path
		out, err := exec.CommandContext(ctx, "tail", "-n", fmt.Sprintf("%d", detailLogMaxLines), path).Output()
		if err != nil {
			msg.err = err
			return msg
		}
		msg.lines = strings.Split(strings.TrimRight(string(out), "\n"), "\n")
		return msg
	}
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

// cancelManyCmd cancels a set of jobs sequentially and reports the aggregate
// result. Sequential keeps error messages clean (vs. a single batched call).
func (m *model) cancelManyCmd(ids []int64) tea.Cmd {
	c := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		var firstErr error
		ok := 0
		for _, id := range ids {
			if err := c.Cancel(ctx, id); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			ok++
		}
		return bulkCancelResultMsg{requested: len(ids), succeeded: ok, firstErr: firstErr}
	}
}

type bulkCancelResultMsg struct {
	requested int
	succeeded int
	firstErr  error
}

// toggleSelection adds/removes the cursor row's job ID from the bulk-select
// set (Jobs/Queue tabs only).
func (m *model) toggleSelection() {
	cur := m.cursor[m.tab]
	var id int64
	switch m.tab {
	case tabJobs:
		if cur >= 0 && cur < len(m.lastJobs) {
			id = m.lastJobs[cur].JobID
		}
	case tabQueue:
		if cur >= 0 && cur < len(m.lastQueue) {
			id = m.lastQueue[cur].JobID
		}
	default:
		return
	}
	if id == 0 {
		return
	}
	if m.selectedJobs == nil {
		m.selectedJobs = map[int64]bool{}
	}
	if m.selectedJobs[id] {
		delete(m.selectedJobs, id)
	} else {
		m.selectedJobs[id] = true
	}
}

// maybeOpenConfirmCancel decides between single-row and bulk-selection cancel
// flows based on whether the user has marked jobs with Space.
func (m *model) maybeOpenConfirmCancel() {
	if len(m.selectedJobs) > 0 {
		m.openConfirmBulkCancel()
		return
	}
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

// openConfirmBulkCancel opens the confirm modal in bulk mode. Resolves owners
// from the most recent rendered slices so we can refuse other-user IDs cleanly.
func (m *model) openConfirmBulkCancel() {
	me := currentUser()
	jobInfo := map[int64]struct{ owner, name string }{}
	for _, j := range m.lastJobs {
		jobInfo[j.JobID] = struct{ owner, name string }{j.User, j.Name}
	}
	for _, q := range m.lastQueue {
		jobInfo[q.JobID] = struct{ owner, name string }{q.User, q.Name}
	}

	var refused []int64
	var ok []int64
	for id := range m.selectedJobs {
		info, found := jobInfo[id]
		if !found || info.owner != me {
			refused = append(refused, id)
			continue
		}
		ok = append(ok, id)
	}
	if len(refused) > 0 {
		m.cancelFlash = fmt.Sprintf("refused: %d selected job(s) not owned by %s", len(refused), me)
		m.cancelFlashErr = true
	}
	if len(ok) == 0 {
		m.selectedJobs = nil
		return
	}
	m.bulkCancelIDs = ok
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

// switchTab handles all the work that needs to happen when the active tab
// changes: reset scroll, clamp cursor to new row count, and (lazy) kick off
// the slow sacct fetch the first time the user lands on History.
func (m *model) switchTab(t tabIdx) tea.Cmd {
	m.tab = t
	m.viewport.GotoTop()
	m.clampCursor()
	if t == tabHistory && !m.acctLoaded && !m.acctInFlight {
		m.acctInFlight = true
		return m.fetchAcctCmd()
	}
	return nil
}

// cyclePartition rotates m.partition through the known partitions plus the
// 'all partitions' / cluster-mode pseudo-value (""). reverse=true goes
// backward. Sparkline history is reset since it was keyed to the previous
// scope.
func (m *model) cyclePartition(reverse bool) {
	// Build a stable list: "" first, then each partition we know about.
	names := []string{""}
	for _, p := range m.lastPartitions {
		names = append(names, p.Name)
	}
	if len(names) <= 1 {
		return
	}
	idx := 0
	for i, n := range names {
		if n == m.partition {
			idx = i
			break
		}
	}
	if reverse {
		idx = (idx - 1 + len(names)) % len(names)
	} else {
		idx = (idx + 1) % len(names)
	}
	m.partition = names[idx]
	m.history = nil
	m.viewport.GotoTop()
	m.clampCursor()
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
	var lines []string
	if len(m.bulkCancelIDs) > 0 {
		lines = append(lines, helpTitleStyle.Render(fmt.Sprintf("Cancel %d selected job%s?", len(m.bulkCancelIDs), plural(len(m.bulkCancelIDs)))))
		lines = append(lines, "")
		// Show up to 8 ids inline.
		shown := m.bulkCancelIDs
		more := 0
		if len(shown) > 8 {
			more = len(shown) - 8
			shown = shown[:8]
		}
		for _, id := range shown {
			lines = append(lines, fmt.Sprintf("  • %d", id))
		}
		if more > 0 {
			lines = append(lines, render.ColorFaint(fmt.Sprintf("  + %d more", more)))
		}
	} else {
		lines = append(lines,
			helpTitleStyle.Render(fmt.Sprintf("Cancel job %d?", m.confirmCancelID)),
			"",
			fmt.Sprintf("  %s  %s", render.ColorFaint("user:"), m.confirmCancelOwner),
			fmt.Sprintf("  %s  %s", render.ColorFaint("name:"), m.confirmCancelName),
		)
	}
	lines = append(lines,
		"",
		"  "+render.ColorYellow("y")+" confirm cancel    "+render.ColorYellow("n / esc")+" abort",
	)
	body := strings.Join(lines, "\n")
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("196")).
		Padding(1, 3)
	return lipgloss.Place(m.width, maxHeight, lipgloss.Center, lipgloss.Center, style.Render(body))
}

func (m *model) renderDetailOverlay(maxHeight int) string {
	// Job/queue detail: full-screen layout with a scrollable log viewer
	// below the metadata. Other entity types use the small centered card.
	if m.detailJobID == 0 {
		body := strings.Join([]string{
			helpTitleStyle.Render(m.detailTitle),
			"",
			m.detailBody,
			"",
			render.ColorFaint("press any key to return"),
		}, "\n")
		return lipgloss.Place(m.width, maxHeight, lipgloss.Center, lipgloss.Center, helpCardStyle.Render(body))
	}

	headerLines := []string{
		helpTitleStyle.Render(m.detailTitle),
		m.detailBody,
	}
	if effLine := m.formatEfficiencyLine(); effLine != "" {
		headerLines = append(headerLines, effLine)
	}
	if gpuLine := m.formatGPUUtilLine(); gpuLine != "" {
		headerLines = append(headerLines, gpuLine)
	}
	if histLine := m.formatHistoryLine(); histLine != "" {
		headerLines = append(headerLines, histLine)
	}
	if m.detailCWD != "" {
		headerLines = append(headerLines, detailLine("cwd", m.detailCWD))
	}
	if m.detailCommand != "" {
		headerLines = append(headerLines, detailLine("command", m.detailCommand))
	}
	header := strings.Join(headerLines, "\n")

	logHeader := helpSectionStyle.Render("stdout")
	if m.detailLogPath != "" {
		logHeader += "  " + render.ColorFaint(m.detailLogPath)
	}

	var hint string
	if m.logSearchMode {
		hint = m.logSearchInput.View()
	} else if m.logSearch != "" {
		match := "no match"
		if len(m.logMatchLines) > 0 {
			match = fmt.Sprintf("match %d/%d", m.logMatchCursor+1, len(m.logMatchLines))
		}
		hint = render.ColorFaint("/"+m.logSearch+"  ") + render.ColorYellow(match) +
			render.ColorFaint("  · n/N next/prev · esc clear · ↑/↓ scroll")
	} else {
		hint = render.ColorFaint("↑/↓/k/j scroll · pgup/pgdn page · g/G top/end · / search · esc close")
	}

	headerH := lipgloss.Height(header) + lipgloss.Height(logHeader) + lipgloss.Height(hint) + 2 // 2 blank lines
	vpHeight := maxHeight - headerH
	if vpHeight < 4 {
		vpHeight = 4
	}
	if !m.detailVPReady || m.detailViewport.Width != m.width || m.detailViewport.Height != vpHeight {
		m.detailViewport = viewport.New(m.width, vpHeight)
		m.detailVPReady = true
		m.updateDetailViewportContent()
	} else {
		m.detailViewport.Width = m.width
		m.detailViewport.Height = vpHeight
	}

	return strings.Join([]string{
		header,
		"",
		logHeader,
		m.detailViewport.View(),
		hint,
	}, "\n")
}

// updateDetailViewportContent rebuilds the viewport's content from
// m.detailLogs, hard-wraps long lines to fit, and (if a search term is
// active) highlights matches and records their wrapped-line indices for
// n/N navigation.
func (m *model) updateDetailViewportContent() {
	if !m.detailVPReady {
		return
	}
	wrapWidth := m.detailViewport.Width
	if wrapWidth < 20 {
		wrapWidth = 20
	}
	m.logMatchLines = nil

	switch {
	case m.detailLogErr != nil:
		m.detailViewport.SetContent(render.ColorFaint("(" + m.detailLogErr.Error() + ")"))
		return
	case m.detailLogs == nil:
		m.detailViewport.SetContent(render.ColorFaint("loading…"))
		return
	case len(m.detailLogs) == 0 || (len(m.detailLogs) == 1 && m.detailLogs[0] == ""):
		m.detailViewport.SetContent(render.ColorFaint("(empty)"))
		return
	}

	var wrapped []string
	for _, raw := range m.detailLogs {
		for _, chunk := range hardWrap(raw, wrapWidth) {
			wrapped = append(wrapped, chunk)
		}
	}

	atBottom := m.detailViewport.AtBottom()
	if m.logSearch != "" {
		needleLower := strings.ToLower(m.logSearch)
		highlighted := make([]string, len(wrapped))
		for i, line := range wrapped {
			lower := strings.ToLower(line)
			if !strings.Contains(lower, needleLower) {
				highlighted[i] = line
				continue
			}
			m.logMatchLines = append(m.logMatchLines, i)
			highlighted[i] = highlightMatches(line, m.logSearch)
		}
		m.detailViewport.SetContent(strings.Join(highlighted, "\n"))
	} else {
		m.detailViewport.SetContent(strings.Join(wrapped, "\n"))
	}

	if atBottom && m.logSearch == "" {
		m.detailViewport.GotoBottom()
	}
}

// hardWrap splits s into chunks no longer than width runes. ASCII-clean
// logs use one byte per rune so this is byte-safe in practice.
func hardWrap(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	r := []rune(s)
	if len(r) <= width {
		return []string{s}
	}
	var out []string
	for len(r) > width {
		out = append(out, string(r[:width]))
		r = r[width:]
	}
	if len(r) > 0 {
		out = append(out, string(r))
	}
	return out
}

// highlightMatches wraps every case-insensitive occurrence of needle in line
// with a bold-yellow ANSI envelope.
func highlightMatches(line, needle string) string {
	if needle == "" {
		return line
	}
	const onLeft = "\x1b[1;43;30m"  // bold black-on-yellow
	const onRight = "\x1b[0m"
	lower := strings.ToLower(line)
	nlower := strings.ToLower(needle)
	var b strings.Builder
	for {
		i := strings.Index(lower, nlower)
		if i < 0 {
			b.WriteString(line)
			return b.String()
		}
		b.WriteString(line[:i])
		b.WriteString(onLeft)
		b.WriteString(line[i : i+len(needle)])
		b.WriteString(onRight)
		line = line[i+len(needle):]
		lower = lower[i+len(needle):]
	}
}

// jumpToCurrentMatch scrolls the log viewport so the active match (per
// m.logMatchCursor) is centered.
func (m *model) jumpToCurrentMatch() {
	if !m.detailVPReady || len(m.logMatchLines) == 0 {
		return
	}
	if m.logMatchCursor >= len(m.logMatchLines) {
		m.logMatchCursor = 0
	}
	line := m.logMatchLines[m.logMatchCursor]
	half := m.detailViewport.Height / 2
	off := line - half
	if off < 0 {
		off = 0
	}
	m.detailViewport.SetYOffset(off)
}

// openDetail prepares the detail overlay for the current cursor row and
// returns a tea.Cmd that fetches the job log tail (jobs/queue tabs only).
//
// Enter on a collapsed array row (ArrayCount>0) drills into the array
// instead of opening detail — the user gets the per-task list; a second
// Enter on a task then opens its detail.
func (m *model) openDetail() tea.Cmd {
	cur := m.cursor[m.tab]
	if cur < 0 || cur >= m.rowCounts[m.tab] {
		return nil
	}

	// Drill into array if the cursor row is a collapsed array.
	if m.tab == tabJobs && cur < len(m.lastJobs) {
		if r := m.lastJobs[cur]; r.ArrayCount > 0 {
			m.arrayDrill = r.JobID
			m.cursor[m.tab] = 0
			m.viewport.GotoTop()
			return nil
		}
	}
	if m.tab == tabQueue && cur < len(m.lastQueue) {
		if r := m.lastQueue[cur]; r.ArrayCount > 0 {
			m.arrayDrill = r.JobID
			m.cursor[m.tab] = 0
			m.viewport.GotoTop()
			return nil
		}
	}
	m.detailJobID = 0
	m.detailLogs = nil
	m.detailLogErr = nil
	m.detailLogPath = ""
	m.detailCWD = ""
	m.detailCommand = ""
	m.detailEff = slurm.JobEfficiency{}
	m.detailEffSeries = nil
	m.detailHistoryRuns = nil
	m.detailGPU = nil

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
			m.detailJobID = j.JobID
			m.detailLogs = nil
			m.detailLogErr = nil
			m.detailOpen = true
		}
	case tabUsers:
		if cur < len(m.lastUsers) {
			u := m.lastUsers[cur]
			m.detailTitle = "User " + u.User
			m.detailBody = renderUserDetail(u)
			m.detailOpen = true
		}
	case tabAccounts:
		if cur < len(m.lastAccounts) {
			a := m.lastAccounts[cur]
			m.detailTitle = "Account " + a.Account
			m.detailBody = renderAccountDetail(a)
			m.detailOpen = true
		}
	case tabQueue:
		if cur < len(m.lastQueue) {
			q := m.lastQueue[cur]
			m.detailTitle = fmt.Sprintf("Pending job %d  %s / %s", q.JobID, q.User, q.Name)
			m.detailBody = renderQueueDetail(q)
			m.detailJobID = q.JobID
			m.detailLogs = nil
			m.detailLogErr = nil
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
	if m.detailJobID > 0 {
		// Find the selected JobRow so we can pass its name to the history query.
		var name, part string
		hasGPU := false
		for _, j := range m.lastJobs {
			if j.JobID == m.detailJobID {
				name = j.Name
				part = j.Partition
				hasGPU = j.GPUs > 0
				break
			}
		}
		if name == "" {
			for _, q := range m.lastQueue {
				if q.JobID == m.detailJobID {
					name = q.Name
					part = q.Partition
					hasGPU = q.GPUs > 0
					break
				}
			}
		}
		cmds := []tea.Cmd{
			m.fetchLogTailCmd(m.detailJobID),
			m.fetchEfficiencyCmd(m.detailJobID),
			m.fetchJobHistoryCmd(m.detailJobID, name, part),
		}
		if hasGPU {
			cmds = append(cmds, m.fetchGPUUtilCmd(m.detailJobID))
		}
		return tea.Batch(cmds...)
	}
	return nil
}

func detailLine(label, value string) string {
	return fmt.Sprintf("  %-14s  %s", render.ColorFaint(label), value)
}

const detailEffMaxSamples = 60 // 60 samples × 10s tick = 10 minutes of trend

// formatHistoryLine summarizes the sacct query for past runs of the same job
// name: how many prior runs, mean elapsed, and how this run compares.
func (m *model) formatHistoryLine() string {
	if m.detailJobID == 0 || len(m.detailHistoryRuns) == 0 {
		return ""
	}
	// Only completed/failed/timeout/cancelled jobs whose elapsed > 0 count
	// toward the average; exclude the current run if it's also in the set.
	var sum time.Duration
	count := 0
	for _, r := range m.detailHistoryRuns {
		if r.ID == m.detailJobID {
			continue
		}
		switch r.State {
		case "COMPLETED", "FAILED", "TIMEOUT", "CANCELLED":
		default:
			continue
		}
		if r.Elapsed <= 0 {
			continue
		}
		sum += r.Elapsed
		count++
	}
	if count == 0 {
		return ""
	}
	avg := sum / time.Duration(count)

	var currentRuntime time.Duration
	var currentState string
	for _, j := range m.lastJobs {
		if j.JobID == m.detailJobID {
			currentRuntime = j.Runtime
			currentState = j.State
			break
		}
	}

	parts := []string{fmt.Sprintf("%d prior runs · avg %s", count, render.HumanDuration(avg))}
	if currentRuntime > 0 && avg > 0 {
		ratio := float64(currentRuntime) / float64(avg)
		pct := int((ratio - 1) * 100)
		var marker string
		if currentState == "RUNNING" {
			runPct := int(ratio * 100)
			if runPct < 100 {
				marker = render.ColorFaint(fmt.Sprintf("this run %s so far (%d%% of avg)", render.HumanDuration(currentRuntime), runPct))
			} else {
				marker = render.ColorYellow(fmt.Sprintf("⚠ this run %s — %d%% of avg, may be running long", render.HumanDuration(currentRuntime), runPct))
			}
		} else {
			sign := "+"
			if pct < 0 {
				sign = ""
			}
			marker = fmt.Sprintf("this run %s (%s%d%% vs avg)", render.HumanDuration(currentRuntime), sign, pct)
			switch {
			case pct > 25:
				marker = render.ColorRed("▲ ") + marker
			case pct < -25:
				marker = render.ColorGreen("▼ ") + marker
			}
		}
		parts = append(parts, marker)
	}
	return detailLine("history", strings.Join(parts, " · "))
}

// cpuPercentFromEff turns a JobEfficiency snapshot into a 0-100 CPU%
// (used / allocated) by finding the matching JobRow for alloc_cpus + runtime.
// Returns ok=false if the math is undefined (zero runtime, missing job).
func (m *model) cpuPercentFromEff(e slurm.JobEfficiency) (int, bool) {
	var allocCPU int
	var runtime time.Duration
	for _, j := range m.lastJobs {
		if j.JobID == e.JobID {
			allocCPU = j.CPUs
			runtime = j.Runtime
			break
		}
	}
	if allocCPU == 0 || runtime <= 0 || e.AveCPU <= 0 {
		return 0, false
	}
	cores := e.AveCPU.Seconds() / runtime.Seconds()
	pct := int(cores / float64(allocCPU) * 100)
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return pct, true
}

// formatEfficiencyLine returns the 'live' metadata row(s): current cores in
// use + percent + a sparkline of recent CPU usage if we have enough samples.
func (m *model) formatEfficiencyLine() string {
	if m.detailJobID == 0 {
		return ""
	}
	e := m.detailEff
	if e.AveCPU == 0 && e.MaxRSSMB == 0 {
		return ""
	}
	pct, ok := m.cpuPercentFromEff(e)
	if !ok {
		bits := []string{}
		if e.AveCPU > 0 {
			bits = append(bits, fmt.Sprintf("cpu time %s", render.HumanDuration(e.AveCPU)))
		}
		if e.MaxRSSMB > 0 {
			bits = append(bits, fmt.Sprintf("peak mem %s", render.HumanMB(e.MaxRSSMB)))
		}
		if len(bits) == 0 {
			return ""
		}
		return detailLine("live", strings.Join(bits, " · "))
	}
	// Look up alloc again — we know it's non-zero from the ok branch above.
	var allocCPU int
	var runtime time.Duration
	for _, j := range m.lastJobs {
		if j.JobID == e.JobID {
			allocCPU = j.CPUs
			runtime = j.Runtime
			break
		}
	}
	cores := e.AveCPU.Seconds() / runtime.Seconds()

	pctStr := fmt.Sprintf("%d%%", pct)
	switch {
	case pct >= 75:
		pctStr = render.ColorGreen(pctStr)
	case pct >= 25:
		pctStr = render.ColorYellow(pctStr)
	default:
		pctStr = render.ColorRed(pctStr)
	}
	parts := []string{
		fmt.Sprintf("%.1f / %d cores (%s)", cores, allocCPU, pctStr),
	}
	if e.AveRSSMB > 0 {
		parts = append(parts, "mem "+render.HumanMB(e.AveRSSMB))
		if e.MaxRSSMB > e.AveRSSMB {
			parts = append(parts, "peak "+render.HumanMB(e.MaxRSSMB))
		}
	} else if e.MaxRSSMB > 0 {
		parts = append(parts, "peak "+render.HumanMB(e.MaxRSSMB))
	}
	if len(m.detailEffSeries) > 1 {
		spark := render.Sparkline(m.detailEffSeries, 24)
		parts = append(parts, "trend "+spark)
	}
	return detailLine("live", strings.Join(parts, " · "))
}

// formatGPUUtilLine renders one entry per assigned GPU with compute% and
// memory used/total. Empty when no nvidia-smi data has arrived yet.
func (m *model) formatGPUUtilLine() string {
	if len(m.detailGPU) == 0 {
		return ""
	}
	parts := make([]string, 0, len(m.detailGPU))
	for _, g := range m.detailGPU {
		util := fmt.Sprintf("%d%%", g.UtilGPUPct)
		switch {
		case g.UtilGPUPct >= 75:
			util = render.ColorGreen(util)
		case g.UtilGPUPct >= 25:
			util = render.ColorYellow(util)
		default:
			util = render.ColorRed(util)
		}
		mem := fmt.Sprintf("%s/%s", render.HumanMB(g.MemUsedMB), render.HumanMB(g.MemTotalMB))
		parts = append(parts, fmt.Sprintf("#%d %s %s", g.Index, util, mem))
	}
	return detailLine("gpu util", strings.Join(parts, "  ·  "))
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

func renderAccountDetail(a aggregate.AccountRollup) string {
	return strings.Join([]string{
		detailLine("distinct users", fmt.Sprintf("%d", a.Users)),
		detailLine("running", fmt.Sprintf("%d jobs", a.Running)),
		detailLine("pending", fmt.Sprintf("%d jobs", a.Pending)),
		detailLine("cpus held", fmt.Sprintf("%d", a.CPUsHeld)),
		detailLine("gpus held", fmt.Sprintf("%d", a.GPUsHeld)),
		detailLine("memory", render.HumanMB(a.MemoryMBHeld)),
		detailLine("oldest run", render.HumanDuration(a.OldestRunAge)),
	}, "\n")
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
		"  " + helpKeyStyle.Render("1 – 7") + "       jump directly to tab",
		"  " + helpKeyStyle.Render("tab / ⇧tab") + "  next / previous tab",
		"  " + helpKeyStyle.Render("h / l") + "       previous / next tab",
		"",
		helpSectionStyle.Render("Actions"),
		"  " + helpKeyStyle.Render("r") + "           refresh now",
		"  " + helpKeyStyle.Render("s") + "           cycle sort key (jobs, users, queue)",
		"  " + helpKeyStyle.Render("/") + "           filter rows (name/user/reason)",
		"  " + helpKeyStyle.Render("m") + "           toggle Me mode (your jobs only)",
		"  " + helpKeyStyle.Render("p / P") + "       cycle partition focus (forward / back)",
		"  " + helpKeyStyle.Render("enter") + "       open detail for the selected row",
		"  " + helpKeyStyle.Render("space") + "       toggle row selection (jobs/queue)",
		"  " + helpKeyStyle.Render("c") + "           cancel cursor row OR all selected (own only)",
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
	// Top strip: cluster · timestamp · user. Self-documents screenshots of
	// the dashboard.
	strip := m.renderClusterStrip()

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
	tabBar := lipgloss.JoinHorizontal(lipgloss.Bottom, title, " ", bar)

	if strip == "" {
		return tabBar
	}
	return lipgloss.JoinVertical(lipgloss.Left, strip, tabBar)
}

func (m *model) renderClusterStrip() string {
	cluster := m.clusterName
	if cluster == "" {
		cluster = "slurm"
	}
	user := currentUser()
	if user == "" {
		user = "?"
	}
	// Avoid Date.Now() in production code paths if anyone reuses this in
	// snapshot mode — use lastFetch when present, falling back to a literal.
	ts := m.lastFetch
	if ts.IsZero() {
		ts = time.Now()
	}
	parts := []string{
		render.Bold(cluster),
		render.ColorFaint(ts.Format("2006-01-02 15:04:05")),
		render.ColorFaint(user),
	}
	return clusterStripStyle.Render(strings.Join(parts, "  ·  "))
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
		jobs := m.jobs
		expand := false
		if m.arrayDrill != 0 {
			expand = true
			// Fresh slice; reusing m.jobs's backing array as `m.jobs[:0]`
			// would overwrite the model's persistent state in-place — the
			// filter loop's range iterator and the append target alias the
			// same memory, so later renders see scrambled data.
			var filtered []slurm.Job
			for _, j := range jobs {
				if j.ArrayJobID == m.arrayDrill {
					filtered = append(filtered, j)
				}
			}
			jobs = filtered
		}
		rows := aggregate.JobsCollapsed(jobs, m.partition, me, false, expand, sort, 0, time.Now())
		if f != "" {
			filtered := rows[:0]
			for _, r := range rows {
				if contains(r.Name, r.User, r.Account, r.Nodes) {
					filtered = append(filtered, r)
				}
			}
			rows = filtered
		}
		for i := range rows {
			if m.jobIsNew[rows[i].JobID] {
				rows[i].IsNew = true
			}
			if m.selectedJobs[rows[i].JobID] {
				rows[i].IsSelected = true
			}
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
	case tabAccounts:
		sort := orDefault(m.currentSort(), "cpus")
		rows := aggregate.Accounts(m.jobs, m.partition, sort, 0, time.Now())
		if f != "" {
			filtered := rows[:0]
			for _, r := range rows {
				if contains(r.Account) {
					filtered = append(filtered, r)
				}
			}
			rows = filtered
		}
		m.lastAccounts = rows
		rowCount = len(rows)
		render.RenderAccounts(&buf, rows)
	case tabQueue:
		sort := orDefault(m.currentSort(), "priority")
		queueJobs := m.jobs
		expand := false
		if m.arrayDrill != 0 {
			expand = true
			// Fresh slice — see comment in tabJobs above.
			var filtered []slurm.Job
			for _, j := range queueJobs {
				if j.ArrayJobID == m.arrayDrill {
					filtered = append(filtered, j)
				}
			}
			queueJobs = filtered
		}
		rows := aggregate.QueueCollapsed(queueJobs, m.partition, false, expand, "", sort, time.Now())
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
		for i := range filtered {
			if m.jobIsNew[filtered[i].JobID] {
				filtered[i].IsNew = true
			}
			if m.selectedJobs[filtered[i].JobID] {
				filtered[i].IsSelected = true
			}
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
		subtitle := "window: last 24h"
		if m.partition != "" {
			subtitle = "partition " + m.partition + " · " + subtitle
		}
		if !m.lastFetch.IsZero() {
			subtitle += " · fetched " + m.lastFetch.Format("15:04:05")
		}
		subtitle += "   (sacct shows only jobs visible to you)"
		render.RenderHistory(&buf, filtered, "user", subtitle)
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
			lines[i] = " " + line // border line
			continue
		}
		if dataIdx == -1 {
			lines[i] = " " + line // header
			dataIdx = 0
			continue
		}
		if dataIdx == rowIdx {
			lines[i] = tintRow(cursorMarker + line)
		} else {
			lines[i] = " " + line
		}
		dataIdx++
	}
	return strings.Join(lines, "\n")
}

// tintRow wraps a line in a soft background color and re-applies the bg after
// every internal `\x1b[0m` reset — go-pretty emits resets around each colored
// cell, and a naive outer bg would drop on the first reset and never come
// back, leaving the background visible only on the leftmost piece of the row.
func tintRow(line string) string {
	const bgOn = "\x1b[48;5;238m"
	const bgOff = "\x1b[0m"
	const reset = "\x1b[0m"
	// Replace every internal reset with reset + re-application of bg.
	patched := strings.ReplaceAll(line, reset, reset+bgOn)
	return bgOn + patched + bgOff
}

// cursorMarker is the leftmost indicator on the selected row: a bold cyan ▶.
var cursorMarker = "\x1b[1;36m▶\x1b[0m"

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
	help := "? help · q · r refresh · tab · 1-7 · s sort · / filter · m me · p partition · space select · c cancel"
	if m.arrayDrill != 0 {
		help = "↩ array " + fmt.Sprintf("%d_*", m.arrayDrill) + " (esc back) · " + help
	}
	if m.meMode {
		help += "  ·  " + render.ColorYellow("Me mode")
	}
	if len(m.selectedJobs) > 0 {
		help += "  ·  " + render.ColorCyan(fmt.Sprintf("%d selected", len(m.selectedJobs)))
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
	clusterStripStyle = lipgloss.NewStyle().
				Padding(0, 1)
)
