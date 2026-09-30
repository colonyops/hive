package agentws

import (
	"context"
	"fmt"
	"os/exec"
	"sort"

	"github.com/colonyops/hive/cmd/desktop/internal/app/mcpcatalog"
)

// CatalogueEntry is one row of the merged MCP catalogue: a shipped registry
// entry, a user library entry, or a user entry that replaces a shipped one.
type CatalogueEntry struct {
	ID          string
	Title       string
	Description string
	Shipped     bool
	Stability   mcpcatalog.Stability
	// Shadows is the shipped id this user entry replaces, empty otherwise.
	Shadows string
	// Server is the resolved launch declaration. Nothing is enabled whose
	// command line the user cannot read first (spec §7.3).
	Server mcpcatalog.Server
	// Problem reports why this entry will not work — today, a stdio Command
	// that does not resolve on the PATH a session is launched with. Showing a
	// resolved command that does not resolve is worse than showing nothing:
	// the agent starts, the MCP silently fails to connect, and the failure
	// surfaces only inside the agent's own /mcp output.
	Problem string
}

// Catalogue merges the shipped registry with the user library into one list,
// sorted by id. A user entry shadowing a shipped id wins and says so — the
// same rule the generator applies to .shared/ (D-D), stated once for the
// whole feature. It reads no store state beyond lib: a free function tests
// without standing up a store over a temp root.
//
// The LookPath check runs here, at catalogue-render time, never inside
// Generate — PATH is machine state and the generator must stay pure (spec
// §4.4). lookPath resolves a stdio command against the PATH a session
// launches with (execenv.Resolver.LookPath, ADR subprocess-environment); nil
// falls back to this process's own, which a desktop launch inherits from
// launchd and which nothing a package manager installed is on.
func Catalogue(ctx context.Context, lib Library, lookPath func(context.Context, string) (string, error)) []CatalogueEntry {
	if lookPath == nil {
		lookPath = func(_ context.Context, name string) (string, error) { return exec.LookPath(name) }
	}

	shipped := mcpcatalog.All()

	entries := make(map[string]CatalogueEntry, len(shipped)+len(lib.Servers))
	for id, d := range shipped {
		entries[id] = CatalogueEntry{
			ID:          id,
			Title:       d.Title,
			Description: d.Description,
			Shipped:     true,
			Stability:   d.Stability,
			Server:      d.Server,
		}
	}

	for id, srv := range lib.Servers {
		entry := CatalogueEntry{
			ID:     id,
			Title:  srv.Title,
			Server: serverFromMCPServer(srv),
		}
		if d, ok := shipped[id]; ok {
			entry.Shadows = id
			entry.Stability = d.Stability
			entry.Description = d.Description
			if entry.Title == "" {
				entry.Title = d.Title
			}
		}
		entries[id] = entry
	}

	ids := make([]string, 0, len(entries))
	for id := range entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := make([]CatalogueEntry, 0, len(ids))
	for _, id := range ids {
		entry := entries[id]
		entry.Problem = problemFor(ctx, entry.Server, lookPath)
		out = append(out, entry)
	}
	return out
}

func serverFromMCPServer(s MCPServer) mcpcatalog.Server {
	return mcpcatalog.Server{
		Transport: s.effectiveType(),
		Command:   s.Command,
		Args:      s.Args,
		Env:       s.Env,
		URL:       s.URL,
		Headers:   s.Headers,
	}
}

// problemFor reports why a resolved server will not work, or "" when it is
// fine. Only a stdio Command is checked: http/sse servers have nothing local
// to resolve.
func problemFor(ctx context.Context, s mcpcatalog.Server, lookPath func(context.Context, string) (string, error)) string {
	if s.Transport != mcpcatalog.TransportStdio || s.Command == "" {
		return ""
	}
	if _, err := lookPath(ctx, s.Command); err != nil {
		return fmt.Sprintf("command %q not found on PATH", s.Command)
	}
	return ""
}
