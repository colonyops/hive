package usageanalytics

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	CommandCompletedName = "cli.command.completed"
	SessionCreatedName   = "hive.session.created"
	TerminalStartedName  = "terminal.session.started"
)

// Event can only carry properties supplied by the three typed constructors.
type Event struct {
	name       string
	occurredAt time.Time
	properties string
	valid      bool
}

func (e Event) Name() string          { return e.name }
func (e Event) OccurredAt() time.Time { return e.occurredAt }
func (e Event) Properties() string    { return e.properties }
func (e Event) Validate() error {
	if !e.valid || e.occurredAt.IsZero() {
		return fmt.Errorf("invalid usage event")
	}
	return nil
}

// Command is a catalog entry, not a command line or user-supplied argument.
type Command struct{ path string }

var commandPaths = strings.Fields(`new batch prune doctor config detect ls review hc
session.list session.info session.show session.create session.update session.delete session.recycle
ctx.init ctx.ls ctx.prune
msg.pub msg.sub msg.list msg.topic msg.inbox
doc.migrate doc.messaging
todo.add todo.list todo.update
hc.create hc.list hc.show hc.update hc.next hc.comment hc.context hc.prune
workspace.list
x.pick x.assess.file x.assess.watch x.assess.replay x.assess.scenario x.assess.drive`)

func LookupCommand(path string) (Command, bool) {
	for _, allowed := range commandPaths {
		if path == allowed {
			return Command{path: path}, true
		}
	}
	return Command{}, false
}

func CommandCompleted(command Command, success bool, duration time.Duration) Event {
	if command.path == "" {
		return Event{}
	}
	duration = max(0, min(duration, 24*time.Hour))
	outcome := "failure"
	if success {
		outcome = "success"
	}
	return event(CommandCompletedName, struct {
		Command    string `json:"command"`
		Outcome    string `json:"outcome"`
		DurationMS int64  `json:"duration_ms"`
	}{command.path, outcome, duration.Milliseconds()})
}

func SessionCreated(strategy string, reusedCheckout bool) Event {
	if strategy != "full" && strategy != "worktree" {
		return Event{}
	}
	return event(SessionCreatedName, struct {
		CloneStrategy  string `json:"clone_strategy"`
		ReusedCheckout bool   `json:"reused_checkout"`
	}{strategy, reusedCheckout})
}

func TerminalStarted(scratch bool) Event {
	kind := "hive"
	if scratch {
		kind = "scratch"
	}
	return event(TerminalStartedName, struct {
		Kind string `json:"kind"`
	}{kind})
}

func event(name string, properties any) Event {
	data, err := json.Marshal(properties)
	return Event{name: name, occurredAt: time.Now().UTC(), properties: string(data), valid: err == nil}
}

// Captured is the process-enriched event passed to a local sink.
type Captured struct {
	Event
	ID             string
	RunID          string
	Surface        string
	AppVersion     string
	ReleaseChannel string
}

func (e Captured) Validate() error {
	if err := e.Event.Validate(); err != nil {
		return err
	}
	if _, err := uuid.Parse(e.ID); err != nil {
		return fmt.Errorf("invalid usage event ID: %w", err)
	}
	if _, err := uuid.Parse(e.RunID); err != nil {
		return fmt.Errorf("invalid usage run ID: %w", err)
	}
	if e.Surface != "cli" && e.Surface != "desktop" {
		return fmt.Errorf("invalid usage surface")
	}
	if len(e.AppVersion) > 128 {
		return fmt.Errorf("usage app version exceeds limit")
	}
	switch e.ReleaseChannel {
	case "stable", "beta", "development":
	default:
		return fmt.Errorf("invalid usage release channel")
	}
	return nil
}

// Noop records nothing and owns no resources.
type Noop struct{}

func (Noop) Record(context.Context, Event) {}
