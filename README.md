# muster

Readable, modern views over Slurm — static commands plus a tabbed, auto-refreshing
TUI dashboard. Wraps `sinfo`, `squeue`, `sacct`, `scontrol`, `sstat`, and
`scancel` (all with `--json` where supported) and turns job-by-job rows into
rollups you can scan at a glance.

```
$ muster partitions
╭───────────┬───────────────┬──────────────────────┬───────────────────────┬────────────────────────┬─────┬──────╮
│ PARTITION │ NODES I/M/A/D │ CPUS                 │ GPUS                  │ MEM                    │ RUN │ PEND │
├───────────┼───────────────┼──────────────────────┼───────────────────────┼────────────────────────┼─────┼──────┤
│ gpu       │ 0/3/0/0       │ 168/352   ████▊░░░░░ │ 3/4 a100   ███████▌░░ │ 1.1T/1.9T   █████▌░░░░ │  30 │   10 │
│ cpu       │ 3/6/0/0       │ 188/1088  █▋░░░░░░░░ │ 0/16 l40s  ░░░░░░░░░░ │ 5.0T/15.5T  ███▏░░░░░░ │  21 │   43 │
│ ml        │ 2/0/0/0       │ 0/224     ░░░░░░░░░░ │ 0/7 l40    ░░░░░░░░░░ │ 0/1.9T      ░░░░░░░░░░ │   0 │    0 │
╰───────────┴───────────────┴──────────────────────┴───────────────────────┴────────────────────────┴─────┴──────╯
```

## Requirements

- Slurm 23.x or newer (`--json` on the standard CLIs)
- Go 1.24+ to build

Shells out to the Slurm CLIs, so it works for any user with shell access to a
submit host. No REST API token required.

## Install

```bash
git clone <repo> muster
cd muster
./build.sh          # go build → bin/muster
make install        # ~/.local/bin/muster (PREFIX= overrides)
muster install-completion        # writes shell completion to the right place
```

## Static commands

Read-only views you can pipe into scripts. All accept `-p PARTITION`,
`--json`, `--no-color`, and `-u USER` where it makes sense.

| Command | What it shows |
|---|---|
| `muster partitions` | per-partition rollup: CPU/GPU/mem A/T with utilization bars, node-state counts, R/PD job counts |
| `muster nodes -p gpu` | per-node detail with users on each node; `--gpu` filters; `--show-jobs` expands |
| `muster jobs -p gpu` | running jobs sorted by `--sort cpus\|gpus\|mem\|runtime\|user`; `--top N`; runtime / time-limit progress bar |
| `muster users -p gpu` | per-user resource holdings (CPUs/GPUs/mem held, oldest run age) |
| `muster queue -p gpu` | pending jobs with reason codes translated to plain English |
| `muster history -p gpu --since 24h` | sacct rollup by user/account/state/partition with CPU/GPU-hours |
| `muster reservations` | active + upcoming reservations with `starts in / ends in` |
| `muster gpu -p gpu` | per-GPU (node, index) attribution — which user holds A100 #0/#1/#2 |
| `muster usage --since 7d` | efficiency: CPU-hours requested vs actually used, worst-offender per user |
| `muster explain <jobid>` | why isn't my job running? Per-node fit check + reason + eligible start |
| `muster logs <jobid> [-f] [--stderr]` | tail (or live `-f`) the job's stdout/stderr (resolved via scontrol) |
| `muster ssh <jobid>` | drop into a shell inside the job's allocation (`srun --pty`) |
| `muster snapshot [path]` | dump current state to JSON for later `muster diff` |
| `muster diff <old.json> [new.json]` | what changed since the snapshot — jobs added/gone/state-changed, nodes drained |
| `muster wait --until 'gpu.gpu_free >= 1'` | poll until condition, then ring bell / POST webhook |
| `muster version` | prints muster version and the detected Slurm version |

### Common one-liners

```bash
# Who's burning the partition right now?
muster jobs -p gpu --top 5

# Why is my pending job not running?
muster explain $(squeue --me -p gpu --noheader -o '%i' | head -1)

# Tail the stdout of my latest GPU job, live
muster logs $(muster jobs -p gpu -u $USER --sort runtime --json | jq '.[0].job_id') -f

# Send me a Slack ping when there's a free A100 on gpu
muster wait --until 'gpu.gpu_free >= 1' --webhook https://hooks.slack.com/services/...

# How efficient were my jobs this week?
muster usage --since 7d

# What's changed in the last 30 minutes?
muster snapshot /tmp/a.json && sleep 1800 && muster diff /tmp/a.json

# Pipe-friendly machine output
muster partitions --json | jq '.[] | select(.gpus_alloc < .gpus_total)'
```

## Interactive dashboard — `muster dash`

```
muster dash                    # cluster-wide; one card per partition
muster dash -p gpu             # partition-focused; with utilization sparklines
```

Tabbed, auto-refreshing TUI (10-second tick). Summary panel at the top
(partition card(s) + your card with running/pending/holdings/oldest job),
five tabs of detail below.

| Tab | Contents |
|---|---|
| 1 Partitions | the cluster summary table |
| 2 Nodes | per-node breakdown with users |
| 3 Jobs | running jobs sorted by resource |
| 4 Users | per-user rollup |
| 5 Queue | pending with reason-explained |
| 6 History | sacct rollup over the last 24h |

### Keybindings

| Key | Action |
|---|---|
| `1` – `6` | jump to a tab |
| `tab` / `shift+tab` / `h` `l` | next / prev tab |
| `j` `k` / `↑` `↓` | move row selection (with auto-scroll) |
| `g` / `G` | first / last row |
| `pgup` / `pgdn` | scroll one page |
| `mouse wheel` | scrolls the active viewport |
| `enter` | open detail overlay for the selected row |
| `space` | toggle row selection (Jobs/Queue, for bulk ops) |
| `c` | cancel cursor row, OR all selected if Space-marked any |
| `m` | toggle Me mode (filter to your jobs across tabs) |
| `s` | cycle sort key (Jobs/Users/Queue) |
| `/` | live filter on the current tab |
| `r` | refresh now |
| `?` | help overlay |
| `q` / `esc` | quit |

### Detail overlay (Enter on a job)

Centered card with the job's full metadata: state, partition, nodes, CPUs,
GPUs, memory, time-limit progress, working directory, command path, and
**live efficiency** from sstat (`3.2 / 8 cores (40%) · peak 6.4G`).

Below the metadata, a scrollable log viewer with up to 2000 trailing lines of
the job's stdout, auto-refreshing each tick. Long lines wrap to fit. Inside
the overlay:

- `↑/↓` `j/k` `pgup/pgdn` `g/G` scroll the log
- `/` search the log; `n`/`N` jump between matches
- `esc` / `q` close

For pending jobs the overlay shows requested resources and the reason; jobs
with no stdout (interactive zsh / RStudio) show a clear note rather than an
error.

### Visual cues

- **Sparklines** in `-p` mode show the last 5 minutes of partition utilization (CPU/GPU/mem)
- **Utilization bars** colored by load (green <50%, yellow <80%, red ≥80%)
- **Soft background tint + `▶` marker** on the cursor row (k9s / lazygit style)
- **Green `●`** prefix on jobs that appeared since the previous refresh
- **Cyan `✓`** prefix on Space-selected rows
- **`Me mode`** label in yellow when self-filter is on; **`N selected`** in cyan during multi-select

## Notes

- Slurm visibility: as a non-admin, sacct (`history`, `usage`) and squeue
  filtering may show only your own jobs depending on cluster policy.
- `sacct` is the slowest call (~10–20s on busy clusters). `dash` fetches it
  in parallel at startup so the UI is interactive in ~150ms either way; the
  History tab shows a dedicated "loading accounting history" message while
  it lands.
- Unknown pending reason codes pass through unchanged so nothing is hidden.

## Status

Production-ready for daily use. Read-only via static commands; bulk-cancel
and per-job `srun --pty` from the dash. No write paths beyond `scancel` /
`scontrol show` and `srun --pty` invocations.

Open follow-ups: REST API backend (so `dash` can run off-cluster), per-tab
saved layouts, `muster exporter` Prometheus endpoint.
