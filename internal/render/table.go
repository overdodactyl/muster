package render

import (
	"fmt"
	"io"
	"os"

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
