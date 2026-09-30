# Playwright

The `playwright` MCP server gives an agent a real, controllable browser:
navigate, click, fill forms, take screenshots, and read page content and the
accessibility tree. It is Microsoft's own MCP server for Playwright
(`@playwright/mcp`), not a Hive-authored wrapper.

## Launch

Hive launches it over stdio:

```
npx -y @playwright/mcp@latest
```

`-y` skips npx's interactive install confirmation, which would otherwise
stall a non-interactive agent launch the first time this package version is
fetched on a machine. `@latest` is the documented invocation; Hive does not
vendor or pin a specific `@playwright/mcp` version, so a workspace enabling
this entry gets whatever the registry resolves at launch time.

## Stability

`stable`. The server and its launch command are settled; this entry's shape
in the catalogue may still change if a configurable variant ships later
(see the catalogue's Migration Notes).

## What it needs

Node.js and a Playwright browser install reachable from the workspace's
environment — the same prerequisites `npx @playwright/mcp` has anywhere else.
Nothing workspace-specific: this entry ships with no configuration in M1.
