# muster

Readable views over Slurm partitions, nodes, users, and queue. `muster` wraps
the standard Slurm CLI tools (`sinfo`, `squeue`, `sacct`, `scontrol`) with
`--json` and presents human-friendly rollups instead of job-by-job rows.

```
$ muster partitions
+-----------+---------------+----------+-----------+------------+-----+------+
| PARTITION | NODES I/M/A/D | CPUS A/T | GPUS A/T  | MEM A/T    | RUN | PEND |
+-----------+---------------+----------+-----------+------------+-----+------+
| gpu       | 0/3/0/0       | 118/352  | 3/4 a100  | 682G/1.9T  |  17 |   10 |
| cpu      | 3/6/0/0       | 216/1088 | 0/16 l40s | 5.2T/15.5T |  30 |   43 |
| ml | 2/0/0/0       | 0/224    | 0/7 l40   | 0/1.9T     |   0 |    0 |
+-----------+---------------+----------+-----------+------------+-----+------+
Legend: I=idle  M=mixed  A=alloc  D=down/drain
```

## Requirements

- Slurm 23.x or newer (must support `--json` on the standard CLI tools)
- Go 1.24+ to build (`module load go/24.2` on the HPC cluster)

`muster` shells out to the Slurm CLIs, so it works for any user with shell
access to a Slurm submit host. No REST API token required.

## Build & install

```bash
git clone <repo> muster
cd muster
./build.sh           # loads Go via Lmod, builds bin/muster
make install         # installs to ~/.local/bin/muster
```

`make install` accepts `PREFIX=` to install elsewhere (e.g. `PREFIX=/opt`).

## Commands

```
muster partitions                              # per-partition rollup
muster nodes -p gpu [--gpu] [--show-jobs] [--state mixed,drain]
muster users -p gpu [--sort cpus|gpus|mem|jobs|age] [--top N] [--user demo-user]
muster queue  -p gpu [--all] [--reason BeginTime] [--sort priority|age|user]
muster history -p gpu --since 24h [--by user|account|state|partition]
muster version
```

Global flags (all subcommands):
- `-p, --partition NAME` filter to a single partition
- `-u, --user NAME`     filter to a single user (where applicable)
- `--json`              emit JSON instead of a table
- `--no-color`          disable ANSI colors (also honors `NO_COLOR` env)

## Examples

```bash
# Which GPU nodes are busy?
muster nodes -p gpu --gpu

# Who exactly has jobs on each node, with job IDs?
muster nodes -p gpu --show-jobs

# Top 5 CPU hogs right now
muster users -p gpu --sort cpus --top 5

# Why is my pending job not running?
muster queue -p gpu --user $USER

# CPU-hours by user over the last week
muster history -p gpu --since 7d --by user

# Pipe-friendly JSON for scripting
muster partitions --json | jq '.[] | select(.gpus_alloc < .gpus_total)'
```

## Notes

- `muster users` and `muster history` aggregate across all visible jobs. As a
  non-admin you may only see your own; ask an admin if you need cluster-wide
  visibility.
- `muster history` queries `sacct`; long windows (>1d) take seconds.
- Slurm reason codes in `muster queue` are translated to plain English; unknown
  codes pass through unchanged so nothing is hidden.

## Status

v1. Read-only. Future ideas: REST API backend, watch/refresh mode, shell
completion, multi-cluster federation.
