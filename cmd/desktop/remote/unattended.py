"""Deterministic agent substitute, run only by the Docker remote's real launches."""

import json
import re
import sys
import time
import urllib.request
from pathlib import Path


def post(path, payload):
    request = urllib.request.Request(
        "http://127.0.0.1:19001/hooks/" + path,
        json.dumps(payload).encode(),
        headers={"Content-Type": "application/json"},
    )
    with urllib.request.urlopen(request, timeout=10) as response:
        assert response.status == 202


run, mode = re.search(r"unattended:([a-f0-9-]+):(scheduled|notification)", sys.argv[-1]).groups()
name = "unattended-" + run
title = ("Scheduled job " if mode == "scheduled" else "Processed notification ") + run
post(name + "/results", {"id": mode + "-" + run, "title": title})
if mode == "notification":
    post(name + "/incoming", {"id": run, "title": "Incoming " + run, "state": "done"})
Path(__file__).with_name(mode + ".json").write_text(
    json.dumps({"run": run, "mode": mode, "completedAt": time.time()})
)
# Remain alive beyond the launcher's early-exit check, without an AI account.
time.sleep(15)
