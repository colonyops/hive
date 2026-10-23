# Canvases carry front matter and users can delete them

- **Status:** accepted
- **Date:** 2026-10-09

## Context

Readers need to see a canvas's creation and update times, and Markdown exports
need to keep them. Agents need a small, structured place for tags, status,
ownership, and similar metadata. Canvas content is agent-authored, but the user
who owns a canvas must be able to remove it.

## Decision

A canvas carries `frontmatter`, a flat mapping of keys to scalar values or
lists of scalars (`drive.ValidateFrontmatter`). `created_at` and `updated_at`
are reserved: Hive derives them from the canvas timestamps, displays them with
the editable fields, and writes all fields as YAML front matter when copying or
saving Markdown.

The `set_frontmatter` MCP tool replaces the complete editable mapping on an
existing canvas. Replacement makes removing a key explicit and keeps metadata
changes atomic with the canvas file. A canvas with no front matter reads with
an empty mapping.

The user deletes a canvas from the desktop after a confirmation dialog. Block
and front matter edits stay MCP-only. The frontend and MCP delete paths both go
through the engine's drive service, remove the same file, and publish the same
change event (ADR
hive-drive-is-one-document-tree-under-the-context-root-and-a-canvas-is-one-document-kind-in-it).

## Consequences

- Front matter is shallow so the reader can show it as a compact property list
  and exports stay easy to edit.
- Tags use the ordinary list form, not a dedicated stored field; the reader
  gives that conventional key a tag treatment.
- Copy and save always add a YAML header, because the automatic timestamps are
  always present.
- The frontend cannot race an agent on canvas content: it can delete, rename,
  or move a canvas, but not edit it.
