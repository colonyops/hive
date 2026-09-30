package actions

import (
	"fmt"
	"strings"
)

// ClipboardConfig is a clipboard action: it renders a template over the
// triggering item and puts the result on the clipboard. TextTemplate is a Go
// text/template string (rendered with the shared "shq" helper available, the
// same OutputData context shell's command_template renders over) — this
// package only parses and validates the config, it never renders it. Unlike a
// shell action there is no command and no side effect in the core: the
// executor renders the text, the desktop adapter performs the clipboard write.
//
// A clipboard action is detail-pane only. It has no clipboard target from a
// headless flow, so it is never HeadlessCapable and a flow action node cannot
// reference it.
type ClipboardConfig struct {
	// TextTemplate renders the text placed on the clipboard.
	TextTemplate string `yaml:"text_template"`
}

func (c *ClipboardConfig) Validate() error {
	if strings.TrimSpace(c.TextTemplate) == "" {
		return fmt.Errorf("clipboard: text_template is required")
	}
	return nil
}
