// github-source is a reference node (0 in / 1 out): it names a github-*
// source defined in profiles/*.yml. The source itself runs on the backend
// (internal/desktop/pipeline.Source / githubSource) — the frontend never
// executes it, only consumes the msgs it already appended to the log, so
// there is no runtime.ts here (role: 'source' means "backend-run" per D2).
//
// The engine still needs to know which log topic feeds this node: sources
// append under topic "source:<source-id>" (see github_source.go), so an
// entry node (no inbound wire) whose config.source is set only accepts
// messages on that topic — see engine/runGraph.ts's `accepts`.

export const type = 'github-source'
export const role = 'source' as const

export interface Config {
  /** source id in profiles/*.yml (a github-search/github-notifications source). */
  source: string
}
