# Canvases carry front matter and users can delete them

- **Status:** accepted
- **Date:** 2026-10-09

## Context

Canvas files already record creation and update times, but readers cannot see
them and Markdown exports discard them. Agents also need a small, structured
place for tags, status, ownership, and similar document metadata. Canvas
content remains agent-authored, but making deletion MCP-only prevents the user
who owns the artifact from removing it in the reader.

## Decision

A canvas carries `frontmatter`, a flat mapping of keys to scalar values or
lists of scalars. `created_at` and `updated_at` are reserved: Hive derives them
from the canvas timestamps, displays them with editable metadata, and writes
all fields as YAML front matter when copying or saving Markdown.

The `set_frontmatter` MCP tool replaces the complete editable mapping on an
existing canvas. Replacement makes removing a key explicit and keeps metadata
changes atomic with the canvas file. Existing canvases read with an empty
mapping and need no migration.

The token-guarded frontend canvas surface may delete a canvas after a
confirmation dialog. Block and front matter edits remain MCP-only. Both delete
paths call `CanvasService`, remove the same file, and publish the same canvas
update event.

## Consequences

- Front matter is intentionally shallow so the reader can present it as a
  compact property list and exports stay easy to edit.
- Tags use the ordinary list form rather than a dedicated stored field; the
  reader gives that conventional key a tag treatment.
- Copy and save now add a YAML header even when an agent set no editable
  fields, because the automatic timestamps are always present.
- The frontend is no longer wholly read-only, but it still cannot race an
  agent with content edits; its only canvas mutation deletes the artifact.
