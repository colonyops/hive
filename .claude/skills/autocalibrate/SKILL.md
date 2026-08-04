---
name: autocalibrate
description: Tune the terminal status assessment engine (Stage 1 rules) and status.Tracker (Stage 2 debounce) against the committed calibration corpus. Use when a status is misdetected or flapping, when adding/adjusting assess rules, or when asked to "calibrate", "tune the debounce", or "fix status flapping".
compatibility: claude
---

# Autocalibrate: Tuning the Two-Stage Status Assessment Engine

Hive detects agent status in two stages: `internal/core/terminal/assess` (Stage 1, stateless: pane content -> `assess.State`) feeds `internal/core/terminal/status` (Stage 2, stateful: debounces `assess.State` into a published `terminal.Status`). This skill is the closed loop for tuning both against a corpus of committed, deterministic recordings — never against live judgment calls that can't be replayed.

## Corpus Layout

- `internal/core/terminal/assess/testdata/<tool>/<scenario>.txt` — single-frame fixtures for Stage 1 rule classification. Table in `internal/core/terminal/assess/fixtures_test.go`.
- `test/calibration/sequences/<name>.jsonl` + `<name>.expected.json` — multi-frame recordings (the `{ts, content, title, inMode}` shape `hive x assess watch --record` produces) plus a sidecar of the published-status sequence `hive x assess replay` must reproduce exactly. Covers Stage 2 debounce timing (idle-confirmation delay, churn-as-working, approval immediacy, ...) that a single-frame fixture can't.
- `test/calibration/scenarios/<name>.yaml` — step/expectation scripts for live, in-container runs against a real pane (`hive x assess scenario`). Not wired into `mise run integration`; run by hand.

## The Tuning Loop

1. **Find mismatches.** Run the automated guardrail:

   ```bash
   go test ./internal/commands/... -run TestCalibrationCorpus_ReplayMatchesExpected -v
   ```

   This replays every `test/calibration/sequences/*.jsonl` through a fresh `assess.Engine` + `status.Tracker` on a virtual clock (never wall time, never an ambient config file — `status.DefaultOptions()` only) and diffs the published-status sequence against its `.expected.json` sidecar. A failing subtest names the sequence and the exact frame index where the sequences diverge.

   For a single sequence, `hive x assess replay <frames.jsonl> --jsonl` prints the full per-frame decision trail (state, rule ID, hold, published, candidate, candidatePolls, churned) — the same information the test compares, but human-readable.

2. **Diagnose with `file`.** Isolate the offending frame's raw content into its own file and run:

   ```bash
   hive x assess file <frame.txt> --tool claude
   ```

   This shows which rule fired (or didn't) and dumps the parsed regions (aboveBox, promptBoxBody, bottomLines, afterLastRule) so you can see exactly what the rule matcher saw. Compare "which rule fired" against "which rule should have fired."

3. **Edit the rule sets or debounce options.**
   - Stage 1 classification bugs (wrong state, wrong hold): `internal/core/terminal/assess/rules_claude.go`, `rules_codex.go`, `rules_generic.go`, `rules_common.go` (shared matchers).
   - Stage 2 timing bugs (flapping, premature/late confirmation): `internal/core/terminal/status/options.go`'s `DefaultOptions()` (`ConfirmIdle`, `ConfirmApproval`, `ChurnWindow`) or `debounce.go`'s transition table.

4. **Re-run everything — no regression trading.**

   ```bash
   mise run test    # the entire fixture + calibration corpus, not just the sequence you were fixing
   ```

   **HARD GUARDRAIL: a tuning change that fixes one scenario by breaking any other committed fixture or sequence is rejected.** If your fix regresses something else, the rule or debounce policy is under-specified — narrow the match condition or add a distinguishing signal instead of trading one failure for another. Do not edit or delete an existing fixture/sidecar to make it agree with new behavior unless you have independently verified the *old* expectation was wrong.

5. **Promote the fix to a permanent fixture.** Every mismatch you resolve must leave a new committed regression test behind, or the next tuning pass can silently re-break it:
   - Single-frame classification bug -> new file under `assess/testdata/<tool>/` plus a row in `fixtures_test.go`.
   - Multi-frame timing bug -> new `test/calibration/sequences/<name>.jsonl` + hand-verified `<name>.expected.json` (verify with `hive x assess replay`, exactly as described in step 1).

## Optional Live Tier (Container-Only)

Everything above is offline (fixture files, recorded frames) and safe anywhere, including this host. The live tier drives a real tmux pane and is **only safe inside `mise container`**:

```bash
mise container
# inside the container:
hive x assess drive test/calibration/sequences/claude-turn-lifecycle.jsonl --target <pane>
hive x assess scenario test/calibration/scenarios/claude-permission-flow.yaml --target <pane>
hive x assess watch <pane> --record new-sequence.jsonl
```

- `drive` replays a recorded sequence into a real pane (via `tmux respawn-pane`, not send-keys — see `cmd_x_assess_drive.go`'s doc comment for why) with its original relative timing, exercising the true `capture-pane -> list-panes -> assess` path with no agent credentials.
- `scenario` drives a pane through a scripted YAML of `send`/`key`/`expect` steps and scores the result: a JSON report with per-expectation pass/fail + detection latency in polls, and a scenario-level **flap count** — published transitions the scenario steps didn't imply. Flap count is the headline metric the entire debounce design exists to drive to zero; watch it even on scenarios whose expectations all pass.
- `watch --record` captures a fresh sequence from a real agent CLI (documented option only, no tooling provided) for promotion into the corpus per step 5.

## Hard Rules

- **Anything that sends pane input runs only inside `mise container`.** `drive` and `scenario` both send real keystrokes to a tmux pane; running that against the host's own tmux server has crashed dev environments before (see the repo's `CLAUDE.md` "Integration Tests" rule, which this inherits). Both commands refuse to run against the host's default tmux socket unless you pass `--allow-host`.
- **`--allow-host` is a deliberate, eyes-open exception only** — for a genuinely isolated non-default socket you've verified yourself. It is not a way to silence the check when you're in a hurry.
- **`hive x assess watch` (without `--record` writing to a live pane's input) is read-only and host-safe** — it only calls `capture-pane`/`display-message`, never `send-keys`. `hive x assess file` and `hive x assess replay` are pure offline file processing and always host-safe.
- **No regression trading** (repeated from step 4 because it's the rule most tempting to skip under time pressure): a green corpus after your change must be a strict superset of the green corpus before it, plus your fix.
