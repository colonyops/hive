---
kind: changed
---

**Pane capture recording is removed.** `tmux.capture_recording` in the hive `config.yaml` no longer does anything, so drop the key. Recordings already under `recordings/tmux` in the hive data directory stay where they are; delete them when you no longer need them.
