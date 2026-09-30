package report

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

// plantedSecrets are written into every config surface the bundle reads. None
// may survive into the compressed payload.
var plantedSecrets = []string{
	"WEBHOOK_SECRET_ABC123",
	"ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	"sk_live_PLAINSECRET999",
	"ghp_BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB",
	"TOPSECRETVALUE",
	"TOKENVALUE123456789",
	"MYKEYSHOULDGO",
	"ghp_CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC",
	"ghp_DDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD",
}

func writeFixtures(t *testing.T) settings.Paths {
	t.Helper()
	dir := t.TempDir()
	flows := filepath.Join(dir, "flows")
	if err := os.MkdirAll(flows, 0o755); err != nil {
		t.Fatal(err)
	}

	write := func(path, content string) {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write(filepath.Join(dir, "settings.yaml"), `
polling:
  interval: 5m
updates:
  enabled: true
  channel: beta
custom:
  api_key: MYKEYSHOULDGO
`)

	write(filepath.Join(dir, "actions.yml"), `
version: 1
actions:
  - id: deploy
    type: shell
    config:
      env:
        API_SECRET: TOPSECRETVALUE
      command_template: "curl -H 'Authorization: Bearer TOKENVALUE123456789'"
`)

	write(filepath.Join(flows, "main.yaml"), `
version: 1
nodes:
  - id: src
    type: sources.webhook
    config:
      path: /hook
      secret: WEBHOOK_SECRET_ABC123
  - id: act
    type: action
    config:
      type: shell
      command_template: "deploy --token ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
      env:
        DEPLOY_KEY: sk_live_PLAINSECRET999
        GITHUB_TOKEN: ghp_BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB
`)

	write(filepath.Join(flows, "main.ui.yaml"), "should: be skipped\n")

	write(filepath.Join(dir, "desktop.log"),
		"2026-07-27 INFO starting up\n"+
			"2026-07-27 DEBUG using token ghp_CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC now\n")

	return settings.Paths{
		SettingsPath: filepath.Join(dir, "settings.yaml"),
		ActionsPath:  filepath.Join(dir, "actions.yml"),
		FlowsDir:     flows,
		LogFile:      filepath.Join(dir, "desktop.log"),
	}
}

func TestAssembleRedactsEverySecret(t *testing.T) {
	paths := writeFixtures(t)
	a := NewAssembler(paths, Build{Version: "1.2.3", Commit: "abcdef1", Date: "2026-07-27"})

	bundle := a.Assemble("rpt_test", time.Now().UTC(), Options{
		Channel:         "beta",
		IncludeLogs:     true,
		IncludeSettings: true,
		IncludeFlows:    true,
		IncludeActions:  true,
	})

	gz, err := GzipJSON(bundle)
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	if len(gz) > MaxBundleBytes {
		t.Fatalf("bundle over cap: %d", len(gz))
	}

	payload := decompress(t, gz)

	for _, secret := range plantedSecrets {
		if strings.Contains(payload, secret) {
			t.Errorf("planted secret leaked into bundle: %q", secret)
		}
	}

	mustContain := []string{
		"1.2.3",       // build version
		"beta",        // update channel
		"[REDACTED]",  // redaction sentinel
		"main.yaml",   // flow file included
		"starting up", // log tail included
	}
	for _, want := range mustContain {
		if !strings.Contains(payload, want) {
			t.Errorf("bundle missing expected content: %q", want)
		}
	}

	if strings.Contains(payload, "should: be skipped") {
		t.Error("layout sidecar (*.ui.yaml) should not be bundled")
	}
}

// Build info is unconditional; every other surface names the user or their
// work and only travels when the reporter asks for it.
func TestAssembleOptOut(t *testing.T) {
	paths := writeFixtures(t)
	a := NewAssembler(paths, Build{Version: "1.0.0"})

	bundle := a.Assemble("rpt_test", time.Now().UTC(), Options{IncludeFlows: true})
	if bundle.Build.Version != "1.0.0" {
		t.Error("build info should always be present")
	}
	if bundle.Logs != nil {
		t.Error("logs included without an opt-in")
	}
	if bundle.Config.Settings != nil {
		t.Error("settings included without an opt-in")
	}
	if bundle.Config.Actions != nil {
		t.Error("actions included without an opt-in")
	}
	if len(bundle.Config.Flows) == 0 {
		t.Error("flows should be included when opted in")
	}

	payload := decompress(t, mustGzip(t, bundle))
	if strings.Contains(payload, "starting up") {
		t.Error("log content present without a log opt-in")
	}
}

func TestAssembleMissingConfigIsNotFatal(t *testing.T) {
	a := NewAssembler(settings.Paths{
		SettingsPath: "/nonexistent/settings.yaml",
		ActionsPath:  "/nonexistent/actions.yml",
		FlowsDir:     "/nonexistent/flows",
		LogFile:      "/nonexistent/desktop.log",
	}, Build{Version: "1.0.0"})

	bundle := a.Assemble("rpt_test", time.Now().UTC(), Options{
		IncludeLogs:     true,
		IncludeSettings: true,
		IncludeFlows:    true,
		IncludeActions:  true,
	})
	if bundle.Build.Version != "1.0.0" {
		t.Fatal("build info missing")
	}
	if bundle.Logs != nil || bundle.Config.Settings != nil || len(bundle.Config.Flows) != 0 {
		t.Fatal("missing files should produce empty sections, not errors")
	}
}

func TestInventoryCounts(t *testing.T) {
	a := NewAssembler(writeFixtures(t), Build{Version: "1.0.0"})
	inv := a.Inventory()

	if !inv.HasSettings || !inv.HasActions {
		t.Errorf("expected settings and actions present: %+v", inv)
	}
	if inv.FlowCount != 1 {
		t.Errorf("expected 1 flow (sidecar excluded), got %d", inv.FlowCount)
	}
	if !inv.HasLogs || inv.LogBytes == 0 {
		t.Errorf("expected a non-empty log tail: %+v", inv)
	}
}

func decompress(t *testing.T, gz []byte) string {
	t.Helper()
	r, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	raw, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read gzip: %v", err)
	}
	return string(raw)
}

func mustGzip(t *testing.T, b *Bundle) []byte {
	t.Helper()
	gz, err := GzipJSON(b)
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	return gz
}
