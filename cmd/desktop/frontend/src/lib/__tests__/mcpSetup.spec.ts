import { describe, expect, it } from 'vitest'
import { mcpSetup, mcpSetupAgents } from '../mcpSetup'

const url = 'http://127.0.0.1:4821/mcp/drive'

describe('mcpSetup', () => {
  it('writes the Claude Code command that adds the server for every project', () => {
    expect(mcpSetup('claude', 'hive-drive', url).text).toBe(
      'claude mcp add --scope user --transport http hive-drive http://127.0.0.1:4821/mcp/drive',
    )
  })

  // The same table Hive writes into a workspace's .codex/config.toml.
  it('writes the Codex config table', () => {
    expect(mcpSetup('codex', 'hive-drive', url).text).toBe(
      '[mcp_servers.hive-drive]\nurl = "http://127.0.0.1:4821/mcp/drive"',
    )
  })

  // The same entry Hive writes into a workspace's .mcp.json.
  it('writes the mcpServers entry other agents read', () => {
    expect(JSON.parse(mcpSetup('json', 'hive-drive', url).text)).toEqual({
      mcpServers: { 'hive-drive': { type: 'http', url } },
    })
  })

  it('says where each form goes', () => {
    for (const { value } of mcpSetupAgents) expect(mcpSetup(value, 'hive-drive', url).hint).not.toBe('')
  })
})
