package mcpcatalog

import (
	"maps"
	"sort"
)

// registry is the shipped set, keyed by Descriptor.Type. Never init()
// self-registration — gochecknoinits is enabled, and an explicit map is the
// only form where the shipped set can be read off one file.
//
// A shipped entry is an endorsement (spec §7.3) and a release to revise, so
// the set grows reluctantly — first-party servers with a keyless, pinnable
// invocation only; mcps.yaml is the escape hatch for everything else.
var registry = map[string]Descriptor{
	// The desktop's own MCP server (ADR mcp-replaces-the-agent-facing-http-api) — the surface an agent drives
	// the app through, and the reason a workspace's agent can read this
	// install's inbox at all. Its URL is this run's loopback endpoint, so the
	// declaration carries none; see Descriptor.RuntimeURL.
	"hive-desktop": {
		Type:        "hive-desktop",
		Title:       "Hive Desktop",
		Description: "This Hive Desktop install: read its inbox, feeds, profiles and action catalog, force a source refresh, and dry-run a flow against input you supply.",
		Stability:   StabilityStable,
		Server:      Server{Transport: TransportHttp},
		RuntimeURL:  true,
		RuntimePath: "/mcp",
	},
	// The desktop's canvas server (ADR canvases-are-named-files-in-the-workspace-folder-served-over-their-own-mcp-entry): the surface an agent
	// puts content in front of the user through. Its own entry rather than
	// more hive-desktop tools, so a workspace can have a canvas without
	// granting the app-control surface — and vice versa.
	"hive-canvas": {
		Type:        "hive-canvas",
		Title:       "Hive Canvas",
		Description: "Canvases beside the chat: named surfaces of markdown and link blocks, saved as files in the workspace folder and shown to the user in the Agents area while the conversation keeps running.",
		Stability:   StabilityStable,
		Server:      Server{Transport: TransportHttp},
		RuntimeURL:  true,
		RuntimePath: "/mcp/canvas",
	},
	"chrome-devtools": {
		Type:        "chrome-devtools",
		Title:       "Chrome DevTools",
		Description: "Browser debugging: read network requests, console output, and performance traces from a Chrome the agent controls.",
		Stability:   StabilityStable,
		Server: Server{
			Transport: TransportStdio,
			Command:   "npx",
			// -y for the same reason as playwright's below. @latest is the
			// documented invocation (ChromeDevTools/chrome-devtools-mcp); Hive
			// does not vendor a pinned version.
			Args: []string{"-y", "chrome-devtools-mcp@latest"},
		},
	},
	"playwright": {
		Type:        "playwright",
		Title:       "Playwright",
		Description: "Browser automation: navigate, click, fill forms, and read page content through a real, controllable browser.",
		Stability:   StabilityStable,
		Server: Server{
			Transport: TransportStdio,
			Command:   "npx",
			// -y suppresses npx's interactive install confirmation, which
			// would otherwise stall a non-interactive agent launch the first
			// time this package version is fetched. @latest is the documented
			// invocation (microsoft/playwright-mcp); Hive does not vendor a
			// pinned version.
			Args: []string{"-y", "@playwright/mcp@latest"},
		},
	},
}

// Types returns every registered type in sorted order. Sorting keeps
// generated output byte-stable across runs — Go map iteration is randomized.
func Types() []string {
	types := make([]string, 0, len(registry))
	for mcpType := range registry {
		types = append(types, mcpType)
	}
	sort.Strings(types)
	return types
}

// Lookup returns the descriptor registered for an MCP type.
func Lookup(mcpType string) (Descriptor, bool) {
	d, ok := registry[mcpType]
	return d, ok
}

// All returns every descriptor, keyed by type. The returned map is a copy, so
// a caller ranging over it cannot mutate the registry.
func All() map[string]Descriptor {
	out := make(map[string]Descriptor, len(registry))
	maps.Copy(out, registry)
	return out
}
