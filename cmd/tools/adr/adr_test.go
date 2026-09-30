package main

import (
	"strings"
	"testing"
)

const good = `# Terminal transport

- **Status:** accepted
- **Date:** 2026-07-28

## Context
`

func TestParseADR(t *testing.T) {
	t.Parallel()

	adr, err := parseADR("2026-07-28-terminal-transport.md", good)
	if err != nil {
		t.Fatalf("parseADR: %v", err)
	}
	if adr.Slug != "terminal-transport" || adr.Date != "2026-07-28" {
		t.Fatalf("got %+v", adr)
	}
	if adr.Title != "Terminal transport" || adr.Status != "accepted" {
		t.Fatalf("got %+v", adr)
	}
}

func TestParseADRRejects(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		file    string
		content string
		want    string
	}{
		{"legacy number", "0036-terminal-transport.md", good, "YYYY-MM-DD-slug.md"},
		{"single word slug", "2026-07-28-transport.md", good, "YYYY-MM-DD-slug.md"},
		{"impossible date", "2026-13-40-terminal-transport.md", good, "not a real date"},
		{
			"date disagrees with filename",
			"2026-07-29-terminal-transport.md", good,
			"disagrees with filename date",
		},
		{
			"numbered heading",
			"2026-07-28-terminal-transport.md",
			strings.Replace(good, "# Terminal", "# 0036 — Terminal", 1),
			"legacy number",
		},
		{
			"no status",
			"2026-07-28-terminal-transport.md",
			strings.Replace(good, "- **Status:** accepted\n", "", 1),
			"no `- **Status:**` line",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseADR(tc.file, tc.content)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestCheckRefs(t *testing.T) {
	t.Parallel()

	adrs := []ADR{
		{Name: "2026-07-28-terminal-transport.md", Date: "2026-07-28", Slug: "terminal-transport"},
		{Name: "2026-07-25-goja-script-runtime.md", Date: "2026-07-25", Slug: "goja-script-runtime"},
	}

	cases := []struct {
		name    string
		content string
		want    string // "" means the line must pass
	}{
		{"valid citation", "the socket is framed here (ADR terminal-transport).", ""},
		{
			"valid multi citation",
			"the engine runs in Go (ADRs goja-script-runtime and terminal-transport).", "",
		},
		{"prose that is not a citation", "decisions are recorded as ADRs in `docs/decisions/`.", ""},
		{"valid link", "see [transport](2026-07-28-terminal-transport.md) for the framing", ""},
		{"valid nested link", "see [t](../docs/decisions/2026-07-28-terminal-transport.md)", ""},
		{"unknown slug", "as decided (ADR terminal-transportt).", `cites unknown ADR "terminal-transportt"`},
		{
			"unknown slug in a tail",
			"both (ADRs goja-script-runtime, flow-engine-in-go).",
			`cites unknown ADR "flow-engine-in-go"`,
		},
		{"dangling link", "see [x](2026-07-28-terminal-transports.md)", "links to missing decision file"},
		{"quoted legacy id is not a citation", "old threads still say `ADR 0036`.", ""},
		{"quoted unknown slug is not a citation", "cite it as `ADR some-future-thing`.", ""},
		{"legacy citation", "the socket is framed here (ADR 0036).", "legacy numbered ADR reference"},
		{"legacy link path", "see [0036](decisions/0036-terminal-transport.md)", "legacy numbered ADR reference"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := checkRefs(adrs, []sourceFile{{Path: "docs/architecture.md", Content: tc.content}})
			switch {
			case tc.want == "" && len(got) > 0:
				t.Fatalf("want clean, got %v", got)
			case tc.want != "":
				if len(got) == 0 || !strings.Contains(strings.Join(got, "\n"), tc.want) {
					t.Fatalf("want a problem containing %q, got %v", tc.want, got)
				}
			}
		})
	}
}

func TestCheckRefsSkipsReleasedMigrations(t *testing.T) {
	t.Parallel()

	adrs := []ADR{{Name: "2026-07-28-terminal-transport.md", Slug: "terminal-transport"}}
	files := []sourceFile{{
		Path:    "cmd/desktop/internal/app/data/queries/migrations/0007_item_session.up.sql",
		Content: "-- the item an action ran against (ADR 0060).",
	}}
	if got := checkRefs(adrs, files); len(got) > 0 {
		t.Fatalf("released migrations are byte-pinned and must be exempt, got %v", got)
	}
}

func TestSlugify(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"CI runs on main to seed the cache PRs read": "ci-runs-on-main-to-seed-the-cache-prs-read",
		"`ptyterm` terminals are caller-addressed":   "ptyterm-terminals-are-caller-addressed",
		"An item↔session link is desktop state":      "an-item-session-link-is-desktop-state",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}
