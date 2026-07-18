// Splits the backend's config error text into displayable entries for the
// config-error overlay's "VALIDATION DETAIL" list.
//
// The backend (internal/desktop/feed/config.go ConfigInfo.Error) currently
// reports a single string — either a hand-written fmt.Errorf validation
// message, or a yaml.v3 decode failure. Multi-problem yaml.v3 failures are
// formatted as one "line N: message" per line under a "yaml: unmarshal
// errors:" header; hand-written validation errors are a single line with no
// line number. This parses the former into multiple line-numbered entries
// and falls back to treating the whole string as one entry otherwise, so the
// overlay already renders a real per-problem list where the data supports
// it, without the Go side needing to change. If the backend later returns
// structured errors, this function (and its ConfigValidationError return
// type) is the only thing that needs to change — callers just consume the
// array.
export interface ConfigValidationError {
  line: number | null
  message: string
}

// Matches "line N:" at the start of a line (the multi-error yaml.v3 format,
// each already split onto its own line) or after a "yaml: " prefix (the
// single-error format, e.g. "yaml: line 3: mapping values are not allowed
// in this context") — anything preceded by whitespace or the string start.
const LINE_PREFIX = /(?:^|\s)line (\d+):\s*(.*)$/i

export function parseConfigErrors(raw: string): ConfigValidationError[] {
  const trimmed = raw.trim()
  if (!trimmed) return []

  const entries: ConfigValidationError[] = []
  for (const rawLine of trimmed.split('\n')) {
    const line = rawLine.trim()
    if (!line) continue
    const match = LINE_PREFIX.exec(line)
    if (match) entries.push({ line: Number(match[1]), message: match[2] })
  }
  if (entries.length > 0) return entries

  return [{ line: null, message: trimmed }]
}
