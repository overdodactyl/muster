package render

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/olekukonko/tablewriter"

	"muster/internal/aggregate"
)

func newTable(w io.Writer, headers []string) *tablewriter.Table {
	t := tablewriter.NewWriter(w)
	t.SetHeader(headers)
	t.SetAlignment(tablewriter.ALIGN_LEFT)
	t.SetAutoWrapText(false)
	t.SetBorder(true)
	t.SetCenterSeparator("+")
	t.SetColumnSeparator("|")
	t.SetRowSeparator("-")
	t.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
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
	t := newTable(w, []string{"PARTITION", "NODES I/M/A/D", "CPUS A/T", "GPUS A/T", "MEM A/T", "RUN", "PEND"})
	for _, r := range rows {
		gpus := "-"
		if r.TotalGPUs > 0 {
			model := ""
			if r.GPUModel != "" {
				model = " " + r.GPUModel
			}
			gpus = fmt.Sprintf("%d/%d%s", r.AllocGPUs, r.TotalGPUs, model)
			if r.AllocGPUs == 0 {
				gpus = ColorGreen(gpus)
			} else if r.AllocGPUs >= r.TotalGPUs {
				gpus = ColorRed(gpus)
			} else {
				gpus = ColorYellow(gpus)
			}
		}
		cpus := fmt.Sprintf("%d/%d", r.AllocCPUs, r.TotalCPUs)
		mem := fmt.Sprintf("%s/%s", HumanMB(r.AllocMemMB), HumanMB(r.TotalMemMB))
		t.Append([]string{
			ColorCyan(r.Name),
			fmtStateCounts(r.NodeCounts),
			cpus,
			gpus,
			mem,
			fmt.Sprintf("%d", r.RunningJobs),
			fmt.Sprintf("%d", r.PendingJobs),
		})
	}
	t.Render()
	fmt.Fprintln(w, "Legend: I=idle  M=mixed  A=alloc  D=down/drain   CPUS A/T=allocated/total  MEM A/T=allocated/total")
}

func RenderNodes(w io.Writer, rows []aggregate.NodeRow, showJobs bool) {
	if w == nil {
		w = os.Stdout
	}
	if len(rows) == 0 {
		fmt.Fprintln(w, "no nodes matched")
		return
	}
	headers := []string{"NODE", "PART", "STATE", "CPUS A/I/T", "MEM USED/T", "GPU A/T", "USERS"}
	t := newTable(w, headers)
	for _, r := range rows {
		gpu := "-"
		if r.GPUsTotal > 0 {
			model := ""
			if r.GPUModel != "" {
				model = " " + r.GPUModel
			}
			gpu = fmt.Sprintf("%d/%d%s", r.GPUsAlloc, r.GPUsTotal, model)
			if r.GPUsAlloc == 0 {
				gpu = ColorGreen(gpu)
			} else if r.GPUsAlloc >= r.GPUsTotal {
				gpu = ColorRed(gpu)
			} else {
				gpu = ColorYellow(gpu)
			}
		}
		users := "-"
		if showJobs && len(r.UserJobs) > 0 {
			users = JoinList(r.UserJobs, 6)
		} else if len(r.Users) > 0 {
			coloured := make([]string, 0, len(r.Users))
			for _, u := range r.Users {
				coloured = append(coloured, ColorCyan(u))
			}
			users = JoinList(coloured, 6)
		}
		t.Append([]string{
			ColorCyan(r.Name),
			r.Partition,
			ColorState(r.StateClass),
			fmt.Sprintf("%d/%d/%d", r.CPUsAlloc, r.CPUsIdle, r.CPUsTotal),
			fmt.Sprintf("%s/%s", HumanMB(r.MemAllocMB), HumanMB(r.MemTotalMB)),
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
		t.Append([]string{
			ColorCyan(r.User),
			fmt.Sprintf("%d", r.Running),
			fmt.Sprintf("%d", r.Pending),
			fmt.Sprintf("%d", r.CPUsHeld),
			fmt.Sprintf("%d", r.GPUsHeld),
			HumanMB(r.MemoryMBHeld),
			HumanDuration(r.OldestRunAge),
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
