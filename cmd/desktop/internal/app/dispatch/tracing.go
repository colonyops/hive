package dispatch

import "github.com/colonyops/hive/internal/platform/observe"

var tracer = observe.Tracer("/internal/app/dispatch")

// A dispatched action is a trigger, so Dispatcher.Execute roots one span per
// run and every executor below it opens a conditional child for the one thing
// it waits on. A child carries the name actions.yml uses for it, so a span
// name needs no translation back to the file that produced it. The clipboard
// executor has no child at all: it renders text and waits on nothing outside
// this process
// (ADR a-span-is-a-trigger-or-a-wait-and-its-count-per-trigger-is-bounded-by-configuration).
//
// Span attributes, not metric labels, which is why an unbounded action id,
// item key and repository are safe here.
const (
	attrActionID   = "dispatch.action.id"
	attrActionType = "dispatch.action.type"
	attrTarget     = "dispatch.action.target"
	attrCommandID  = "dispatch.command.id"
	attrRerun      = "dispatch.command.rerun"
	attrAttempted  = "dispatch.attempted"
	attrAgent      = "dispatch.session.agent"
	attrRepo       = "dispatch.session.repo"
	attrWorkspace  = "dispatch.session.workspace"
	attrSeverity   = "dispatch.notify.severity"
	attrInApp      = "dispatch.notify.in_app"
	attrTopic      = "dispatch.message.topic"
)
