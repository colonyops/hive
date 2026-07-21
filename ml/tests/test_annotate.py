"""Tests for the append-only annotation store."""

from __future__ import annotations

from datetime import datetime, timedelta, timezone
from pathlib import Path

import pytest

from hive_corpus.annotate import (
    AnnotationError,
    AnnotationStore,
    effective_label,
)
from hive_corpus.schema import Annotation, Capture

BASE = datetime(2026, 7, 21, 12, 0, 0, tzinfo=timezone.utc)
SHA_A = "a" * 64
SHA_B = "b" * 64


def ann(sha: str, state: str, source: str, *, offset_s: float = 0.0, **kw) -> Annotation:
    return Annotation(
        content_sha256=sha,
        state=state,
        source=source,
        created_at=BASE + timedelta(seconds=offset_s),
        **kw,
    )


def make_capture(sha: str, weak: str) -> Capture:
    return Capture(
        schema_version=2,
        captured_at=BASE,
        run_id="r",
        session_key="s",
        pane_key="p",
        content="x",
        content_sha256=sha,
        weak_label=weak,
        weak_label_source="hive_state_tracker_v1",
    )


def test_append_and_read_roundtrip(tmp_path: Path):
    store = AnnotationStore(tmp_path / "a.jsonl")
    store.append(ann(SHA_A, "working", "llm", confidence=0.9, evidence="spinner"))
    got = store.all()
    assert len(got) == 1
    assert got[0].state == "working"
    assert got[0].confidence == 0.9
    assert got[0].evidence == "spinner"


def test_append_only_preserves_history(tmp_path: Path):
    store = AnnotationStore(tmp_path / "a.jsonl")
    store.append(ann(SHA_A, "idle", "llm", offset_s=0))
    store.append(ann(SHA_A, "question", "human", offset_s=10))
    assert len(store.all()) == 2  # nothing overwritten


def test_resolve_human_beats_llm(tmp_path: Path):
    store = AnnotationStore(tmp_path / "a.jsonl")
    store.append(ann(SHA_A, "idle", "llm", offset_s=100))  # later in time
    store.append(ann(SHA_A, "question", "human", offset_s=0))  # earlier but higher rank
    resolved = store.resolve()
    assert resolved[SHA_A].state == "question"
    assert resolved[SHA_A].source == "human"
    assert resolved[SHA_A].superseded == 1


def test_resolve_latest_wins_within_same_source(tmp_path: Path):
    store = AnnotationStore(tmp_path / "a.jsonl")
    store.append(ann(SHA_A, "working", "llm", offset_s=0))
    store.append(ann(SHA_A, "idle", "llm", offset_s=50))
    resolved = store.resolve()
    assert resolved[SHA_A].state == "idle"


def test_resolve_multiple_captures(tmp_path: Path):
    store = AnnotationStore(tmp_path / "a.jsonl")
    store.append(ann(SHA_A, "working", "llm"))
    store.append(ann(SHA_B, "permission", "human"))
    resolved = store.resolve()
    assert resolved[SHA_A].state == "working"
    assert resolved[SHA_B].state == "permission"


def test_effective_label_falls_back_to_weak(tmp_path: Path):
    store = AnnotationStore(tmp_path / "a.jsonl")
    store.append(ann(SHA_A, "question", "human"))
    resolved = store.resolve()

    annotated = make_capture(SHA_A, "active")
    state, source = effective_label(annotated, resolved)
    assert (state, source) == ("question", "human")

    unannotated = make_capture(SHA_B, "active")
    state, source = effective_label(unannotated, resolved)
    assert (state, source) == ("working", "weak")


def test_effective_label_missing_weak_maps_unknown(tmp_path: Path):
    store = AnnotationStore(tmp_path / "a.jsonl")
    cap = make_capture(SHA_A, "missing")
    state, source = effective_label(cap, store.resolve())
    assert (state, source) == ("unknown", "weak")


def test_invalid_state_rejected(tmp_path: Path):
    store = AnnotationStore(tmp_path / "a.jsonl")
    with pytest.raises(AnnotationError):
        store.append(ann(SHA_A, "bogus", "human"))


def test_weak_source_not_writable(tmp_path: Path):
    store = AnnotationStore(tmp_path / "a.jsonl")
    with pytest.raises(AnnotationError):
        store.append(ann(SHA_A, "idle", "weak"))


def test_bad_sha_rejected(tmp_path: Path):
    store = AnnotationStore(tmp_path / "a.jsonl")
    with pytest.raises(AnnotationError):
        store.append(ann("short", "idle", "human"))


def test_confidence_range_enforced(tmp_path: Path):
    store = AnnotationStore(tmp_path / "a.jsonl")
    with pytest.raises(AnnotationError):
        store.append(ann(SHA_A, "idle", "llm", confidence=1.5))


def test_bad_alternative_rejected(tmp_path: Path):
    store = AnnotationStore(tmp_path / "a.jsonl")
    with pytest.raises(AnnotationError):
        store.append(ann(SHA_A, "idle", "llm", alternatives=["nope"]))


def test_corrupt_line_raises(tmp_path: Path):
    p = tmp_path / "a.jsonl"
    p.write_text("{not json}\n")
    with pytest.raises(AnnotationError):
        AnnotationStore(p).all()


def test_empty_store(tmp_path: Path):
    store = AnnotationStore(tmp_path / "missing.jsonl")
    assert store.all() == []
    assert store.resolve() == {}
