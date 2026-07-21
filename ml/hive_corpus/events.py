"""Controlled-lab manifest and scenario-event correlation.

The corpus lab (``~/.local/share/hive/corpus-lab/<run>/``) records:

- ``manifest.json`` — tmux pane IDs mapped to stable opaque ``pane_key`` values
- ``events.jsonl`` — timestamped controller intent + observed-state windows

Events describe *controller intent*, not exact per-capture truth. We associate
captures with the scenario window that contains them by ``pane_key`` and time,
and expose the expected state as **scenario context** — never as ground truth.
Transition-boundary frames (near a scenario change) are flagged so they can be
routed to human review rather than trusted.
"""

from __future__ import annotations

import json
from bisect import bisect_right
from dataclasses import dataclass
from datetime import datetime, timedelta
from pathlib import Path

from .schema import Capture, parse_timestamp


@dataclass(frozen=True)
class LabEvent:
    timestamp: datetime
    agent: str
    pane_key: str
    scenario: str
    expected_state: str
    action: str
    note: str = ""


@dataclass(frozen=True)
class LabManifest:
    created_at: datetime
    tmux_session: str
    session_key: str
    # agent name -> (tmux_pane, pane_key)
    panes: dict[str, tuple[str, str]]

    def pane_key_to_agent(self) -> dict[str, str]:
        return {pane_key: agent for agent, (_, pane_key) in self.panes.items()}


@dataclass(frozen=True)
class ScenarioContext:
    """Scenario context for a capture (NOT ground truth)."""

    scenario: str
    expected_state: str
    action: str
    note: str
    event_timestamp: datetime
    # Seconds between the governing event and the capture time.
    seconds_after_event: float
    # True when another event for the same pane occurs close in time, so the
    # capture may straddle a state transition and must be human-reviewed.
    near_boundary: bool


def load_manifest(path: str | Path) -> LabManifest:
    raw = json.loads(Path(path).read_text(encoding="utf-8"))
    panes = {
        agent: (info["tmux_pane"], info["pane_key"])
        for agent, info in raw["panes"].items()
    }
    return LabManifest(
        created_at=parse_timestamp(raw["created_at"]),
        tmux_session=raw["tmux_session"],
        session_key=raw["session_key"],
        panes=panes,
    )


def load_events(path: str | Path) -> list[LabEvent]:
    events: list[LabEvent] = []
    for line in Path(path).read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line:
            continue
        raw = json.loads(line)
        events.append(
            LabEvent(
                timestamp=parse_timestamp(raw["timestamp"]),
                agent=raw["agent"],
                pane_key=raw["pane_key"],
                scenario=raw["scenario"],
                expected_state=raw["expected_state"],
                action=raw["action"],
                note=raw.get("note", ""),
            )
        )
    return events


def correlate(
    captures: list[Capture],
    events: list[LabEvent],
    *,
    boundary_window: timedelta = timedelta(seconds=20),
) -> dict[str, ScenarioContext]:
    """Associate captures with the governing scenario event.

    Returns a mapping of ``content_sha256`` -> :class:`ScenarioContext` for
    captures whose ``pane_key`` and time fall within a known scenario window.
    A capture is governed by the most recent event (same pane) at or before its
    capture time. ``near_boundary`` marks captures within ``boundary_window`` of
    the next event for that pane.
    """
    # Group event timestamps per pane, sorted.
    by_pane: dict[str, list[LabEvent]] = {}
    for ev in events:
        by_pane.setdefault(ev.pane_key, []).append(ev)
    for evs in by_pane.values():
        evs.sort(key=lambda e: e.timestamp)

    result: dict[str, ScenarioContext] = {}
    for cap in captures:
        pane_events = by_pane.get(cap.pane_key)
        if not pane_events:
            continue
        times = [e.timestamp for e in pane_events]
        idx = bisect_right(times, cap.captured_at) - 1
        if idx < 0:
            # Capture predates the first event for this pane.
            continue
        governing = pane_events[idx]

        near_boundary = False
        if idx + 1 < len(pane_events):
            next_ev = pane_events[idx + 1]
            if next_ev.timestamp - cap.captured_at <= boundary_window:
                near_boundary = True
        # Also treat captures very close *after* the governing event as boundary.
        if cap.captured_at - governing.timestamp <= boundary_window and idx > 0:
            near_boundary = True

        result[cap.content_sha256] = ScenarioContext(
            scenario=governing.scenario,
            expected_state=governing.expected_state,
            action=governing.action,
            note=governing.note,
            event_timestamp=governing.timestamp,
            seconds_after_event=(cap.captured_at - governing.timestamp).total_seconds(),
            near_boundary=near_boundary,
        )
    return result


def find_latest_lab(root: str | Path | None = None) -> Path | None:
    """Return the most recent corpus-lab run directory, if any."""
    if root is None:
        root = Path.home() / ".local" / "share" / "hive" / "corpus-lab"
    root = Path(root)
    if not root.is_dir():
        return None
    runs = [d for d in root.iterdir() if d.is_dir() and (d / "manifest.json").exists()]
    if not runs:
        return None
    return max(runs, key=lambda d: d.name)
