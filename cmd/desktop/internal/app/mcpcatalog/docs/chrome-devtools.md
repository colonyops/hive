# Chrome DevTools

The `chrome-devtools` MCP server gives an agent DevTools' view of a real
Chrome: read network requests, console messages, and performance traces, run
Lighthouse-style audits, and inspect the live page. It is Google's own MCP
server (`chrome-devtools-mcp`), not a Hive-authored wrapper.

It is the inspection counterpart to `playwright`, not a replacement: enable
`playwright` to drive UI flows, `chrome-devtools` to debug what a page is
doing — network, console, and performance — while it runs. A web workspace
often wants both.

## Launch

Hive launches it over stdio:

```
npx -y chrome-devtools-mcp@latest
```

`-y` skips npx's interactive install confirmation, which would otherwise
stall a non-interactive agent launch the first time this package version is
fetched on a machine. `@latest` is the documented invocation; Hive does not
vendor or pin a specific `chrome-devtools-mcp` version, so a workspace
enabling this entry gets whatever the registry resolves at launch time.

## Stability

`stable`. The server is Google-maintained, past 1.0, and its launch command
is settled.

## What it needs

Node.js 20.19 or newer and a current stable Chrome installed and reachable
from the workspace's environment. No credentials, and nothing
workspace-specific: this entry ships with no configuration in M1.
