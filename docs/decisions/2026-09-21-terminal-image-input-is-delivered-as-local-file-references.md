# Terminal image input is delivered as local file references

- **Status:** accepted
- **Date:** 2026-09-21

## Context

CLI agents recognize local image paths in pasted input. Hive's terminal streams
carry text, and tmux owns the pane's bracketed-paste mode. Clipboard screenshots
have no path, while agent conversations can outlive the app and resume later.

## Decision

Capture native file drops in Wails and image paste at each terminal host. Prepare
paths in the core and paste each image separately into the captured target.
Keep image bytes off the terminal stream. The tmux control plane acknowledges
paste delivery; popup terminals retain xterm's paste handling over their PTY.

Store clipboard images under StateDir with private permissions, preserving their
bytes. Retain completed files across restarts and session deletion. Bound uploads
and total storage; refuse new files at quota rather than evicting references that
an external agent may still use. Native disk images remain at their original path.

## Consequences

Images work for agents sharing the host filesystem. Remote filesystem transfer is
outside this feature. Users remove stored clipboard images they no longer need.
Delivery acknowledgment does not prove an agent accepted an image. Preparation
is cancellable; a dispatched paste can finish only in its original pane.
