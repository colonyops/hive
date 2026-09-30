package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/pkg/tmpl"
)

// RenderClipboardText renders a clipboard action's text_template over the item
// — the same OutputData context a shell action's command_template renders over,
// with the shq helper available — and trims it. A blank render is an error, not
// an empty copy.
//
// It is the single renderer both the detail-pane render-only path
// (InboxService.RenderClipboardAction) and ClipboardExecutor call, so the text
// a user copies can never drift from what a durable dispatch would produce.
func RenderClipboardText(action actions.Action, key string, payload []byte, inputs map[string]string) (string, error) {
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return "", fmt.Errorf("clipboard: decode payload: %w", err)
	}
	return renderClipboardOver(action, OutputData{
		Key:     key,
		Payload: decoded,
		Raw:     json.RawMessage(payload),
		Inputs:  inputs,
	})
}

// renderClipboardOver is the render itself, over whichever context the caller
// assembled — a feed item's payload, or a terminal target's session and
// window. Keeping it one function is what stops the copied text from drifting
// from what a dispatch of the same action would produce.
func renderClipboardOver(action actions.Action, data OutputData) (string, error) {
	cfg, ok := action.Config.(*actions.ClipboardConfig)
	if !ok {
		return "", fmt.Errorf("clipboard: action %q has config type %T", action.ID, action.Config)
	}
	text, err := tmpl.New(tmpl.Config{}).Render(cfg.TextTemplate, data)
	if err != nil {
		return "", fmt.Errorf("clipboard: text_template: %w", err)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("clipboard: text_template rendered blank")
	}
	return text, nil
}

// ClipboardExecutor renders a clipboard action's text_template and returns the
// text as its outcome. A clipboard action is never enqueued as an
// output_command — both the detail pane and the terminal surfaces render it
// non-durably, because text is not a side effect worth replaying — so this
// executor is what the render-only paths dispatch through, and what keeps the
// dispatch contract complete (every catalog action type has one).
type ClipboardExecutor struct{}

func NewClipboardExecutor() *ClipboardExecutor { return &ClipboardExecutor{} }

func (e *ClipboardExecutor) Execute(_ context.Context, action actions.Action, data OutputData, _ ActionInvocationInput) (ExecutionResult, error) {
	text, err := renderClipboardOver(action, data)
	if err != nil {
		return ExecutionResult{}, err
	}
	return ExecutionResult{Attempted: true, Outcome: &ExecutionOutcome{Clipboard: &ClipboardExecutionOutcome{Text: text}}}, nil
}
