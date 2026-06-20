package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"muster/internal/aggregate"
	"muster/internal/slurm"
)

var (
	exporterListen     string
	exporterRefresh    time.Duration
	exporterPartitions string
)

var exporterCmd = &cobra.Command{
	Use:   "exporter",
	Short: "Serve Slurm metrics in Prometheus text-exposition format",
	Long: `Runs an HTTP server that exposes /metrics in the Prometheus
text-exposition format. Point a Prometheus server at it and you can
build Grafana dashboards / alerts on cluster state without each
consumer having to run scontrol/squeue themselves.

The exporter caches each scrape for --refresh (default 15s) so a noisy
Prometheus polling every second still only hits Slurm once per window.

Metrics emitted:

  slurm_partition_cpus{partition,state}            gauge
  slurm_partition_gpus{partition,model,state}      gauge
  slurm_partition_memory_bytes{partition,state}    gauge
  slurm_partition_nodes{partition,state}           gauge
  slurm_partition_jobs{partition,state}            gauge
  slurm_node_state{node,partition,state}           0|1
  slurm_scrape_seconds                             scrape duration

Use --partitions to restrict to a subset (comma-separated).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		ex := &exporter{
			client:  client,
			refresh: exporterRefresh,
		}
		if exporterPartitions != "" {
			ex.partitions = map[string]bool{}
			for _, p := range strings.Split(exporterPartitions, ",") {
				ex.partitions[strings.TrimSpace(p)] = true
			}
		}

		mux := http.NewServeMux()
		mux.HandleFunc("/metrics", ex.serveMetrics)
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "<html><body><h1>muster exporter</h1><p><a href=\"/metrics\">/metrics</a></p></body></html>")
		})

		srv := &http.Server{
			Addr:              exporterListen,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		}

		// Graceful shutdown on SIGINT/SIGTERM.
		shutdown := make(chan os.Signal, 1)
		signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-shutdown
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = srv.Shutdown(ctx)
		}()

		fmt.Fprintf(os.Stderr, "muster exporter listening on %s (refresh=%s)\n", exporterListen, exporterRefresh)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	},
}

func init() {
	exporterCmd.Flags().StringVar(&exporterListen, "listen", ":9100", "HTTP listen address")
	exporterCmd.Flags().DurationVar(&exporterRefresh, "refresh", 15*time.Second, "minimum interval between Slurm fetches (cache window)")
	exporterCmd.Flags().StringVar(&exporterPartitions, "partitions", "", "restrict metrics to these partitions (comma-separated, default = all)")
	rootCmd.AddCommand(exporterCmd)
}

// exporter holds the cached scrape result and serves /metrics handlers.
type exporter struct {
	client     slurm.Client
	refresh    time.Duration
	partitions map[string]bool

	mu        sync.Mutex
	cachedAt  time.Time
	cachedOut string
}

func (e *exporter) serveMetrics(w http.ResponseWriter, r *http.Request) {
	body, err := e.scrape(r.Context())
	if err != nil {
		http.Error(w, "scrape failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = io.WriteString(w, body)
}

// scrape returns the latest text-exposition body, refreshing if the cache is stale.
func (e *exporter) scrape(ctx context.Context) (string, error) {
	e.mu.Lock()
	if time.Since(e.cachedAt) < e.refresh && e.cachedOut != "" {
		out := e.cachedOut
		e.mu.Unlock()
		return out, nil
	}
	e.mu.Unlock()

	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	nodes, err := e.client.Nodes(ctx)
	if err != nil {
		return "", err
	}
	jobs, err := e.client.Jobs(ctx, "")
	if err != nil {
		return "", err
	}

	parts := aggregate.Partitions(nodes, jobs, "")
	if e.partitions != nil {
		filtered := parts[:0]
		for _, p := range parts {
			if e.partitions[p.Name] {
				filtered = append(filtered, p)
			}
		}
		parts = filtered
	}

	var b strings.Builder
	writePartitionMetrics(&b, parts)
	writeNodeMetrics(&b, nodes, e.partitions)
	fmt.Fprintf(&b, "# HELP slurm_scrape_seconds Time spent fetching Slurm state for this scrape\n")
	fmt.Fprintf(&b, "# TYPE slurm_scrape_seconds gauge\n")
	fmt.Fprintf(&b, "slurm_scrape_seconds %.3f\n", time.Since(start).Seconds())

	e.mu.Lock()
	e.cachedOut = b.String()
	e.cachedAt = time.Now()
	out := e.cachedOut
	e.mu.Unlock()
	return out, nil
}

func writePartitionMetrics(b *strings.Builder, parts []aggregate.PartitionSummary) {
	fmt.Fprintln(b, "# HELP slurm_partition_cpus CPUs in a partition by state")
	fmt.Fprintln(b, "# TYPE slurm_partition_cpus gauge")
	for _, p := range parts {
		fmt.Fprintf(b, "slurm_partition_cpus{partition=%q,state=\"alloc\"} %d\n", p.Name, p.AllocCPUs)
		fmt.Fprintf(b, "slurm_partition_cpus{partition=%q,state=\"total\"} %d\n", p.Name, p.TotalCPUs)
	}

	fmt.Fprintln(b, "# HELP slurm_partition_gpus GPUs in a partition by state")
	fmt.Fprintln(b, "# TYPE slurm_partition_gpus gauge")
	for _, p := range parts {
		if p.TotalGPUs == 0 {
			continue
		}
		model := p.GPUModel
		if model == "" {
			model = "unknown"
		}
		fmt.Fprintf(b, "slurm_partition_gpus{partition=%q,model=%q,state=\"alloc\"} %d\n", p.Name, model, p.AllocGPUs)
		fmt.Fprintf(b, "slurm_partition_gpus{partition=%q,model=%q,state=\"total\"} %d\n", p.Name, model, p.TotalGPUs)
	}

	fmt.Fprintln(b, "# HELP slurm_partition_memory_bytes Memory in a partition by state (bytes)")
	fmt.Fprintln(b, "# TYPE slurm_partition_memory_bytes gauge")
	for _, p := range parts {
		fmt.Fprintf(b, "slurm_partition_memory_bytes{partition=%q,state=\"alloc\"} %d\n", p.Name, int64(p.AllocMemMB)*1024*1024)
		fmt.Fprintf(b, "slurm_partition_memory_bytes{partition=%q,state=\"total\"} %d\n", p.Name, int64(p.TotalMemMB)*1024*1024)
	}

	fmt.Fprintln(b, "# HELP slurm_partition_nodes Node count in a partition by canonical state")
	fmt.Fprintln(b, "# TYPE slurm_partition_nodes gauge")
	for _, p := range parts {
		fmt.Fprintf(b, "slurm_partition_nodes{partition=%q,state=\"idle\"} %d\n", p.Name, p.NodeCounts.Idle)
		fmt.Fprintf(b, "slurm_partition_nodes{partition=%q,state=\"mixed\"} %d\n", p.Name, p.NodeCounts.Mixed)
		fmt.Fprintf(b, "slurm_partition_nodes{partition=%q,state=\"alloc\"} %d\n", p.Name, p.NodeCounts.Alloc)
		fmt.Fprintf(b, "slurm_partition_nodes{partition=%q,state=\"down\"} %d\n", p.Name, p.NodeCounts.Down)
		fmt.Fprintf(b, "slurm_partition_nodes{partition=%q,state=\"drain\"} %d\n", p.Name, p.NodeCounts.Drain)
		fmt.Fprintf(b, "slurm_partition_nodes{partition=%q,state=\"total\"} %d\n", p.Name, p.TotalNodes)
	}

	fmt.Fprintln(b, "# HELP slurm_partition_jobs Job count in a partition by state")
	fmt.Fprintln(b, "# TYPE slurm_partition_jobs gauge")
	for _, p := range parts {
		fmt.Fprintf(b, "slurm_partition_jobs{partition=%q,state=\"running\"} %d\n", p.Name, p.RunningJobs)
		fmt.Fprintf(b, "slurm_partition_jobs{partition=%q,state=\"pending\"} %d\n", p.Name, p.PendingJobs)
	}
}

func writeNodeMetrics(b *strings.Builder, nodes []slurm.Node, partitionFilter map[string]bool) {
	fmt.Fprintln(b, "# HELP slurm_node_state Per-node canonical state (1 = node is in this state)")
	fmt.Fprintln(b, "# TYPE slurm_node_state gauge")
	for _, n := range nodes {
		class := slurm.Classify(n.State).String()
		for _, p := range n.Partitions {
			if partitionFilter != nil && !partitionFilter[p] {
				continue
			}
			fmt.Fprintf(b, "slurm_node_state{node=%q,partition=%q,state=%q} 1\n", n.Name, p, class)
		}
	}
}
