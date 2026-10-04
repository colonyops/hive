/** The agents the MCP settings page writes setup for. `json` is the entry most other agents read. */
export type McpSetupAgent = 'claude' | 'codex' | 'json'

export const mcpSetupAgents: Array<{ value: McpSetupAgent; label: string }> = [
  { value: 'claude', label: 'Claude Code' },
  { value: 'codex', label: 'Codex' },
  { value: 'json', label: 'Other' },
]

export interface McpSetup {
  /** Where the text goes. */
  hint: string
  /** What to run or paste. */
  text: string
}

/**
 * What to run or paste so one agent reaches the HTTP MCP server at `url`. The
 * Codex and JSON forms are the entries Hive writes into a workspace
 * (agentws/wiring.go), so an agent set up by hand reads what a wired one does.
 */
export function mcpSetup(agent: McpSetupAgent, id: string, url: string): McpSetup {
  switch (agent) {
    case 'claude':
      return {
        hint: 'Run this once. It adds the server for every project.',
        text: `claude mcp add --scope user --transport http ${id} ${url}`,
      }
    case 'codex':
      return {
        hint: 'Add this to ~/.codex/config.toml.',
        text: `[mcp_servers.${id}]\nurl = "${url}"`,
      }
    case 'json':
      return {
        hint: "Add this entry to the agent's MCP configuration file.",
        text: JSON.stringify({ mcpServers: { [id]: { type: 'http', url } } }, null, 2),
      }
  }
}
