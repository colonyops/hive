"""Corpus reader and integrity validator.

Reads schema-v2 content-addressed capture files and verifies:

- valid JSON and required fields present
- ``schema_version`` matches the expected version
- filename stem equals ``content_sha256`` equals ``sha256(content)``
- ``weak_label`` is a known value
- file mode is ``0600`` (best-effort; skipped on filesystems without POSIX modes)

Malformed records are collected as :class:`ValidationIssue` rather than raising,
so a single corrupt file never aborts a corpus scan.
"""

from __future__ import annotations

import json
import os
import stat
from collections.abc import Iterator
from dataclasses import dataclass
from pathlib import Path

from .schema import (
    CAPTURE_SCHEMA_VERSION,
    REQUIRED_CAPTURE_FIELDS,
    WEAK_LABELS,
    Capture,
    ValidationIssue,
)

# The recorder's private identity key lives alongside captures; never read as data.
IDENTITY_KEY_FILENAME = ".identity.key"


@dataclass
class ScanResult:
    """Outcome of scanning a corpus directory."""

    captures: list[Capture]
    issues: list[ValidationIssue]
    skipped: int = 0  # non-capture files ignored (e.g. .identity.key, temp files)

    @property
    def ok(self) -> bool:
        return not self.issues


def _iter_json_files(root: Path) -> Iterator[Path]:
    for entry in sorted(root.iterdir()):
        if not entry.is_file():
            continue
        if entry.name == IDENTITY_KEY_FILENAME:
            continue
        if entry.name.startswith(".capture-") and entry.name.endswith(".tmp"):
            continue
        if entry.suffix != ".json":
            continue
        yield entry


def _check_mode(path: Path, issues: list[ValidationIssue]) -> None:
    try:
        mode = stat.S_IMODE(path.stat().st_mode)
    except OSError as exc:  # pragma: no cover - unusual FS error
        issues.append(ValidationIssue(str(path), "stat_error", str(exc)))
        return
    # Only flag if group/other have any access. On some filesystems modes are
    # not meaningful; a 0600 file is the contract Hive writes.
    if mode & (stat.S_IRWXG | stat.S_IRWXO):
        issues.append(
            ValidationIssue(str(path), "bad_mode", f"expected 0600, found {mode:04o}")
        )


def read_capture_file(path: Path, *, check_mode: bool = True) -> tuple[Capture | None, list[ValidationIssue]]:
    """Read and validate a single capture file.

    Returns ``(capture, issues)``. ``capture`` is None only when the record
    cannot be parsed into a :class:`Capture` at all.
    """
    issues: list[ValidationIssue] = []

    if check_mode:
        _check_mode(path, issues)

    try:
        raw = json.loads(path.read_text(encoding="utf-8"))
    except (json.JSONDecodeError, UnicodeDecodeError) as exc:
        issues.append(ValidationIssue(str(path), "corrupt_json", str(exc)))
        return None, issues
    except OSError as exc:  # pragma: no cover
        issues.append(ValidationIssue(str(path), "read_error", str(exc)))
        return None, issues

    if not isinstance(raw, dict):
        issues.append(ValidationIssue(str(path), "corrupt_json", "top-level value is not an object"))
        return None, issues

    missing = [f for f in REQUIRED_CAPTURE_FIELDS if f not in raw]
    if missing:
        issues.append(ValidationIssue(str(path), "missing_field", f"missing: {', '.join(missing)}"))
        return None, issues

    try:
        capture = Capture.from_dict(raw, path=str(path))
    except (ValueError, KeyError, TypeError) as exc:
        issues.append(ValidationIssue(str(path), "bad_field", str(exc)))
        return None, issues

    if capture.schema_version != CAPTURE_SCHEMA_VERSION:
        issues.append(
            ValidationIssue(
                str(path),
                "schema_version",
                f"expected {CAPTURE_SCHEMA_VERSION}, found {capture.schema_version}",
            )
        )

    stem = path.stem
    if stem != capture.content_sha256:
        issues.append(
            ValidationIssue(str(path), "filename_mismatch", f"stem {stem} != content_sha256 {capture.content_sha256}")
        )

    computed = capture.computed_sha256()
    if computed != capture.content_sha256:
        issues.append(
            ValidationIssue(str(path), "hash_mismatch", f"sha256(content) {computed} != content_sha256 {capture.content_sha256}")
        )

    if capture.weak_label not in WEAK_LABELS:
        issues.append(ValidationIssue(str(path), "unknown_weak_label", capture.weak_label))

    return capture, issues


def scan_corpus(root: str | os.PathLike[str], *, check_mode: bool = True) -> ScanResult:
    """Scan a corpus directory, returning valid captures and any issues."""
    root_path = Path(root)
    if not root_path.is_dir():
        raise NotADirectoryError(f"corpus directory not found: {root_path}")

    captures: list[Capture] = []
    issues: list[ValidationIssue] = []
    skipped = 0

    seen_files = set()
    for path in _iter_json_files(root_path):
        seen_files.add(path.name)
        capture, file_issues = read_capture_file(path, check_mode=check_mode)
        issues.extend(file_issues)
        if capture is not None:
            captures.append(capture)

    # Count files we intentionally ignored for transparency.
    for entry in root_path.iterdir():
        if entry.is_file() and entry.name not in seen_files:
            skipped += 1

    return ScanResult(captures=captures, issues=issues, skipped=skipped)


def default_corpus_dir() -> Path:
    """The default local capture directory Hive writes to."""
    data_home = os.environ.get("XDG_DATA_HOME")
    base = Path(data_home) if data_home else Path.home() / ".local" / "share"
    return base / "hive" / "recordings" / "tmux"
