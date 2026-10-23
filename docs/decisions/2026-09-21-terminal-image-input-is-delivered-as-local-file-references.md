# Terminal image input is delivered as local file references

- **Status:** accepted
- **Date:** 2026-09-21

## Context

CLI agents recognize local image paths in pasted input. Hive's terminal
streams carry text, tmux owns the pane's bracketed-paste mode, clipboard
screenshots have no path, and agent conversations can outlive the app and
resume later.

## Decision

Capture native file drops in Wails and image paste at each terminal host.
The core prepares paths and pastes each image separately into the captured
target, so image bytes never travel on the terminal stream. The tmux control
plane acknowledges paste delivery; popup terminals keep xterm's paste
handling over their PTY.

Clipboard images are stored with their original bytes under
`<StateDir>/assets/terminal-images` with private permissions, and kept
across restarts and session deletion. Uploads and total storage (1 GiB) are
bounded; at quota new files are refused rather than evicting references an
external agent may still use. A dropped file is referenced at its original
path.

## Consequences

Images work only for agents that share the host filesystem. Users remove
stored clipboard images they no longer need. A delivery acknowledgment does
not prove the agent accepted the image. Preparation is cancellable; a
dispatched paste can finish only in its original pane.
