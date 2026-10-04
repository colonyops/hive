import { describe, expect, it } from 'vitest'
import { mcpSetup, mcpSetupAgents } from '../mcpSetup'

const url = 'http://127.0.0.1:4821/mcp/canvas'

describe('mcpSetup', () => {
  it('writes the Claude Code command that adds the server for every project', () => {
    expect(mcpSetup('claude', 'hive-canvas', url).text).toBe(
      'claude mcp add --scope user --transport http hive-canvas http://127.0.0.1:4821/mcp/canvas',
    )
  })

  // The same table Hive writes into a workspace's .codex/config.toml.
  it('writes the Codex config table', () => {
    expect(mcpSetup('codex', 'hive-canvas', url).text).toBe(
      '[mcp_servers.hive-canvas]\nurl = "http://127.0.0.1:4821/mcp/canvas"',
    )
  })

  // The same entry Hive writes into a workspace's .mcp.json.
  it('writes the mcpServers entry other agents read', () => {
    expect(JSON.parse(mcpSetup('json', 'hive-canvas', url).text)).toEqual({
      mcpServers: { 'hive-canvas': { type: 'http', url } },
    })
  })

  it('says where each form goes', () => {
    for (const { value } of mcpSetupAgents) expect(mcpSetup(value, 'hive-canvas', url).hint).not.toBe('')
  })
})
