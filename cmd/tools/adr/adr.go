package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const decisionsDir = "docs/decisions"

// ADR is one decision record. Its identity is the filename: the date orders it,
// the slug names it, and the slug alone is how prose cites it.
type ADR struct {
	Name   string
	Date   string
	Slug   string
	Title  string
	Status string
}

var (
	nameRe   = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})-([a-z0-9]+(?:-[a-z0-9]+)+)\.md$`)
	titleRe  = regexp.MustCompile(`(?m)^# (.+)$`)
	statusRe = regexp.MustCompile(`(?m)^- \*\*Status:\*\* (.+)$`)
	dateRe   = regexp.MustCompile(`(?m)^- \*\*Date:\*\* (\d{4}-\d{2}-\d{2})\s*$`)

	// A citation is "ADR <slug>", optionally continuing ", <slug>" or
	// " and <slug>". Requiring a hyphen keeps prose like "ADRs in docs/" from
	// parsing as a citation; every slug has at least one.
	citeRe     = regexp.MustCompile(`\bADRs? ([a-z0-9]+(?:-[a-z0-9]+)+)`)
	citeMore   = regexp.MustCompile(`^(?:, | and | & )([a-z0-9]+(?:-[a-z0-9]+)+)`)
	mdLinkRe   = regexp.MustCompile(`\]\(((?:\.\./)*(?:docs/)?(?:decisions/)?(\d{4}-\d{2}-\d{2}-[a-z0-9-]+\.md))\)`)
	legacyRe   = regexp.MustCompile(`\bADRs? (00\d{2})\b|decisions/(00\d{2})-|\[(00\d{2})\]`)
	numberedH  = regexp.MustCompile(`(?m)^# \d{4} — `)
	codeSpanRe = regexp.MustCompile("`[^`]*`")
)

func parseADR(name, content string) (ADR, error) {
	m := nameRe.FindStringSubmatch(name)
	if m == nil {
		return ADR{}, fmt.Errorf("filename must be YYYY-MM-DD-slug.md")
	}
	adr := ADR{Name: name, Date: m[1], Slug: m[2]}

	if _, err := time.Parse("2006-01-02", adr.Date); err != nil {
		return ADR{}, fmt.Errorf("filename date %q is not a real date", adr.Date)
	}
	if loc := numberedH.FindString(content); loc != "" {
		return ADR{}, fmt.Errorf("heading still carries a legacy number: %q", strings.TrimSpace(loc))
	}

	t := titleRe.FindStringSubmatch(content)
	if t == nil {
		return ADR{}, fmt.Errorf("no `# Title` heading")
	}
	adr.Title = strings.TrimSpace(t[1])

	s := statusRe.FindStringSubmatch(content)
	if s == nil {
		return ADR{}, fmt.Errorf("no `- **Status:**` line")
	}
	adr.Status = strings.TrimSpace(s[1])

	d := dateRe.FindStringSubmatch(content)
	if d == nil {
		return ADR{}, fmt.Errorf("no `- **Date:**` line")
	}
	if d[1] != adr.Date {
		return ADR{}, fmt.Errorf("date field %s disagrees with filename date %s", d[1], adr.Date)
	}
	return adr, nil
}

func loadDir(dir string) ([]ADR, []string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}
	var (
		adrs     []ADR
		problems []string
	)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".md") || name == "README.md" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, nil, err
		}
		adr, err := parseADR(name, string(raw))
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s/%s: %v", dir, name, err))
			continue
		}
		adrs = append(adrs, adr)
	}
	sortADRs(adrs)

	// The slug is the citation handle, so it has to be unique across all dates,
	// not just within one.
	seen := map[string]string{}
	for _, a := range adrs {
		if prev, dup := seen[a.Slug]; dup {
			problems = append(problems, fmt.Sprintf(
				"%s/%s: slug %q is already used by %s — the slug is the citation handle and must be unique",
				dir, a.Name, a.Slug, prev))
			continue
		}
		seen[a.Slug] = a.Name
	}
	return adrs, problems, nil
}

func sortADRs(adrs []ADR) {
	sort.Slice(adrs, func(i, j int) bool {
		if adrs[i].Date != adrs[j].Date {
			return adrs[i].Date < adrs[j].Date
		}
		return adrs[i].Slug < adrs[j].Slug
	})
}

type sourceFile struct {
	Path    string
	Content string
}

// isReleasedMigration reports paths that check:migrations pins byte-for-byte
// against the last release tag. Their comments cannot be rewritten, so the
// legacy citations a few of them carry are exempt rather than fixable.
func isReleasedMigration(path string) bool {
	return strings.HasPrefix(path, "cmd/desktop/internal/app/data/queries/migrations/")
}

// isCheckerFixture reports this tool's own sources, whose test fixtures are
// deliberately malformed citations.
func isCheckerFixture(path string) bool {
	return strings.HasPrefix(path, "cmd/tools/adr/")
}

// checkRefs verifies every citation and every link into the decisions directory
// resolves. This is what turns a rename or a typo into a failing gate instead of
// a dangling reference nobody notices.
func checkRefs(adrs []ADR, files []sourceFile) []string {
	slugs := make(map[string]bool, len(adrs))
	names := make(map[string]bool, len(adrs))
	for _, a := range adrs {
		slugs[a.Slug] = true
		names[a.Name] = true
	}

	var problems []string
	for _, f := range files {
		if isReleasedMigration(f.Path) || isCheckerFixture(f.Path) {
			continue
		}
		for i, raw := range strings.Split(f.Content, "\n") {
			// A backticked id is being quoted, not cited — this is how the
			// migration ADR and the docs discuss the retired numbers.
			line := codeSpanRe.ReplaceAllString(raw, "``")
			at := func(format string, args ...any) {
				problems = append(problems, fmt.Sprintf("%s:%d: %s", f.Path, i+1, fmt.Sprintf(format, args...)))
			}

			if m := legacyRe.FindStringSubmatch(line); m != nil {
				at("legacy numbered ADR reference %q — cite the slug instead",
					strings.TrimSpace(m[0]))
			}

			for _, loc := range citeRe.FindAllStringSubmatchIndex(line, -1) {
				slug := line[loc[2]:loc[3]]
				if !slugs[slug] {
					at("cites unknown ADR %q", slug)
				}
				// Walk the ", x and y" tail of a multi-ADR citation.
				rest := line[loc[1]:]
				for {
					more := citeMore.FindStringSubmatch(rest)
					if more == nil {
						break
					}
					if !slugs[more[1]] {
						at("cites unknown ADR %q", more[1])
					}
					rest = rest[len(more[0]):]
				}
			}

			for _, m := range mdLinkRe.FindAllStringSubmatch(line, -1) {
				if !names[m[2]] {
					at("links to missing decision file %q", m[2])
				}
			}
		}
	}
	sort.Strings(problems)
	return problems
}

func slugify(title string) string {
	var b strings.Builder
	prevDash := true
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

const newTemplate = `# %s

- **Status:** proposed
- **Date:** %s

## Context

## Decision

## Consequences
`
