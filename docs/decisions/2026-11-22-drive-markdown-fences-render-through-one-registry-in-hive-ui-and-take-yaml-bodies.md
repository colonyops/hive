# Drive markdown fences render through one registry in hive-ui and take YAML bodies

- **Status:** accepted
- **Date:** 2026-11-22

## Context

Agents write Drive docs that describe file trees, call paths, PRs, and task lists. Plain markdown renders these as code blocks or ad-hoc lists. The markdown split in `@hive/ui` knew one special fence, mermaid, and hardcoded it. Users also need a way to add their own components without a second extension system next to applets.

## Decision

**Every fenced block that renders as a component goes through one registry in `@hive/ui`** (`components/fences/registry.ts`). The markdown split takes an `isFence(lang)` predicate and stays a pure string transform. `FenceHost` looks up the renderer and falls back to the normal code block when there is none or when the renderer fails, so a fence never breaks a doc. Mermaid is one entry in the registry. The built-ins are `hive-tree`, `hive-calls`, and `hive-forge`.

**A fence body is YAML.** `FenceHost` parses it with the YAML 1.2 core schema and hands the component plain JSON after the renderer's `validate`. Agents write a format they already know, the raw block stays readable on GitHub, in Obsidian, and in the TUI, and one parser serves every fence. Mermaid is the only built-in that takes the raw text.

**The `hive-` prefix is reserved for built-ins.** A user fence comes from an applet that declares `surfaces.markdown.fences` and registers through the same registry with the same props.

## Consequences

- The registry is module-level reactive state in `@hive/ui`, the one exception to the package's no-shared-state lint rule. A register or unregister must re-split every open document.
- `@hive/ui` depends on the `yaml` package.
- A rendered fence takes review comments as a whole block, the same as a mermaid diagram. Line-level comments inside a fence need a source map per node and are not supported.
- `hive-forge` is a static snapshot that the agent writes. Rendering never calls a forge, so share links work without credentials.
