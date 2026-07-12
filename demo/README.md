# demo/

Slurm `--json` fixtures that let muster run without a real cluster. Point
either the flag or the env var at this directory:

```bash
muster --fixtures ./demo dash
MUSTER_FIXTURES=./demo muster partitions
```

Files (all in Slurm wire format):

| File                          | Consumed by                     |
|-------------------------------|---------------------------------|
| `scontrol_nodes.json`         | `nodes`, `dash` (Nodes tab)     |
| `scontrol_partitions.json`    | `partitions`, `dash` (Partitions tab) |
| `squeue.json`                 | `jobs`, `queue`, `users`, `gpu`, `dash` |
| `sacct.json`                  | `history`, `usage`, `dash` (History tab) |

In fixture mode `scancel` is a no-op, `nvidia-smi` returns nothing, and
`sstat`-derived job efficiency is synthesized from wall-clock runtime so the
detail overlay renders realistically.

## Regenerating the fixtures

`squeue.json` and `sacct.json` timestamps are absolute Unix epochs. If they
start looking stale (jobs "started 6 months ago" etc.), regenerate:

```bash
python3 demo/gen_demo.py demo/
```

Node and partition fixtures are stable and can be hand-edited.
