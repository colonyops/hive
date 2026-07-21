"""Command-line entry point for the corpus pipeline.

Subcommands:

    validate    Scan a corpus directory and report integrity issues.
    summary     Print a distribution summary (tools, labels, sessions, sizes).
    correlate   Join captures with controlled-lab scenario events.

More stages (annotate, label, review, export, train) are added as the
pipeline grows; this base ships stdlib-only.
"""

from __future__ import annotations

import argparse
import json
import sys
from collections import Counter
from datetime import datetime, timezone
from pathlib import Path

from .annotate import (
    WRITABLE_SOURCES,
    AnnotationStore,
    default_annotations_path,
    effective_label,
)
from .events import (
    correlate,
    find_latest_lab,
    load_events,
    load_manifest,
)
from .reader import default_corpus_dir, scan_corpus
from .schema import MODEL_STATES, Annotation, weak_to_model


def _add_corpus_arg(p: argparse.ArgumentParser) -> None:
    p.add_argument(
        "--corpus",
        type=Path,
        default=None,
        help="corpus directory (default: $XDG_DATA_HOME/hive/recordings/tmux)",
    )
    p.add_argument(
        "--no-check-mode",
        action="store_true",
        help="skip file-permission (0600) checks",
    )


def _resolve_corpus(args: argparse.Namespace) -> Path:
    return args.corpus if args.corpus is not None else default_corpus_dir()


def cmd_validate(args: argparse.Namespace) -> int:
    corpus = _resolve_corpus(args)
    result = scan_corpus(corpus, check_mode=not args.no_check_mode)

    by_kind = Counter(i.kind for i in result.issues)
    report = {
        "corpus": str(corpus),
        "captures": len(result.captures),
        "issues": len(result.issues),
        "issues_by_kind": dict(by_kind),
        "skipped_files": result.skipped,
        "ok": result.ok,
    }
    if args.json:
        print(json.dumps(report, indent=2))
    else:
        print(f"corpus:  {corpus}")
        print(f"valid:   {len(result.captures)} captures")
        print(f"issues:  {len(result.issues)}")
        for kind, n in by_kind.most_common():
            print(f"  {kind}: {n}")
        print(f"skipped: {result.skipped} non-capture files")
        print("OK" if result.ok else "INTEGRITY ISSUES FOUND")

    if args.show_issues and result.issues:
        for issue in result.issues[: args.show_issues]:
            print(f"  [{issue.kind}] {issue.path}: {issue.detail}", file=sys.stderr)

    return 0 if result.ok else 1


def cmd_summary(args: argparse.Namespace) -> int:
    corpus = _resolve_corpus(args)
    result = scan_corpus(corpus, check_mode=not args.no_check_mode)
    caps = result.captures

    if not caps:
        print("no captures found", file=sys.stderr)
        return 1

    tools = Counter(c.tool or "(none)" for c in caps)
    weak = Counter(c.weak_label for c in caps)
    model_states = Counter(weak_to_model(c.weak_label) or "(non-model)" for c in caps)
    sessions = Counter(c.session_key for c in caps)
    panes = Counter(c.pane_key for c in caps)
    runs = Counter(c.run_id for c in caps)

    byte_lens = sorted(c.byte_len for c in caps)
    line_counts = sorted(c.line_count for c in caps)
    times = sorted(c.captured_at for c in caps)

    def pct(sorted_vals: list[int], p: float) -> int:
        if not sorted_vals:
            return 0
        idx = min(len(sorted_vals) - 1, int(p * len(sorted_vals)))
        return sorted_vals[idx]

    # Dominance: share of the single busiest pane.
    top_pane_share = panes.most_common(1)[0][1] / len(caps)

    summary = {
        "corpus": str(corpus),
        "captures": len(caps),
        "issues": len(result.issues),
        "tools": dict(tools.most_common()),
        "weak_labels": dict(weak.most_common()),
        "model_states": dict(model_states.most_common()),
        "sessions": len(sessions),
        "panes": len(panes),
        "runs": len(runs),
        "top_pane_share": round(top_pane_share, 4),
        "bytes": {
            "min": byte_lens[0],
            "median": pct(byte_lens, 0.5),
            "p95": pct(byte_lens, 0.95),
            "max": byte_lens[-1],
        },
        "lines": {
            "min": line_counts[0],
            "median": pct(line_counts, 0.5),
            "max": line_counts[-1],
        },
        "time_range": {
            "first": times[0].isoformat(),
            "last": times[-1].isoformat(),
        },
    }

    if args.json:
        print(json.dumps(summary, indent=2))
    else:
        print(f"corpus:   {corpus}")
        print(f"captures: {len(caps)}  (issues: {len(result.issues)})")
        print(f"sessions: {summary['sessions']}  panes: {summary['panes']}  runs: {summary['runs']}")
        print(f"top pane share: {top_pane_share:.1%}")
        print("tools:")
        for k, n in tools.most_common():
            print(f"  {k}: {n} ({n / len(caps):.1%})")
        print("weak labels -> model state:")
        for k, n in weak.most_common():
            print(f"  {k} -> {weak_to_model(k)}: {n} ({n / len(caps):.1%})")
        b = summary["bytes"]
        print(f"content bytes: min={b['min']} median={b['median']} p95={b['p95']} max={b['max']}")
        print(f"time: {summary['time_range']['first']} .. {summary['time_range']['last']}")

    return 0


def cmd_correlate(args: argparse.Namespace) -> int:
    corpus = _resolve_corpus(args)

    lab = args.lab
    if lab is None:
        lab = find_latest_lab()
        if lab is None:
            print("no corpus-lab run found; pass --lab", file=sys.stderr)
            return 1
    lab = Path(lab)

    manifest = load_manifest(lab / "manifest.json")
    events = load_events(lab / "events.jsonl")
    result = scan_corpus(corpus, check_mode=not args.no_check_mode)

    contexts = correlate(result.captures, events)

    pane_to_agent = manifest.pane_key_to_agent()
    per_scenario: Counter[str] = Counter()
    per_expected: Counter[str] = Counter()
    boundary = 0
    disagreements = 0
    rows = []
    for cap in result.captures:
        ctx = contexts.get(cap.content_sha256)
        if ctx is None:
            continue
        per_scenario[ctx.scenario] += 1
        per_expected[ctx.expected_state] += 1
        if ctx.near_boundary:
            boundary += 1
        weak_model = weak_to_model(cap.weak_label)
        disagree = weak_model != ctx.expected_state
        if disagree:
            disagreements += 1
        rows.append(
            {
                "content_sha256": cap.content_sha256,
                "agent": pane_to_agent.get(cap.pane_key, "?"),
                "scenario": ctx.scenario,
                "expected_state": ctx.expected_state,
                "weak_label": cap.weak_label,
                "weak_model_state": weak_model,
                "weak_disagrees": disagree,
                "near_boundary": ctx.near_boundary,
                "seconds_after_event": round(ctx.seconds_after_event, 1),
            }
        )

    report = {
        "corpus": str(corpus),
        "lab": str(lab),
        "captures_correlated": len(rows),
        "captures_total": len(result.captures),
        "events": len(events),
        "scenarios": dict(per_scenario.most_common()),
        "expected_states": dict(per_expected.most_common()),
        "near_boundary": boundary,
        "weak_disagreements": disagreements,
    }

    if args.jsonl:
        for row in rows:
            print(json.dumps(row))
    elif args.json:
        print(json.dumps(report, indent=2))
    else:
        print(f"lab:        {lab}")
        print(f"correlated: {len(rows)}/{len(result.captures)} captures  ({len(events)} events)")
        print(f"near-boundary (need review): {boundary}")
        print(f"weak vs expected disagreements: {disagreements}")
        print("expected states:")
        for k, n in per_expected.most_common():
            print(f"  {k}: {n}")
        print("scenarios:")
        for k, n in per_scenario.most_common():
            print(f"  {k}: {n}")

    return 0


def _add_store_arg(p: argparse.ArgumentParser) -> None:
    p.add_argument(
        "--store",
        type=Path,
        default=None,
        help="annotation store JSONL (default: $XDG_DATA_HOME/hive/corpus-annotations/annotations.jsonl)",
    )


def _resolve_store(args: argparse.Namespace) -> AnnotationStore:
    path = args.store if args.store is not None else default_annotations_path()
    return AnnotationStore(path)


def cmd_annotate_add(args: argparse.Namespace) -> int:
    store = _resolve_store(args)
    ann = Annotation(
        content_sha256=args.sha,
        state=args.state,
        source="human",
        created_at=datetime.now(timezone.utc),
        confidence=args.confidence,
        evidence=args.evidence or "",
        alternatives=args.alternative or [],
        needs_human=False,
        note=args.note or "",
    )
    try:
        store.append(ann)
    except ValueError as exc:
        print(f"invalid annotation: {exc}", file=sys.stderr)
        return 2
    print(f"added human annotation: {args.sha[:12]}\u2026 -> {args.state}")
    return 0


def cmd_annotate_list(args: argparse.Namespace) -> int:
    store = _resolve_store(args)
    anns = store.all()
    if args.sha:
        anns = [a for a in anns if a.content_sha256 == args.sha]
    if args.source:
        anns = [a for a in anns if a.source == args.source]
    for ann in anns:
        print(json.dumps(ann.to_dict()))
    if not args.sha and not args.source:
        print(f"{len(anns)} annotations", file=sys.stderr)
    return 0


def cmd_annotate_resolve(args: argparse.Namespace) -> int:
    store = _resolve_store(args)
    resolved = store.resolve()

    by_source: Counter[str] = Counter(r.source for r in resolved.values())
    by_state: Counter[str] = Counter(r.state for r in resolved.values())
    superseded = sum(1 for r in resolved.values() if r.superseded)

    report: dict = {
        "store": str(store.path),
        "annotated_captures": len(resolved),
        "by_source": dict(by_source.most_common()),
        "by_state": dict(by_state.most_common()),
        "captures_with_superseded": superseded,
    }

    # Optionally join against the corpus to show effective-label coverage.
    if args.corpus is not None or args.with_corpus:
        corpus = _resolve_corpus(args)
        result = scan_corpus(corpus, check_mode=False)
        eff_source: Counter[str] = Counter()
        eff_state: Counter[str] = Counter()
        for cap in result.captures:
            state, source = effective_label(cap, resolved)
            eff_source[source] += 1
            eff_state[state] += 1
        report["corpus"] = str(corpus)
        report["corpus_captures"] = len(result.captures)
        report["effective_label_source"] = dict(eff_source.most_common())
        report["effective_state"] = dict(eff_state.most_common())

    if args.json:
        print(json.dumps(report, indent=2))
    else:
        print(f"store:      {store.path}")
        print(f"annotated:  {len(resolved)} captures")
        print("by source:")
        for k, n in by_source.most_common():
            print(f"  {k}: {n}")
        print("by state:")
        for k, n in by_state.most_common():
            print(f"  {k}: {n}")
        print(f"captures with superseded labels: {superseded}")
        if "effective_state" in report:
            print(f"corpus:     {report['corpus']} ({report['corpus_captures']} captures)")
            print("effective label source:")
            for k, n in report["effective_label_source"].items():
                print(f"  {k}: {n}")
            print("effective state:")
            for k, n in report["effective_state"].items():
                print(f"  {k}: {n}")
    return 0


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="hive-corpus", description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)

    p_validate = sub.add_parser("validate", help="scan corpus and report integrity issues")
    _add_corpus_arg(p_validate)
    p_validate.add_argument("--json", action="store_true", help="emit JSON report")
    p_validate.add_argument("--show-issues", type=int, default=0, metavar="N", help="print first N issues to stderr")
    p_validate.set_defaults(func=cmd_validate)

    p_summary = sub.add_parser("summary", help="print corpus distribution summary")
    _add_corpus_arg(p_summary)
    p_summary.add_argument("--json", action="store_true", help="emit JSON summary")
    p_summary.set_defaults(func=cmd_summary)

    p_corr = sub.add_parser("correlate", help="join captures with controlled-lab events")
    _add_corpus_arg(p_corr)
    p_corr.add_argument("--lab", type=Path, default=None, help="corpus-lab run dir (default: latest)")
    p_corr.add_argument("--json", action="store_true", help="emit JSON report")
    p_corr.add_argument("--jsonl", action="store_true", help="emit one JSON row per correlated capture")
    p_corr.set_defaults(func=cmd_correlate)

    p_ann = sub.add_parser("annotate", help="manage the append-only annotation store")
    ann_sub = p_ann.add_subparsers(dest="annotate_command", required=True)

    p_add = ann_sub.add_parser("add", help="append a human (gold) annotation")
    _add_store_arg(p_add)
    p_add.add_argument("sha", help="content_sha256 of the capture to label")
    p_add.add_argument("state", choices=MODEL_STATES, help="model state label")
    p_add.add_argument("--confidence", type=float, default=None, help="0..1")
    p_add.add_argument("--evidence", default=None, help="short quote justifying the label")
    p_add.add_argument("--alternative", action="append", choices=MODEL_STATES, help="other plausible state (repeatable)")
    p_add.add_argument("--note", default=None)
    p_add.set_defaults(func=cmd_annotate_add)

    p_list = ann_sub.add_parser("list", help="list annotations (JSONL)")
    _add_store_arg(p_list)
    p_list.add_argument("--sha", default=None, help="filter by content_sha256")
    p_list.add_argument("--source", choices=WRITABLE_SOURCES, default=None, help="filter by source")
    p_list.set_defaults(func=cmd_annotate_list)

    p_res = ann_sub.add_parser("resolve", help="show resolved labels (human > llm > weak)")
    _add_store_arg(p_res)
    _add_corpus_arg(p_res)
    p_res.add_argument("--with-corpus", action="store_true", help="join against corpus for effective-label coverage")
    p_res.add_argument("--json", action="store_true", help="emit JSON report")
    p_res.set_defaults(func=cmd_annotate_resolve)

    return parser


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    return args.func(args)


if __name__ == "__main__":
    raise SystemExit(main())
