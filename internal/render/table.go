package render

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"

	"muster/internal/aggregate"
)

// newTable returns a go-pretty table writer with the project-wide style:
// rounded unicode borders, left-aligned cells, no auto-wrap, bold-cyan
// headers, and zebra-striped rows for scannability.
func newTable(w io.Writer, headers []string) table.Writer {
	t := table.NewWriter()
	t.SetOutputMirror(w)

	hdr := make(table.Row, len(headers))
	for i, h := range headers {
		hdr[i] = h
	}
	t.AppendHeader(hdr)

	style := table.StyleRounded
	style.Options.SeparateRows = false
	if !color.NoColor {
		style.Color.Header = text.Colors{text.FgHiCyan, text.Bold}
		style.Color.Border = text.Colors{text.Faint}
		style.Color.Separator = text.Colors{text.Faint}
	}
	t.SetStyle(style)

	return t
}

// fmtStateCounts renders "I/M/A/D" with each count colored individually.
func fmtStateCounts(c aggregate.StateCounts) string {
	idle := fmt.Sprintf("%d", c.Idle)
	mixed := fmt.Sprintf("%d", c.Mixed)
	alloc := fmt.Sprintf("%d", c.Alloc)
	down := fmt.Sprintf("%d", c.Down+c.Drain)
	if c.Idle > 0 {
		idle = ColorGreen(idle)
	}
	if c.Mixed > 0 {
		mixed = ColorYellow(mixed)
	}
	if c.Alloc > 0 {
		alloc = ColorGreen(alloc)
	}
	if c.Down+c.Drain > 0 {
		down = ColorRed(down)
	}
	return fmt.Sprintf("%s/%s/%s/%s", idle, mixed, alloc, down)
}

func RenderPartitions(w io.Writer, rows []aggregate.PartitionSummary) {
	if w == nil {
		w = os.Stdout
	}
	if len(rows) == 0 {
		fmt.Fprintln(w, "no partitions matched")
		return
	}
	cpuStrs := make([]string, len(rows))
	gpuStrs := make([]string, len(rows))
	memStrs := make([]string, len(rows))
	for i, r := range rows {
		cpuStrs[i] = fmt.Sprintf("%d/%d", r.AllocCPUs, r.TotalCPUs)
		if r.TotalGPUs > 0 {
			model := ""
			if r.GPUModel != "" {
				model = " " + r.GPUModel
			}
			gpuStrs[i] = fmt.Sprintf("%d/%d%s", r.AllocGPUs, r.TotalGPUs, model)
		}
		memStrs[i] = fmt.Sprintf("%s/%s", HumanMB(r.AllocMemMB), HumanMB(r.TotalMemMB))
	}
	cw, gw, mw := maxLen(cpuStrs), maxLen(gpuStrs), maxLen(memStrs)

	t := newTable(w, []string{"PARTITION", "NODES I/M/A/D", "CPUS", "GPUS", "MEM", "RUN", "PEND"})
	for i, r := range rows {
		gpus := "-"
		if gpuStrs[i] != "" {
			gpus = padRight(gpuStrs[i], gw) + "  " + Bar(r.AllocGPUs, r.TotalGPUs, 10)
		}
		cpus := padRight(cpuStrs[i], cw) + "  " + Bar(r.AllocCPUs, r.TotalCPUs, 10)
		mem := padRight(memStrs[i], mw) + "  " + Bar(r.AllocMemMB, r.TotalMemMB, 10)
		t.AppendRow(table.Row{
			ColorCyan(r.Name),
			fmtStateCounts(r.NodeCounts),
			cpus,
			gpus,
			mem,
			r.RunningJobs,
			r.PendingJobs,
		})
	}
	t.Render()
	fmt.Fprintln(w, ColorFaint("legend: I/M/A/D = idle/mixed/alloc/down+drain   bar = allocated share"))
}

func RenderNodes(w io.Writer, rows []aggregate.NodeRow, showJobs bool) {
	if w == nil {
		w = os.Stdout
	}
	if len(rows) == 0 {
		fmt.Fprintln(w, "no nodes matched")
		return
	}
	cpuStrs := make([]string, len(rows))
	gpuStrs := make([]string, len(rows))
	memStrs := make([]string, len(rows))
	memUsed := make([]int, len(rows))
	for i, r := range rows {
		cpuStrs[i] = fmt.Sprintf("%d/%d", r.CPUsAlloc, r.CPUsTotal)
		if r.GPUsTotal > 0 {
			model := ""
			if r.GPUModel != "" {
				model = " " + r.GPUModel
			}
			gpuStrs[i] = fmt.Sprintf("%d/%d%s", r.GPUsAlloc, r.GPUsTotal, model)
		}
		mu := r.MemTotalMB - r.MemFreeMB
		if mu < r.MemAllocMB {
			mu = r.MemAllocMB
		}
		memUsed[i] = mu
		memStrs[i] = fmt.Sprintf("%s/%s", HumanMB(mu), HumanMB(r.MemTotalMB))
	}
	cw, gw, mw := maxLen(cpuStrs), maxLen(gpuStrs), maxLen(memStrs)

	t := newTable(w, []string{"NODE", "PART", "STATE", "CPUS", "MEM", "GPU", "USERS"})
	for i, r := range rows {
		gpu := ColorFaint("-")
		if gpuStrs[i] != "" {
			gpu = padRight(gpuStrs[i], gw) + "  " + Bar(r.GPUsAlloc, r.GPUsTotal, 6)
		}
		users := ColorFaint("-")
		if showJobs && len(r.UserJobs) > 0 {
			users = JoinList(r.UserJobs, 6)
		} else if len(r.Users) > 0 {
			coloured := make([]string, 0, len(r.Users))
			for _, u := range r.Users {
				coloured = append(coloured, ColorCyan(u))
			}
			users = JoinList(coloured, 6)
		}
		cpus := padRight(cpuStrs[i], cw) + "  " + Bar(r.CPUsAlloc, r.CPUsTotal, 8)
		mem := padRight(memStrs[i], mw) + "  " + Bar(memUsed[i], r.MemTotalMB, 8)
		t.AppendRow(table.Row{
			ColorCyan(r.Name),
			r.Partition,
			ColorState(r.StateClass),
			cpus,
			mem,
			gpu,
			users,
		})
	}
	t.Render()
}

func RenderUsers(w io.Writer, rows []aggregate.UserRollup) {
	if w == nil {
		w = os.Stdout
	}
	if len(rows) == 0 {
		fmt.Fprintln(w, "no users matched")
		return
	}
	t := newTable(w, []string{"USER", "RUN", "PEND", "CPUS", "GPUS", "MEM", "OLDEST RUN"})
	for _, r := range rows {
		t.AppendRow(table.Row{
			ColorCyan(r.User),
			r.Running,
			r.Pending,
			r.CPUsHeld,
			r.GPUsHeld,
			HumanMB(r.MemoryMBHeld),
			HumanDuration(r.OldestRunAge),
		})
	}
	t.Render()
}

func RenderUsage(w io.Writer, rows []aggregate.UsageRow) {
	if w == nil {
		w = os.Stdout
	}
	if len(rows) == 0 {
		fmt.Fprintln(w, "no completed jobs in this window")
		return
	}
	t := newTable(w, []string{"USER", "JOBS", "CPU-HRS REQ", "CPU-HRS USED", "EFF%", "WORST JOB"})
	for _, r := range rows {
		effStr := fmt.Sprintf("%.0f%%", r.Efficiency)
		switch {
		case r.Efficiency >= 75:
			effStr = ColorGreen(effStr)
		case r.Efficiency >= 25:
			effStr = ColorYellow(effStr)
		default:
			effStr = ColorRed(effStr)
		}
		worst := ColorFaint("-")
		if r.WorstJobID != 0 {
			worst = fmt.Sprintf("%d (%.0f%%) %s",
				r.WorstJobID, r.WorstJobEff, truncate(r.WorstJobName, 22))
		}
		t.AppendRow(table.Row{
			ColorCyan(r.User),
			r.Jobs,
			fmt.Sprintf("%.1f", r.CPUHoursReq),
			fmt.Sprintf("%.1f", r.CPUHoursUsed),
			effStr,
			worst,
		})
	}
	t.Render()
}

func RenderGPUs(w io.Writer, rows []aggregate.GPURow) {
	if w == nil {
		w = os.Stdout
	}
	if len(rows) == 0 {
		fmt.Fprintln(w, "no GPU-bearing nodes in this scope")
		return
	}
	t := newTable(w, []string{"NODE", "GPU", "STATE", "USER", "JOB", "RUNTIME"})
	for _, r := range rows {
		gpu := fmt.Sprintf("#%d", r.Index)
		if r.Model != "" {
			gpu = fmt.Sprintf("%s #%d", r.Model, r.Index)
		}
		state := ColorGreen("idle")
		user := ColorFaint("-")
		job := ColorFaint("-")
		runtime := ColorFaint("-")
		if r.InUse {
			state = ColorRed("in use")
			user = ColorCyan(r.User)
			job = fmt.Sprintf("%s (%d)", truncate(r.JobName, 20), r.JobID)
			runtime = HumanDuration(r.Runtime)
		}
		t.AppendRow(table.Row{
			ColorCyan(r.Node),
			gpu,
			state,
			user,
			job,
			runtime,
		})
	}
	t.Render()
}

func RenderReservations(w io.Writer, rows []aggregate.ReservationRow) {
	if w == nil {
		w = os.Stdout
	}
	if len(rows) == 0 {
		fmt.Fprintln(w, "no reservations")
		return
	}
	t := newTable(w, []string{"NAME", "STATE", "USERS", "PARTITION", "NODES", "WINDOW", "TIMING"})
	for _, r := range rows {
		state := r.State
		switch r.State {
		case "active":
			state = ColorGreen(state)
		case "upcoming":
			state = ColorYellow(state)
		case "ended":
			state = ColorFaint(state)
		}
		window := "-"
		if !r.StartTime.IsZero() {
			window = r.StartTime.Format("Jan 02 15:04") + " → "
			if !r.EndTime.IsZero() {
				window += r.EndTime.Format("Jan 02 15:04")
			} else {
				window += "∞"
			}
		}
		timing := "-"
		switch r.State {
		case "active":
			if r.EndsIn > 0 {
				timing = "ends in " + HumanDuration(r.EndsIn)
			}
		case "upcoming":
			timing = "starts in " + HumanDuration(r.StartsIn)
		}
		users := r.Users
		if users == "" {
			users = ColorFaint("-")
		}
		part := r.Partition
		if part == "" {
			part = ColorFaint("any")
		}
		t.AppendRow(table.Row{
			ColorCyan(r.Name),
			state,
			users,
			part,
			truncate(r.Nodes, 30),
			window,
			timing,
		})
	}
	t.Render()
}

func RenderJobs(w io.Writer, rows []aggregate.JobRow) {
	if w == nil {
		w = os.Stdout
	}
	if len(rows) == 0 {
		fmt.Fprintln(w, "no jobs matched")
		return
	}
	t := newTable(w, []string{"JOBID", "USER", "NAME", "PART", "ST", "NODES", "CPU", "GPU", "MEM", "RUNTIME"})
	for _, r := range rows {
		gpu := ColorFaint("-")
		if r.GPUs > 0 {
			gpu = fmt.Sprintf("%d", r.GPUs)
		}
		state := r.State
		if r.State == "PENDING" {
			state = ColorYellow("PD")
		} else if r.State == "RUNNING" {
			state = ColorGreen("R")
		}
		nodes := r.Nodes
		if nodes == "" {
			nodes = ColorFaint("-")
		}
		t.AppendRow(table.Row{
			r.JobID,
			ColorCyan(r.User),
			truncate(r.Name, 28),
			r.Partition,
			state,
			truncate(nodes, 30),
			r.CPUs,
			gpu,
			HumanMB(r.MemoryMB),
			HumanDuration(r.Runtime),
		})
	}
	t.Render()
}

func RenderQueue(w io.Writer, rows []aggregate.QueueRow) {
	if w == nil {
		w = os.Stdout
	}
	if len(rows) == 0 {
		fmt.Fprintln(w, "no queued jobs matched")
		return
	}
	t := newTable(w, []string{"JOBID", "USER", "NAME", "CPU", "GPU", "MEM", "PRI", "REASON", "EXPLANATION"})
	for _, r := range rows {
		gpu := ColorFaint("-")
		if r.GPUs > 0 {
			gpu = fmt.Sprintf("%d", r.GPUs)
		}
		reason := r.Reason
		if reason == "" {
			reason = "-"
		}
		switch r.Reason {
		case "JobHeldUser", "JobHeldAdmin", "DependencyNeverSatisfied":
			reason = ColorYellow(reason)
		}
		t.AppendRow(table.Row{
			r.JobID,
			ColorCyan(r.User),
			truncate(r.Name, 24),
			r.CPUs,
			gpu,
			HumanMB(r.MemoryMB),
			r.Priority,
			reason,
			r.ReasonHuman,
		})
	}
	t.Render()
}

func RenderHistory(w io.Writer, rows []aggregate.HistoryRow, keyHeader string) {
	if w == nil {
		w = os.Stdout
	}
	if len(rows) == 0 {
		fmt.Fprintln(w, "no completed jobs in this window")
		return
	}
	if keyHeader == "" {
		keyHeader = "KEY"
	}
	t := newTable(w, []string{strings.ToUpper(keyHeader), "JOBS", "DONE", "FAIL", "TO", "CANC", "CPU-HOURS", "GPU-HOURS"})
	for _, r := range rows {
		fail := fmt.Sprintf("%d", r.Failed)
		if r.Failed > 0 {
			fail = ColorYellow(fail)
		}
		to := fmt.Sprintf("%d", r.Timeout)
		if r.Timeout > 0 {
			to = ColorRed(to)
		}
		t.AppendRow(table.Row{
			ColorCyan(r.Key),
			r.Jobs,
			r.Completed,
			fail,
			to,
			r.Cancelled,
			fmt.Sprintf("%.1f", r.CPUHours),
			fmt.Sprintf("%.1f", r.GPUHours),
		})
	}
	t.Render()
}

// JoinList truncates a string slice for fixed-width display.
func JoinList(items []string, max int) string {
	if len(items) == 0 {
		return "-"
	}
	if max <= 0 || len(items) <= max {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:max], ", ") + fmt.Sprintf(", +%d more", len(items)-max)
}

// truncate shortens s to at most n runes, appending an ellipsis when cut.
func truncate(s string, n int) string {
	if n <= 1 || len(s) <= n {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// maxLen returns the longest string length in the slice.
func maxLen(ss []string) int {
	n := 0
	for _, s := range ss {
		if len(s) > n {
			n = len(s)
		}
	}
	return n
}

// padRight space-pads s on the right so it is at least n runes wide.
func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}
