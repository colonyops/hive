# Hive Terminal-Status Corpus Pipeline

Offline, developer-facing tooling to build a training corpus and model for
Hive's zero-setup terminal-status detector. It reads the content-addressed
capture recordings Hive writes to disk, validates their integrity, correlates
controlled-lab scenario events, and (as the pipeline grows) drives LLM
silver-labeling, human review, dataset export, and baseline training.

Nothing here ships inside the Hive binary. The production detector is a small
separate Go runtime; this package produces the model and datasets it consumes.

See the handoff `.hive/handoffs/2026-07-21-terminal-status-model-corpus.md` and
research `.hive/research/2026-07-17-terminal-status-ml-classifier.md` for the
architecture, taxonomy, and evaluation plan.

## Label taxonomy

Model states (predicted from visible pane content):

- `working` — agent is actively producing output
- `permission` — agent is blocked on a permission/approval dialog
- `question` — agent is asking the user a question
- `idle` — agent finished; waiting for the next input
- `unknown` — indeterminate / out of distribution

`missing` is **not** a model state — it is a direct tmux/process fact.

Label provenance (lowest → highest authority): `weak` (Hive's StateTracker,
prioritization only) → `llm` (silver) → `human` (gold).

## Layout

```
ml/
├── hive_corpus/
│   ├── schema.py    # capture/annotation dataclasses, taxonomy, weak→model map
│   ├── reader.py    # content-addressed reader + integrity validator
│   ├── events.py    # controlled-lab manifest + scenario-event correlation
│   ├── annotate.py  # append-only annotation store (silver/gold labels)
│   └── cli.py       # `hive-corpus` CLI (validate / summary / correlate / annotate)
└── tests/           # stdlib-only unit tests with synthetic fixtures
```

## Usage

Base commands are stdlib-only (no install required):

```bash
cd ml

# Integrity scan of the default corpus (~/.local/share/hive/recordings/tmux)
python3 -m hive_corpus validate --show-issues 10

# Distribution summary (tools, weak labels, sessions, sizes, time range)
python3 -m hive_corpus summary

# Correlate captures with the latest controlled-lab run
python3 -m hive_corpus correlate

# Annotations: append a human (gold) label, list, and resolve
python3 -m hive_corpus annotate add <content_sha256> idle --confidence 0.95 --evidence "$ prompt, no spinner"
python3 -m hive_corpus annotate list --source human
python3 -m hive_corpus annotate resolve --with-corpus

# Point at a specific corpus or lab run
python3 -m hive_corpus summary --corpus /path/to/recordings/tmux
python3 -m hive_corpus correlate --lab ~/.local/share/hive/corpus-lab/<run>
```

`validate` exits non-zero when integrity issues are found.

## Development

```bash
cd ml
uv run --with pytest python -m pytest -q
```

Optional extras (added as later stages land): `train` (numpy + scikit-learn),
`llm` (httpx).

## Status

Implemented: corpus reader/validator, distribution summary, controlled-lab
event correlation, append-only annotation store with provenance resolution
(`human` > `llm` > `weak`).

Planned (see handoff "Immediate Next Work"): LLM silver-labeling, human review
queue/TUI, dataset export with session/pane/run splits + near-duplicate
downsampling, baseline trainer, and Go inference parity.
