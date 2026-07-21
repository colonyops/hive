"""Tests for the corpus reader/validator using synthetic fixtures."""

from __future__ import annotations

import hashlib
import json
import os
import stat
from datetime import datetime, timezone
from pathlib import Path

import pytest

from hive_corpus.reader import read_capture_file, scan_corpus


def write_capture(
    dir_path: Path,
    content: str,
    *,
    weak_label: str = "active",
    tool: str = "pi",
    filename: str | None = None,
    mode: int = 0o600,
    overrides: dict | None = None,
) -> Path:
    sha = hashlib.sha256(content.encode("utf-8")).hexdigest()
    record = {
        "schema_version": 2,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "run_id": "run0",
        "session_key": "sess0",
        "pane_key": "pane0",
        "tool": tool,
        "content": content,
        "content_sha256": sha,
        "weak_label": weak_label,
        "weak_label_source": "hive_state_tracker_v1",
    }
    if overrides:
        record.update(overrides)
    name = filename if filename is not None else f"{sha}.json"
    path = dir_path / name
    path.write_text(json.dumps(record), encoding="utf-8")
    os.chmod(path, mode)
    return path


def test_valid_capture_reads_clean(tmp_path: Path):
    write_capture(tmp_path, "hello world\n$ ")
    result = scan_corpus(tmp_path)
    assert result.ok
    assert len(result.captures) == 1
    cap = result.captures[0]
    assert cap.tool == "pi"
    assert cap.weak_model_state == "working"


def test_hash_mismatch_detected(tmp_path: Path):
    p = write_capture(tmp_path, "content A")
    # Corrupt content_sha256 without renaming the file.
    raw = json.loads(p.read_text())
    raw["content"] = "tampered content"
    p.write_text(json.dumps(raw))
    _, issues = read_capture_file(p)
    kinds = {i.kind for i in issues}
    assert "hash_mismatch" in kinds


def test_filename_mismatch_detected(tmp_path: Path):
    write_capture(tmp_path, "abc", filename="deadbeef.json")
    result = scan_corpus(tmp_path)
    kinds = {i.kind for i in result.issues}
    assert "filename_mismatch" in kinds


def test_corrupt_json_detected(tmp_path: Path):
    p = tmp_path / ("a" * 64 + ".json")
    p.write_text("{ not valid json")
    os.chmod(p, 0o600)
    result = scan_corpus(tmp_path)
    assert not result.ok
    assert any(i.kind == "corrupt_json" for i in result.issues)


def test_missing_field_detected(tmp_path: Path):
    p = tmp_path / ("b" * 64 + ".json")
    p.write_text(json.dumps({"schema_version": 2, "content": "x"}))
    os.chmod(p, 0o600)
    result = scan_corpus(tmp_path)
    assert any(i.kind == "missing_field" for i in result.issues)


def test_bad_mode_detected(tmp_path: Path):
    write_capture(tmp_path, "world-readable", mode=0o644)
    result = scan_corpus(tmp_path)
    assert any(i.kind == "bad_mode" for i in result.issues)


def test_bad_mode_skippable(tmp_path: Path):
    write_capture(tmp_path, "world-readable", mode=0o644)
    result = scan_corpus(tmp_path, check_mode=False)
    assert result.ok


def test_unknown_weak_label_detected(tmp_path: Path):
    write_capture(tmp_path, "weird", weak_label="frobnicated")
    result = scan_corpus(tmp_path)
    assert any(i.kind == "unknown_weak_label" for i in result.issues)


def test_schema_version_mismatch(tmp_path: Path):
    write_capture(tmp_path, "old", overrides={"schema_version": 1})
    result = scan_corpus(tmp_path)
    assert any(i.kind == "schema_version" for i in result.issues)


def test_identity_key_and_temp_skipped(tmp_path: Path):
    write_capture(tmp_path, "real")
    (tmp_path / ".identity.key").write_bytes(b"\x00" * 32)
    (tmp_path / ".capture-xyz.tmp").write_text("partial")
    result = scan_corpus(tmp_path)
    assert len(result.captures) == 1
    assert result.ok
