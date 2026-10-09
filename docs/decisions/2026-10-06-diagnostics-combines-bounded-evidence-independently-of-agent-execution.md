# Diagnostics combines bounded evidence independently of agent execution

- **Status:** accepted
- **Date:** 2026-10-06

## Context

Session failures can be recorded in jobs without reaching the process log.
An agent may fail to launch for the same reason the user needs diagnostics.

## Decision

A core Diagnostics service reads bounded Desktop and CLI log tails and job
outcomes without copying them into another store. Wails and read-only MCP
adapters share this service. CLI and Desktop share hive.log with service_name identifying the program.
The reader accepts text and JSON lines, preserving raw records and source identity.

Diagnostics opens in its own window without an agent. Investigation is an
explicit action: save a bounded incident snapshot, render a prompt in Go, and
launch the selected configured profile through the existing authenticated PTY
transport. Copy and export work independently of that transport.

## Consequences

Unavailable sources and truncated history are visible. An agent receives the
same evidence and can request surrounding entries through MCP. CLI flags used
by a separate invocation cannot be discovered from Desktop's environment.
