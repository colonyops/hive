"""Append-only annotation store, keyed by ``content_sha256``.

Captures on disk are immutable and carry only Hive's ``weak_label``. Higher
quality labels (LLM silver, human gold) live here, in a separate append-only
JSONL file. Nothing is ever overwritten or deleted: re-labeling a capture just
appends another record, preserving a full audit trail and allowing prompt/model
versions to be revisited.

At read time, :meth:`AnnotationStore.resolve` collapses all records for each
capture to a single winning label by provenance rank (``human`` > ``llm`` >
``weak``), breaking ties by most-recent ``created_at``. The implicit ``weak``
tier comes from the capture itself, so it is never stored here.
"""

from __future__ import annotations

import json
import os
from dataclasses import dataclass
from pathlib import Path

from .schema import (
    LABEL_SOURCE_RANK,
    MODEL_STATES,
    Annotation,
    Capture,
)

# Sources that may be written to the store. "weak" is implicit (from the
# capture) and must never be persisted as an annotation.
WRITABLE_SOURCES: tuple[str, ...] = ("llm", "human")


class AnnotationError(ValueError):
    """Raised when an annotation is structurally invalid."""


def validate_annotation(ann: Annotation) -> None:
    if ann.state not in MODEL_STATES:
        raise AnnotationError(f"invalid state {ann.state!r}; must be one of {MODEL_STATES}")
    if ann.source not in WRITABLE_SOURCES:
        raise AnnotationError(f"invalid source {ann.source!r}; must be one of {WRITABLE_SOURCES}")
    if len(ann.content_sha256) != 64:
        raise AnnotationError(f"content_sha256 must be a 64-char hex digest, got {ann.content_sha256!r}")
    if ann.confidence is not None and not (0.0 <= ann.confidence <= 1.0):
        raise AnnotationError(f"confidence must be in [0, 1], got {ann.confidence}")
    for alt in ann.alternatives:
        if alt not in MODEL_STATES:
            raise AnnotationError(f"invalid alternative {alt!r}; must be one of {MODEL_STATES}")


@dataclass(frozen=True)
class ResolvedLabel:
    """The winning label for one capture after provenance resolution."""

    content_sha256: str
    state: str
    source: str  # "human" | "llm"
    annotation: Annotation
    superseded: int  # number of other annotations outranked for this capture


def _rank(ann: Annotation) -> tuple[int, float]:
    # Higher provenance rank wins; break ties by newest timestamp.
    return (LABEL_SOURCE_RANK.get(ann.source, -1), ann.created_at.timestamp())


class AnnotationStore:
    """Append-only JSONL annotation store."""

    def __init__(self, path: str | os.PathLike[str]):
        self.path = Path(path)

    # --- reading ----------------------------------------------------------

    def all(self) -> list[Annotation]:
        if not self.path.exists():
            return []
        out: list[Annotation] = []
        for lineno, line in enumerate(self.path.read_text(encoding="utf-8").splitlines(), 1):
            line = line.strip()
            if not line:
                continue
            try:
                out.append(Annotation.from_dict(json.loads(line)))
            except (json.JSONDecodeError, KeyError, ValueError) as exc:
                raise AnnotationError(f"{self.path}:{lineno}: corrupt annotation: {exc}") from exc
        return out

    def for_capture(self, content_sha256: str) -> list[Annotation]:
        return [a for a in self.all() if a.content_sha256 == content_sha256]

    def resolve(self) -> dict[str, ResolvedLabel]:
        """Collapse all annotations to one winning label per capture."""
        by_sha: dict[str, list[Annotation]] = {}
        for ann in self.all():
            by_sha.setdefault(ann.content_sha256, []).append(ann)

        resolved: dict[str, ResolvedLabel] = {}
        for sha, anns in by_sha.items():
            winner = max(anns, key=_rank)
            resolved[sha] = ResolvedLabel(
                content_sha256=sha,
                state=winner.state,
                source=winner.source,
                annotation=winner,
                superseded=len(anns) - 1,
            )
        return resolved

    # --- writing ----------------------------------------------------------

    def append(self, ann: Annotation) -> None:
        validate_annotation(ann)
        self.path.parent.mkdir(parents=True, exist_ok=True)
        line = json.dumps(ann.to_dict(), separators=(",", ":"))
        # Open in append mode; each write is a single line, so concurrent
        # appends interleave safely at line granularity.
        with self.path.open("a", encoding="utf-8") as f:
            f.write(line + "\n")

    def extend(self, anns: list[Annotation]) -> int:
        for ann in anns:
            validate_annotation(ann)
        self.path.parent.mkdir(parents=True, exist_ok=True)
        with self.path.open("a", encoding="utf-8") as f:
            for ann in anns:
                f.write(json.dumps(ann.to_dict(), separators=(",", ":")) + "\n")
        return len(anns)


def default_annotations_path() -> Path:
    """Default annotation store location."""
    data_home = os.environ.get("XDG_DATA_HOME")
    base = Path(data_home) if data_home else Path.home() / ".local" / "share"
    return base / "hive" / "corpus-annotations" / "annotations.jsonl"


def effective_label(capture: Capture, resolved: dict[str, ResolvedLabel]) -> tuple[str, str]:
    """Return ``(state, source)`` for a capture, falling back to its weak label.

    If an annotation exists, its resolved label/source wins. Otherwise the
    capture's weak label is mapped to a model state with source ``"weak"``.
    A weak label with no model mapping (e.g. ``missing``) yields ``unknown``.
    """
    r = resolved.get(capture.content_sha256)
    if r is not None:
        return r.state, r.source
    return (capture.weak_model_state or "unknown"), "weak"
