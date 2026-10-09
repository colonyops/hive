package app

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/diagnostics"
	"github.com/colonyops/hive/cmd/desktop/internal/app/prompts"
	"github.com/colonyops/hive/cmd/desktop/internal/app/report"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

type DiagnosticsService struct {
	paths    settings.Paths
	jobs     *JobService
	build    report.Build
	environ  func(context.Context) []string
	hive     *HiveConfigService
	commands func() map[string]string
	webhooks *WebhookService
	reveal   func(string) error
}

type DiagnosticsQuery struct {
	Source      string   `json:"source"`
	Since       string   `json:"since"`
	Until       string   `json:"until"`
	Levels      []string `json:"levels"`
	Search      string   `json:"search"`
	Reference   string   `json:"reference"`
	Limit       int      `json:"limit"`
	OmitRoutine bool     `json:"omitRoutine"`
}

type DiagnosticsSnapshot struct {
	CapturedAt string               `json:"capturedAt"`
	Version    string               `json:"version"`
	Commit     string               `json:"commit"`
	BuildDate  string               `json:"buildDate"`
	Sources    []diagnostics.Source `json:"sources"`
	Entries    []diagnostics.Entry  `json:"entries"`
	Truncated  bool                 `json:"truncated"`
	Query      DiagnosticsQuery     `json:"query"`
}

func (s *DiagnosticsService) Read(ctx context.Context, q DiagnosticsQuery) (DiagnosticsSnapshot, error) {
	out := DiagnosticsSnapshot{CapturedAt: time.Now().UTC().Format(time.RFC3339Nano), Version: s.build.Version, Commit: s.build.Commit, BuildDate: s.build.Date, Entries: make([]diagnostics.Entry, 0)}
	if q.Limit <= 0 {
		q.Limit = 300
	}
	q.Limit = min(q.Limit, 1000)
	out.Query = q
	if q.Source != "" && q.Source != "desktop" && q.Source != "cli" && q.Source != "jobs" {
		return out, Errorf(KindInvalid, "unknown diagnostics source %q", q.Source)
	}
	selectedLevels := make(map[string]struct{}, len(q.Levels))
	for _, level := range q.Levels {
		if level != "debug" && level != "info" && level != "warn" && level != "error" && level != "unknown" {
			return out, Errorf(KindInvalid, "unknown diagnostics level %q", level)
		}
		selectedLevels[level] = struct{}{}
	}
	var since, until time.Time
	for _, bound := range []struct {
		text string
		dest *time.Time
	}{{q.Since, &since}, {q.Until, &until}} {
		if bound.text != "" {
			t, err := time.Parse(time.RFC3339Nano, bound.text)
			if err != nil {
				return out, Wrap(err, KindInvalid, "invalid diagnostics time")
			}
			*bound.dest = t
		}
	}
	if !since.IsZero() && !until.IsZero() && since.After(until) {
		return out, Errorf(KindInvalid, "since must precede until")
	}
	cli := diagnostics.CLIPath(s.environ(ctx), s.paths.HiveDataDir)
	entries := make([]diagnostics.Entry, 0)
	statuses := make(map[string]diagnostics.Source)
	for _, src := range []diagnostics.Source{{ID: "desktop", Path: s.paths.LogFile}, {ID: "cli", Path: cli}} {
		if status, ok := statuses[src.Path]; ok {
			status.ID = src.ID
			out.Sources = append(out.Sources, status)
			if q.Source == "" || q.Source == src.ID {
				out.Truncated = out.Truncated || status.Truncated
			}
			continue
		}
		status, logs := diagnostics.Read(src.ID, src.Path)
		statuses[src.Path] = status
		out.Sources = append(out.Sources, status)
		for _, entry := range logs {
			if q.Source == "" || q.Source == entry.Source {
				entries = append(entries, entry)
			}
		}
		if q.Source == "" || q.Source == src.ID {
			out.Truncated = out.Truncated || status.Truncated
		}
	}
	jobSource := diagnostics.Source{ID: "jobs"}
	var before int64
	if after, ok := strings.CutPrefix(q.Reference, "job-"); ok {
		id, err := strconv.ParseInt(after, 10, 64)
		if err != nil || id <= 0 || id >= math.MaxInt64-11 {
			return out, Errorf(KindInvalid, "invalid job reference")
		}
		before = id + 11
	}
	rows, err := s.jobs.List(ctx, before, 1000)
	if err != nil {
		jobSource.Error = err.Error()
	} else {
		jobSource.Truncated = len(rows) == 1000
		for _, job := range rows {
			if q.Source != "" && q.Source != "jobs" {
				break
			}
			raw, _ := json.Marshal(job)
			severity := "info"
			if job.Error != "" {
				severity = "error"
			}
			message := fmt.Sprintf("%s: %s — %s", job.Label, job.Target, job.Status)
			if job.Error != "" {
				message += " — " + job.Error
			}
			fields := map[string]string{
				"job_id": strconv.FormatInt(job.ID, 10), "status": job.Status.String(), "label": job.Label,
				"action_id": job.ActionID, "target": job.Target, "step": job.Step,
			}
			if job.CommandID != nil {
				fields["command_id"] = strconv.FormatInt(*job.CommandID, 10)
			}
			entries = append(entries, diagnostics.NewEntry(
				"jobs", fmt.Sprintf("job-%d", job.ID), time.UnixMilli(job.UpdatedAt).UTC().Format(time.RFC3339Nano),
				severity, message, string(raw), fields,
			))
		}
	}
	out.Sources = append(out.Sources, jobSource)
	if q.Source == "" || q.Source == "jobs" {
		out.Truncated = out.Truncated || jobSource.Truncated
	}
	sort.SliceStable(entries, func(i, j int) bool {
		a, _ := time.Parse(time.RFC3339Nano, entries[i].Time)
		b, _ := time.Parse(time.RFC3339Nano, entries[j].Time)
		return a.Before(b)
	})
	if q.Reference != "" {
		for i, e := range entries {
			if e.ID == q.Reference {
				start := max(0, i-10)
				var truncated bool
				out.Entries, truncated = boundDiagnosticsEntries(entries[start:min(len(entries), i+11)], q.Limit, i-start)
				out.Truncated = out.Truncated || truncated
				return out, nil
			}
		}
		return out, Errorf(KindNotFound, "evidence %q is outside the retained tail or no longer exists", q.Reference)
	}
	for _, e := range entries {
		if q.OmitRoutine && isRoutineHTTP2xx(e) {
			continue
		}
		t, _ := time.Parse(time.RFC3339Nano, e.Time)
		if (!since.IsZero() || !until.IsZero()) && t.IsZero() {
			continue
		}
		if !since.IsZero() && t.Before(since) {
			continue
		}
		if !until.IsZero() && t.After(until) {
			continue
		}
		if len(selectedLevels) > 0 {
			if _, ok := selectedLevels[e.Level]; !ok {
				continue
			}
		}
		if q.Search != "" && !entryContains(e, q.Search) {
			continue
		}
		out.Entries = append(out.Entries, e)
	}
	var truncated bool
	out.Entries, truncated = boundDiagnosticsEntries(out.Entries, q.Limit, -1)
	out.Truncated = out.Truncated || truncated
	return out, nil
}

func boundDiagnosticsEntries(entries []diagnostics.Entry, limit, anchor int) ([]diagnostics.Entry, bool) {
	truncated := false
	if len(entries) > limit {
		if anchor < 0 {
			entries = entries[len(entries)-limit:]
		} else {
			start := max(0, anchor-(limit-1)/2)
			end := min(len(entries), start+limit)
			start = max(0, end-limit)
			entries = entries[start:end]
			anchor -= start
		}
		truncated = true
	}

	sizes := make([]int, len(entries))
	total := 0
	for i, entry := range entries {
		data, _ := json.Marshal(entry)
		sizes[i] = len(data)
		total += sizes[i]
	}
	for total > 512<<10 && len(entries) > 0 {
		dropFirst := anchor < 0
		if anchor >= 0 {
			left := anchor
			right := len(entries) - 1 - anchor
			dropFirst = left >= right && left > 0
		}
		if dropFirst {
			total -= sizes[0]
			entries = entries[1:]
			sizes = sizes[1:]
			if anchor >= 0 {
				anchor--
			}
		} else {
			total -= sizes[len(sizes)-1]
			entries = entries[:len(entries)-1]
			sizes = sizes[:len(sizes)-1]
		}
		truncated = true
	}
	return entries, truncated
}

func isRoutineHTTP2xx(entry diagnostics.Entry) bool {
	if entry.Level != "info" || !strings.Contains(strings.ToLower(entry.Message), "request complete") {
		return false
	}
	status, err := strconv.Atoi(entry.Fields["status"])
	return err == nil && status >= 200 && status < 300
}

func entryContains(entry diagnostics.Entry, search string) bool {
	search = strings.ToLower(search)
	if strings.Contains(strings.ToLower(entry.ID+entry.Raw+entry.Message), search) {
		return true
	}
	for key, value := range entry.Fields {
		if strings.Contains(strings.ToLower(key+value), search) {
			return true
		}
	}
	return false
}

type DiagnosticsIncident struct {
	Query       DiagnosticsQuery `json:"query"`
	Description string           `json:"description"`
	Agent       string           `json:"agent"`
}

type DiagnosticsContext struct {
	Text    string `json:"text"`
	Path    string `json:"path"`
	Dir     string `json:"dir"`
	Command string `json:"command"`
}

func (s *DiagnosticsService) Context(ctx context.Context, req DiagnosticsIncident) (DiagnosticsContext, error) {
	if len(req.Description) > 8000 {
		return DiagnosticsContext{}, Errorf(KindInvalid, "incident description exceeds 8000 characters")
	}
	snapshot, err := s.Read(ctx, req.Query)
	if err != nil {
		return DiagnosticsContext{}, err
	}
	// Bound the handoff independently of the interactive viewer's row limit.
	for len(snapshot.Entries) > 0 {
		data, _ := json.Marshal(snapshot)
		if len(data) <= 192<<10 {
			break
		}
		snapshot.Entries = snapshot.Entries[1:]
		snapshot.Truncated = true
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return DiagnosticsContext{}, Wrap(err, KindInternal, "encoding diagnostic context")
	}
	endpoint := ""
	if running, port := s.webhooks.Endpoint(ctx); running {
		endpoint = MCPEndpointAt(s.webhooks.Host(), port)
	}
	text, err := prompts.Diagnostics(req.Description, string(data), endpoint)
	if err != nil {
		return DiagnosticsContext{}, Wrap(err, KindInternal, "rendering diagnostic context")
	}
	return DiagnosticsContext{Text: text}, nil
}

func (s *DiagnosticsService) Save(ctx context.Context, req DiagnosticsIncident) (DiagnosticsContext, error) {
	result, err := s.Context(ctx, req)
	if err != nil {
		return result, err
	}
	dir := filepath.Join(s.paths.ReportsDir, "diagnostics")
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return result, Wrap(err, KindInternal, "creating diagnostics directory")
	}
	f, err := os.CreateTemp(dir, "incident-*.txt")
	if err != nil {
		return result, Wrap(err, KindInternal, "creating incident snapshot")
	}
	result.Path = f.Name()
	result.Dir = dir
	_, writeErr := f.WriteString(result.Text)
	closeErr := f.Close()
	if writeErr != nil {
		return result, Wrap(writeErr, KindInternal, "writing incident snapshot")
	}
	if closeErr != nil {
		return result, Wrap(closeErr, KindInternal, "closing incident snapshot")
	}
	return result, nil
}

// Prepare saves evidence and renders a command; only the authenticated PTY
// transport may execute it.
func (s *DiagnosticsService) Prepare(ctx context.Context, req DiagnosticsIncident) (DiagnosticsContext, error) {
	options := s.Agents(ctx)
	agent := req.Agent
	if agent == "" {
		agent = options.DefaultAgent
	}
	command, ok := s.commands()[agent]
	if !ok || command == "" {
		return DiagnosticsContext{}, Errorf(KindUnavailable, "configure a default agent or select an agent profile")
	}
	result, err := s.Save(ctx, req)
	if err != nil {
		return result, err
	}
	prompt, err := prompts.DiagnosticsLaunch(result.Path)
	if err != nil {
		return result, Wrap(err, KindInternal, "rendering investigation prompt")
	}
	result.Command = command + " " + "'" + strings.ReplaceAll(prompt, "'", "'\"'\"'") + "'"
	return result, nil
}

type DiagnosticsAgents struct {
	Names        []string `json:"names"`
	DefaultAgent string   `json:"defaultAgent"`
}

func (s *DiagnosticsService) Agents(ctx context.Context) DiagnosticsAgents {
	setup := s.hive.Setup(ctx)
	result := DiagnosticsAgents{Names: make([]string, 0), DefaultAgent: setup.Config.DefaultAgent}
	if setup.DefaultAgentOverride != "" {
		result.DefaultAgent = setup.DefaultAgentOverride
	}
	for name := range s.commands() {
		result.Names = append(result.Names, name)
	}
	sort.Strings(result.Names)
	return result
}

func (s *DiagnosticsService) Reveal(ctx context.Context, source string) error {
	var path string
	switch source {
	case "desktop":
		path = s.paths.LogFile
	case "cli":
		path = diagnostics.CLIPath(s.environ(ctx), s.paths.HiveDataDir)
	case "exports":
		path = s.paths.ReportsDir
	default:
		return Errorf(KindInvalid, "unknown diagnostics source %q", source)
	}
	return Wrap(s.reveal(path), KindInternal, "revealing diagnostics source")
}
