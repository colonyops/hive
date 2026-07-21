"""Schema definitions and taxonomy for the terminal-status corpus.

The on-disk capture record is written by Hive's ``JSONCaptureRecorder``
(``internal/core/terminal/tmux/capture_recorder.go``). This module mirrors
that contract for reading and defines the model label taxonomy plus the
mapping from Hive's weak-label enum to model states.
"""

from __future__ import annotations

import hashlib
from dataclasses import dataclass, field
from datetime import datetime
from typing import Any

# --- Model label taxonomy -------------------------------------------------
#
# These are the states the runtime model predicts from visible pane content.
# ``missing`` is intentionally excluded: it is a direct tmux/process fact,
# not something inferred from screen content.

MODEL_STATES: tuple[str, ...] = (
    "working",
    "permission",
    "question",
    "idle",
    "unknown",
)

# Hive's StateTracker weak labels (internal/core/terminal/terminal.go).
WEAK_LABELS: tuple[str, ...] = ("active", "approval", "ready", "missing")

# Weak label -> model state. Weak labels are a low-quality prior used only
# for review-queue prioritization; they never become ground truth. ``missing``
# maps to None because it is not a model prediction target.
WEAK_TO_MODEL: dict[str, str | None] = {
    "active": "working",
    "approval": "permission",
    "ready": "idle",
    "missing": None,
}

# Label provenance ranking. Higher wins when resolving a final label.
LABEL_SOURCES: tuple[str, ...] = ("weak", "llm", "human")
LABEL_SOURCE_RANK: dict[str, int] = {name: i for i, name in enumerate(LABEL_SOURCES)}

CAPTURE_SCHEMA_VERSION = 2

REQUIRED_CAPTURE_FIELDS: tuple[str, ...] = (
    "schema_version",
    "captured_at",
    "run_id",
    "session_key",
    "pane_key",
    "content",
    "content_sha256",
    "weak_label",
    "weak_label_source",
)


def weak_to_model(weak_label: str) -> str | None:
    """Map a Hive weak label to a model state, or None if not a target."""
    return WEAK_TO_MODEL.get(weak_label)


def parse_timestamp(value: str) -> datetime:
    """Parse an RFC3339/ISO8601 timestamp, tolerating a trailing ``Z``."""
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


@dataclass(frozen=True)
class Capture:
    """A single content-addressed pane capture (schema v2)."""

    schema_version: int
    captured_at: datetime
    run_id: str
    session_key: str
    pane_key: str
    content: str
    content_sha256: str
    weak_label: str
    weak_label_source: str
    tool: str = ""
    # Absolute path this record was read from (not part of the on-disk schema).
    path: str = ""

    @property
    def line_count(self) -> int:
        return self.content.count("\n") + 1 if self.content else 0

    @property
    def byte_len(self) -> int:
        return len(self.content.encode("utf-8"))

    @property
    def weak_model_state(self) -> str | None:
        return weak_to_model(self.weak_label)

    def computed_sha256(self) -> str:
        return hashlib.sha256(self.content.encode("utf-8")).hexdigest()

    @classmethod
    def from_dict(cls, raw: dict[str, Any], *, path: str = "") -> "Capture":
        return cls(
            schema_version=int(raw["schema_version"]),
            captured_at=parse_timestamp(raw["captured_at"]),
            run_id=str(raw["run_id"]),
            session_key=str(raw["session_key"]),
            pane_key=str(raw["pane_key"]),
            content=str(raw["content"]),
            content_sha256=str(raw["content_sha256"]),
            weak_label=str(raw["weak_label"]),
            weak_label_source=str(raw["weak_label_source"]),
            tool=str(raw.get("tool", "")),
            path=path,
        )


@dataclass(frozen=True)
class ValidationIssue:
    """A single integrity problem found while reading a capture file."""

    path: str
    kind: str  # e.g. "corrupt_json", "hash_mismatch", "missing_field", "bad_mode"
    detail: str


@dataclass
class Annotation:
    """An append-only label for a capture, keyed by content_sha256."""

    content_sha256: str
    state: str  # one of MODEL_STATES
    source: str  # one of LABEL_SOURCES ("llm" | "human"); "weak" is implicit
    created_at: datetime
    confidence: float | None = None
    evidence: str = ""  # short quote from the screen justifying the label
    alternatives: list[str] = field(default_factory=list)
    needs_human: bool = False
    model: str = ""  # LLM model id, if source == "llm"
    prompt_version: str = ""  # labeling prompt version, if source == "llm"
    note: str = ""

    def to_dict(self) -> dict[str, Any]:
        return {
            "content_sha256": self.content_sha256,
            "state": self.state,
            "source": self.source,
            "created_at": self.created_at.isoformat(),
            "confidence": self.confidence,
            "evidence": self.evidence,
            "alternatives": self.alternatives,
            "needs_human": self.needs_human,
            "model": self.model,
            "prompt_version": self.prompt_version,
            "note": self.note,
        }

    @classmethod
    def from_dict(cls, raw: dict[str, Any]) -> "Annotation":
        return cls(
            content_sha256=str(raw["content_sha256"]),
            state=str(raw["state"]),
            source=str(raw["source"]),
            created_at=parse_timestamp(raw["created_at"]),
            confidence=raw.get("confidence"),
            evidence=str(raw.get("evidence", "")),
            alternatives=list(raw.get("alternatives", [])),
            needs_human=bool(raw.get("needs_human", False)),
            model=str(raw.get("model", "")),
            prompt_version=str(raw.get("prompt_version", "")),
            note=str(raw.get("note", "")),
        )
