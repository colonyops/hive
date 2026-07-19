// Builds a schema-complete, paste-ready prompt for a coding agent to author
// or edit a flows/*.yaml document — the flows equivalent of
// internal/desktop/feed/prompt.go's BuildConfigPrompt (see
// useFeedState.ts's copyConfigPrompt for the feed-side "Copy prompt" flow
// this mirrors in spirit). Unlike the feed prompt, this one is built
// entirely client-side out of the registry.ts app registry — each node
// type's help.md and defaults are already that type's own single source of
// truth (nodeType.ts), so this never duplicates a per-type schema
// description by hand; adding a node type directory automatically adds it
// to the prompt.

import { palette } from '../registry'
import type { NodeCategory } from '../nodeType'

const CATEGORY_ORDER: NodeCategory[] = ['Sources', 'Process', 'Destinations']

// A condensed transcription of internal/desktop/pipeline/flow/loader_test.go's
// workedExampleYAML (the design doc's canonical sample flow: github-source ->
// github-filter -> function(2 outputs) -> {feed, action}) — concrete enough
// that an agent can see a real, valid graph instead of inventing field names
// from the schema description alone.
const WORKED_EXAMPLE = `version: 1
name: Frontend Triage
nodes:
  - { id: in-prs, type: github-source, source: team-prs }
  - { id: drop-bots, type: github-filter, exclude_authors: ["*[bot]"], repos: ["colonyops/*"] }
  - id: tag
    type: function
    outputs: 2
    on_message: |
      if (msg.Payload.state === "closed") return null;
      msg.Payload.tag = "review"; return [msg, null];
  - { id: team-feed, type: feed, feed: team-review }
  - { id: spawn-review, type: action, action: review-pr }
wires:
  - { from: in-prs, to: drop-bots }
  - { from: drop-bots, to: tag }
  - { from: tag, out: 0, to: team-feed }
  - { from: tag, out: 0, to: spawn-review }`

/**
 * Renders every registered node type's help.md plus its default config into
 * one prompt alongside the flows/*.yaml top-level schema and a worked
 * example, so a coding agent has everything needed to author or edit a flow
 * with no other context — the user only appends what they actually want.
 */
export function buildFlowPrompt(): string {
  const lines: string[] = []

  lines.push('Author or edit a Hive Desktop pipeline flow (a flows/*.yaml file).')
  lines.push('')
  lines.push('File: flows/<id>.yaml — the id is the filename stem (e.g. flows/triage.yaml -> id "triage"), never a value written inside the file.')
  lines.push('The app watches flows/ and hot-reloads on save; no restart needed.')
  lines.push('')
  lines.push('Top-level schema (strict YAML — unknown keys are rejected):')
  lines.push('- version: must be 1.')
  lines.push('- name: display name.')
  lines.push('- enabled: optional bool, defaults to true.')
  lines.push('- nodes: list of nodes. Every node has:')
  lines.push('  - id: unique within the flow.')
  lines.push('  - type: one of the node types documented below.')
  lines.push("  - name: optional display name (falls back to the type's label).")
  lines.push('  - disabled: optional bool — a disabled node drops every message it receives instead of running.')
  lines.push("  - plus that type's own fields, flattened at the same level (NOT nested under a `config:` key).")
  lines.push('- wires: list of { from, out, to } — from/to are node ids; out defaults to 0 (omit it for single-output nodes).')
  lines.push('')
  lines.push('Node types:')

  for (const category of CATEGORY_ORDER) {
    const defs = palette[category]
    if (defs.length === 0) continue
    lines.push('')
    lines.push(`## ${category}`)
    for (const def of defs) {
      lines.push('')
      lines.push(`### ${def.label} (type: "${def.type}")`)
      lines.push('')
      lines.push(def.help.trim())
      lines.push('')
      lines.push('Default config:')
      lines.push('```json')
      lines.push(JSON.stringify(def.defaults, null, 2))
      lines.push('```')
    }
  }

  lines.push('')
  lines.push('Worked example:')
  lines.push('```yaml')
  lines.push(WORKED_EXAMPLE)
  lines.push('```')
  lines.push('')
  lines.push('What I want: <describe the flow you want here>')

  return lines.join('\n')
}
