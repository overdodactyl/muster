#!/usr/bin/env python3
"""Generate demo/squeue.json and demo/sacct.json for muster's --fixtures mode."""
import json, sys
from pathlib import Path

# Anchor: 2026-07-12 09:00:00 UTC. Kept as a constant so screenshots recorded
# soon after this file was authored look natural. Feel free to bump this and
# rerun when the fixtures start feeling stale.
import time
NOW = int(time.time())

def n(v, s=True, inf=False):
    return {"set": s, "infinite": inf, "number": v}

def job(*, jid, user, account, name, part, state, reason="None",
        nodes="", node_count=1, cpus=4, mem_per_cpu=0, mem_per_node=0,
        gres="", gres_detail=None,
        submit_offset, start_offset=0, end_offset=0,
        priority=1000, time_limit=1440,
        array_job=0, array_task=-1,
        dependency=""):
    end = NOW + end_offset if state != "RUNNING" and end_offset else 0
    start = NOW + start_offset if start_offset else 0
    submit = NOW + submit_offset
    j = {
        "job_id": jid,
        "user_name": user,
        "account": account,
        "name": name,
        "partition": part,
        "job_state": [state],
        "state_reason": reason,
        "nodes": nodes,
        "node_count": n(node_count),
        "cpus": n(cpus),
        "memory_per_cpu": n(mem_per_cpu),
        "memory_per_node": n(mem_per_node),
        "tres_per_node": gres,
        "submit_time": n(submit),
        "start_time": n(start),
        "end_time": n(end),
        "priority": n(priority),
        "time_limit": n(time_limit),
    }
    if gres_detail:
        j["gres_detail"] = gres_detail
    if dependency:
        j["dependency"] = dependency
    if array_job:
        j["array_job_id"] = n(array_job)
        j["array_task_id"] = n(array_task) if array_task >= 0 else {"set": False, "infinite": False, "number": 0}
    return j

MIN = 60
HR = 3600
DAY = 86400

jobs = [
    # === Running on gpu ===
    job(jid=9310042, user="alice", account="labA", name="stable-diffusion-train",
        part="gpu", state="RUNNING", nodes="node006a",
        cpus=24, mem_per_node=200000, gres="gres/gpu:2",
        gres_detail=["gpu:a100:2(IDX:0-1)"],
        submit_offset=-6*HR - 10*MIN, start_offset=-6*HR,
        priority=5200, time_limit=24*60),

    job(jid=9310058, user="carol", account="labC", name="resnet50-hparam",
        part="gpu", state="RUNNING", nodes="node006a",
        cpus=8, mem_per_node=64000, gres="gres/gpu:1",
        gres_detail=["gpu:a100:1(IDX:2)"],
        submit_offset=-2*HR - 34*MIN, start_offset=-2*HR - 30*MIN,
        priority=3100, time_limit=12*60),

    # === Running on cpu ===
    job(jid=9310071, user="bob", account="labB", name="qc-pipeline",
        part="cpu", state="RUNNING", nodes="node002a",
        cpus=32, mem_per_node=64000,
        submit_offset=-3*DAY, start_offset=-3*DAY + 5*MIN,
        priority=1400, time_limit=7*24*60),

    job(jid=9310089, user="dave", account="labB", name="fastqc-batch",
        part="cpu", state="RUNNING", nodes="node003a",
        cpus=16, mem_per_node=32000,
        submit_offset=-4*HR, start_offset=-4*HR + 2*MIN,
        priority=2000, time_limit=8*60),

    # === Array job (running tasks + one pending) ===
    *[job(jid=9310110+i, user="eve", account="labA", name="bootstrap-sample",
          part="cpu", state="RUNNING", nodes=f"node00{7 if i<2 else 12}a",
          cpus=8, mem_per_node=16000,
          submit_offset=-90*MIN, start_offset=-88*MIN,
          array_job=9310110, array_task=i,
          priority=1800, time_limit=4*60)
      for i in range(3)],
    job(jid=9310113, user="eve", account="labA", name="bootstrap-sample",
        part="cpu", state="PENDING", reason="Resources",
        cpus=8, mem_per_node=16000,
        submit_offset=-90*MIN,
        array_job=9310110, array_task=3,
        priority=1800, time_limit=4*60),

    # === Interactive on gpu ===
    job(jid=9310128, user="frank", account="labC", name="interactive",
        part="gpu", state="RUNNING", nodes="node013a",
        cpus=4, mem_per_node=16000, gres="gres/gpu:0",
        submit_offset=-45*MIN, start_offset=-44*MIN,
        priority=2500, time_limit=6*60),

    # === Pending: Priority queue ===
    job(jid=9310140, user="alice", account="labA", name="finetune-lora",
        part="gpu", state="PENDING", reason="Priority",
        cpus=16, mem_per_node=128000, gres="gres/gpu:2",
        submit_offset=-20*MIN,
        priority=5000, time_limit=6*60),

    # === Pending: Resources ===
    job(jid=9310142, user="grace", account="labD", name="big-inference",
        part="gpu", state="PENDING", reason="Resources",
        cpus=24, mem_per_node=200000, gres="gres/gpu:4",
        submit_offset=-15*MIN,
        priority=4200, time_limit=2*60),

    # === Pending: dependency on 9310042 ===
    job(jid=9310144, user="alice", account="labA", name="eval-lora",
        part="gpu", state="PENDING", reason="Dependency",
        cpus=4, mem_per_node=32000, gres="gres/gpu:1",
        submit_offset=-19*MIN,
        priority=5100, time_limit=60,
        dependency="afterok:9310042"),

    # === Pending on ml partition ===
    job(jid=9310146, user="dave", account="labB", name="l40-benchmark",
        part="ml", state="PENDING", reason="ReqNodeNotAvail",
        cpus=12, mem_per_node=48000, gres="gres/gpu:2",
        submit_offset=-8*MIN,
        priority=1500, time_limit=3*60),

    # === Pending on cpu ===
    job(jid=9310147, user="bob", account="labB", name="align-cohort",
        part="cpu", state="PENDING", reason="Priority",
        cpus=64, mem_per_node=256000,
        submit_offset=-5*MIN,
        priority=1300, time_limit=12*60),

    # === Pending: BeginTime ===
    job(jid=9310148, user="grace", account="labD", name="nightly-scan",
        part="cpu", state="PENDING", reason="BeginTime",
        cpus=32, mem_per_node=64000,
        submit_offset=-2*MIN, start_offset=6*HR,
        priority=800, time_limit=4*60),
]

sacct = []
# Recent successes + a few failures for the history view.
for i, (name, user, part, gres, cpus, mem, dur_min, state, exit_code, since_h) in enumerate([
    ("stable-diffusion-train", "alice", "gpu", "gres/gpu:2", 24, 200000, 47*60,  "COMPLETED", 0, 1),
    ("resnet50-hparam",        "carol", "gpu", "gres/gpu:1",  8,  64000, 12*60,  "COMPLETED", 0, 4),
    ("resnet50-hparam",        "carol", "gpu", "gres/gpu:1",  8,  64000, 11*60,  "COMPLETED", 0, 22),
    ("qc-pipeline",            "bob",   "cpu", "",           32,  64000, 3*24*60,"COMPLETED", 0, 8),
    ("fastqc-batch",           "dave",  "cpu", "",           16,  32000, 6*60,   "COMPLETED", 0, 5),
    ("bootstrap-sample",       "eve",   "cpu", "",            8,  16000, 210,    "COMPLETED", 0, 3),
    ("bootstrap-sample",       "eve",   "cpu", "",            8,  16000, 208,    "COMPLETED", 0, 15),
    ("l40-benchmark",          "dave",  "ml",  "gres/gpu:2", 12,  48000, 90,     "COMPLETED", 0, 2),
    ("finetune-lora",          "alice", "gpu", "gres/gpu:2", 16, 128000, 5*60,   "COMPLETED", 0, 26),
    ("eval-lora",              "alice", "gpu", "gres/gpu:1",  4,  32000, 45,     "COMPLETED", 0, 27),
    ("interactive",            "frank", "gpu", "",            4,  16000, 3*60,   "COMPLETED", 0, 12),
    ("preprocess-cohort",      "bob",   "cpu", "",           16,  32000, 4*60,   "FAILED",    2, 19),
    ("bad-cli",                "grace", "cpu", "",            8,   8000, 1,      "FAILED",    127, 21),
    ("out-of-memory",          "dave",  "cpu", "",           16,  16000, 45,     "OUT_OF_MEMORY", 137, 30),
    ("hparam-search",          "carol", "gpu", "gres/gpu:1",  8,  64000, 30*60,  "TIMEOUT",   0, 48),
    ("segment-brains",         "grace", "ml",  "gres/gpu:1", 12,  48000, 8*60,   "COMPLETED", 0, 33),
    ("segment-brains",         "grace", "ml",  "gres/gpu:1", 12,  48000, 8*60,   "COMPLETED", 0, 40),
    ("qc-pipeline",            "bob",   "cpu", "",           32,  64000, 25*60,  "COMPLETED", 0, 60),
    ("small-test",             "frank", "gpu", "gres/gpu:1",  4,   8000, 12,     "COMPLETED", 0, 6),
    ("small-test",             "frank", "gpu", "gres/gpu:1",  4,   8000, 15,     "COMPLETED", 0, 9),
    ("small-test",             "frank", "gpu", "gres/gpu:1",  4,   8000, 13,     "COMPLETED", 0, 11),
]):
    end = NOW - since_h * HR
    start = end - dur_min * MIN
    submit = start - 20
    account = {"alice":"labA","bob":"labB","carol":"labC","dave":"labB","eve":"labA","frank":"labC","grace":"labD"}[user]
    entry = {
        "job_id": 9309000 + i,
        "name": name,
        "user": user,
        "account": account,
        "partition": part,
        "state": {"current": [state], "reason": ""},
        "time": {
            "submission": submit,
            "start": start,
            "end": end,
            "elapsed": dur_min * MIN,
            "total": {"seconds": int(dur_min * MIN * cpus * 0.72), "microseconds": 0},
        },
        "exit_code": {"return_code": n(exit_code)},
        "tres": {
            "allocated": [
                {"type": "cpu", "name": "", "count": n(cpus)},
                {"type": "mem", "name": "", "count": n(mem)},
            ] + ([{"type": "gres", "name": "gpu", "count": n(int(gres.split(':')[-1]) if gres else 0)}] if gres else []),
        },
    }
    sacct.append(entry)

out = Path(sys.argv[1]) if len(sys.argv) > 1 else Path("demo")
(out / "squeue.json").write_text(json.dumps({"jobs": jobs}, indent=2) + "\n")
(out / "sacct.json").write_text(json.dumps({"jobs": sacct}, indent=2) + "\n")
print(f"wrote {out}/squeue.json ({len(jobs)} jobs) and {out}/sacct.json ({len(sacct)} jobs)")
