"""Tests for controlled-lab event correlation using synthetic fixtures."""

from __future__ import annotations

import json
from datetime import datetime, timedelta, timezone
from pathlib import Path

from hive_corpus.events import correlate, load_events, load_manifest
from hive_corpus.schema import Capture

BASE = datetime(2026, 7, 21, 15, 0, 0, tzinfo=timezone.utc)


def make_capture(pane_key: str, offset_s: float, content: str, weak: str = "active") -> Capture:
    return Capture(
        schema_version=2,
        captured_at=BASE + timedelta(seconds=offset_s),
        run_id="run0",
        session_key="sess0",
        pane_key=pane_key,
        content=content,
        content_sha256=content,  # fine for correlation tests (used only as key)
        weak_label=weak,
        weak_label_source="hive_state_tracker_v1",
    )


def write_lab(tmp_path: Path) -> Path:
    manifest = {
        "created_at": BASE.isoformat(),
        "tmux_session": "test",
        "session_key": "sess0",
        "panes": {"pi": {"tmux_pane": "%1", "pane_key": "paneP"}},
    }
    (tmp_path / "manifest.json").write_text(json.dumps(manifest))
    events = [
        {"timestamp": BASE.isoformat(), "agent": "pi", "pane_key": "paneP",
         "scenario": "startup-idle", "expected_state": "idle", "action": "observed", "note": ""},
        {"timestamp": (BASE + timedelta(seconds=60)).isoformat(), "agent": "pi", "pane_key": "paneP",
         "scenario": "tool-task", "expected_state": "working", "action": "submitted", "note": ""},
    ]
    with (tmp_path / "events.jsonl").open("w") as f:
        for e in events:
            f.write(json.dumps(e) + "\n")
    return tmp_path


def test_manifest_and_events_load(tmp_path: Path):
    write_lab(tmp_path)
    manifest = load_manifest(tmp_path / "manifest.json")
    assert manifest.panes["pi"] == ("%1", "paneP")
    assert manifest.pane_key_to_agent()["paneP"] == "pi"
    events = load_events(tmp_path / "events.jsonl")
    assert len(events) == 2


def test_capture_governed_by_latest_prior_event(tmp_path: Path):
    write_lab(tmp_path)
    events = load_events(tmp_path / "events.jsonl")
    # 30s in -> still under startup-idle window.
    cap = make_capture("paneP", 30, "c1")
    ctx = correlate([cap], events)
    assert ctx["c1"].scenario == "startup-idle"
    assert ctx["c1"].expected_state == "idle"


def test_capture_after_second_event(tmp_path: Path):
    write_lab(tmp_path)
    events = load_events(tmp_path / "events.jsonl")
    cap = make_capture("paneP", 90, "c2")
    ctx = correlate([cap], events)
    assert ctx["c2"].scenario == "tool-task"


def test_capture_before_first_event_uncorrelated(tmp_path: Path):
    write_lab(tmp_path)
    events = load_events(tmp_path / "events.jsonl")
    cap = make_capture("paneP", -10, "c3")
    ctx = correlate([cap], events)
    assert "c3" not in ctx


def test_near_boundary_flagged(tmp_path: Path):
    write_lab(tmp_path)
    events = load_events(tmp_path / "events.jsonl")
    # 55s in -> within 20s before the 60s event -> boundary.
    cap = make_capture("paneP", 55, "c4")
    ctx = correlate([cap], events)
    assert ctx["c4"].near_boundary is True


def test_unknown_pane_uncorrelated(tmp_path: Path):
    write_lab(tmp_path)
    events = load_events(tmp_path / "events.jsonl")
    cap = make_capture("otherPane", 30, "c5")
    ctx = correlate([cap], events)
    assert "c5" not in ctx
