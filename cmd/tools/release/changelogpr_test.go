package main

import (
	"strings"
	"testing"

	"github.com/colonyops/hive/cmd/desktop/releasenotes"
)

const promotedStatus = `?? cmd/desktop/releasenotes/changelog/0.5.0.md
 D cmd/desktop/releasenotes/changelog/unreleased/20260912T135002-a-thing.md
 D cmd/desktop/releasenotes/changelog/unreleased/20260912T135003-another-thing.md
`

func TestParsePromotedStatus(t *testing.T) {
	found, err := parsePromotedStatus(promotedStatus)
	if err != nil {
		t.Fatalf("parsePromotedStatus: %v", err)
	}
	if found.version != "0.5.0" {
		t.Errorf("version = %q, want 0.5.0", found.version)
	}
	if found.entryPath != "cmd/desktop/releasenotes/changelog/0.5.0.md" {
		t.Errorf("entryPath = %q", found.entryPath)
	}
	if len(found.fragments) != 2 {
		t.Errorf("fragments = %d, want 2", len(found.fragments))
	}
}

// The whole point of reading the worktree is that the release-notes commit
// holds the entry and nothing else. Unrelated work has to stop it.
func TestParsePromotedStatusRefusesUnrelatedChanges(t *testing.T) {
	_, err := parsePromotedStatus(promotedStatus + " M desktop/frontend/src/App.vue\n")
	if err == nil {
		t.Fatal("expected an unrelated change to be refused")
	}
	if !strings.Contains(err.Error(), "App.vue") {
		t.Fatalf("error should name the unrelated file, got %q", err)
	}
}

func TestParsePromotedStatusRequiresAPromotion(t *testing.T) {
	for name, status := range map[string]string{
		"nothing at all": "",
		"only deletions": " D cmd/desktop/releasenotes/changelog/unreleased/20260912T135002-a-thing.md\n",
		"only an entry":  "?? cmd/desktop/releasenotes/changelog/0.5.0.md\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parsePromotedStatus(status); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestParsePromotedStatusRefusesTwoEntries(t *testing.T) {
	_, err := parsePromotedStatus(promotedStatus + "?? cmd/desktop/releasenotes/changelog/0.6.0.md\n")
	if err == nil || !strings.Contains(err.Error(), "two new changelog entries") {
		t.Fatalf("expected two entries to be refused, got %v", err)
	}
}

// Promotion writes an empty summary on purpose. Nothing downstream fails on
// one, so this is the only place that can catch it before it ships.
func TestValidatePromotedEntryRequiresASummary(t *testing.T) {
	err := validatePromotedEntry(releasenotes.Entry{Version: "0.5.0", Body: "## Added\n\n- a thing"}, "0.5.0.md")
	if err == nil || !strings.Contains(err.Error(), "no summary") {
		t.Fatalf("expected an empty summary to be refused, got %v", err)
	}

	ok := releasenotes.Entry{Version: "0.5.0", Summary: "A short line.", Body: "## Added\n\n- a thing"}
	if err := validatePromotedEntry(ok, "0.5.0.md"); err != nil {
		t.Fatalf("a complete entry should pass, got %v", err)
	}
}
