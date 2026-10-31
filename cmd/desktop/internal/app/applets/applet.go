// Package applets holds the schema and the on-disk catalog of applets: small
// UIs an agent or a user declares in YAML, which Hive renders natively and
// feeds from shell commands (ADR applets-are-declared-in-yaml-and-rendered-natively).
//
// This package parses and validates. It never executes a command.
package applets

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/colonyops/hive/cmd/desktop/internal/app/icons"
	"github.com/colonyops/hive/cmd/desktop/internal/app/slug"
)

// FileName is the file that makes a directory under the applets root an applet.
const FileName = "applet.yaml"

const (
	ViewTable  = "table"
	ViewPicker = "picker"
	// ViewHTML is a page the applet brings itself, with full access to the machine
	// through the bridge in bridge.js.
	ViewHTML = "html"

	// DefaultEntry is the page an html applet opens on.
	DefaultEntry = "index.html"

	FormatJSON  = "json"
	FormatLines = "lines"

	ModeBackground = "background"
	ModeTerminal   = "terminal"

	// LineKey is the field each output line lands in under FormatLines.
	LineKey = "line"

	// minRefresh stops a typo from running a command many times a second for as
	// long as the overlay stays open.
	minRefresh = time.Second
)

var (
	columnTones = map[string]bool{"neutral": true, "success": true, "accent": true, "muted": true, "danger": true}
	// One printable key without a modifier: anything more belongs in
	// settings.yaml, and j, k, g and / already navigate the list.
	actionKey    = regexp.MustCompile(`^[a-z0-9]$`)
	reservedKeys = map[string]bool{"j": true, "k": true, "g": true}
)

// Applet is one applet.yaml. ID is the name of its directory, so an applet can
// be copied or renamed by moving the directory.
type Applet struct {
	ID string `yaml:"-"`
	// Dir is the applet's directory, exported to its commands as HIVE_APPLET_DIR
	// so a script beside applet.yaml can be run wherever the command's cwd is.
	Dir     string `yaml:"-"`
	Version int    `yaml:"version"`
	Title   string `yaml:"title"`
	Icon    string `yaml:"icon,omitempty"`
	View    string `yaml:"view"`
	// Cwd pins the applet's commands to one directory. Empty means the session on
	// screen, and the applet is offered only inside one, as a launcher is (ADR
	// quick-terminal-launchers-are-session-scoped).
	Cwd string `yaml:"cwd,omitempty"`
	// Entry is the page an html applet opens on, relative to its directory.
	Entry string `yaml:"entry,omitempty"`
	// Width and Height size an html applet's panel in pixels. Zero fills the window.
	Width   int      `yaml:"width,omitempty"`
	Height  int      `yaml:"height,omitempty"`
	Data    Data     `yaml:"data,omitempty"`
	Columns []Column `yaml:"columns,omitempty"`
	Actions []Action `yaml:"actions,omitempty"`
}

// Data is the command that produces the applet's rows.
type Data struct {
	Command string `yaml:"command"`
	// Format is how stdout becomes rows: json takes one array of objects or one
	// object per line, lines takes each line as a row with the field LineKey.
	Format string `yaml:"format,omitempty"`
	// Refresh re-runs the command this often while the applet is open. Zero means
	// only on open, on an action, and on request.
	Refresh time.Duration `yaml:"refresh,omitempty"`
}

// Column is one field of a row shown on screen.
type Column struct {
	Key   string `yaml:"key"`
	Label string `yaml:"label,omitempty"`
	// Tones draws the value as a badge, in the tone its value maps to.
	Tones map[string]string `yaml:"tones,omitempty"`
}

// Action is something the user can run against the selected row. The row reaches
// Run through the environment rather than a template, so a value in a row can
// never become shell syntax: see RowEnv.
type Action struct {
	ID    string `yaml:"id"`
	Label string `yaml:"label"`
	Run   string `yaml:"run"`
	// Mode is background, which captures output and refreshes the rows when it
	// exits, or terminal, which opens the command in the pop-up terminal.
	Mode string `yaml:"mode,omitempty"`
	// Key runs the action from the table view.
	Key string `yaml:"key,omitempty"`
	// Confirm is the question asked before running. Empty runs at once.
	Confirm string `yaml:"confirm,omitempty"`
}

// Parse decodes one applet.yaml strictly, so a misspelled key is an error an
// agent can read rather than a feature that silently does nothing.
func Parse(id string, raw []byte) (Applet, error) {
	var a Applet
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&a); err != nil {
		if errors.Is(err, io.EOF) {
			return Applet{}, errors.New("the file is empty")
		}
		return Applet{}, err
	}
	a.ID = id
	if a.Data.Format == "" {
		a.Data.Format = FormatJSON
	}
	if a.View == ViewHTML && a.Entry == "" {
		a.Entry = DefaultEntry
	}
	for i := range a.Actions {
		if a.Actions[i].Mode == "" {
			a.Actions[i].Mode = ModeBackground
		}
	}
	return a, a.Validate()
}

// Validate reports the first problem with the declaration.
func (a Applet) Validate() error {
	switch {
	case a.Version != 1:
		return fmt.Errorf("version must be 1, got %d", a.Version)
	case !slug.Valid(a.ID):
		return fmt.Errorf("directory name %q is not a valid slug (lowercase letters, digits, hyphens, starting with a letter or digit, max %d chars)", a.ID, slug.MaxLen)
	case strings.TrimSpace(a.Title) == "":
		return errors.New("title is required")
	case !icons.ValidLauncher(a.Icon):
		return fmt.Errorf("unknown icon %q", a.Icon)
	case a.View == ViewHTML:
		return a.validateHTML()
	case a.View != ViewTable && a.View != ViewPicker:
		return fmt.Errorf("view must be %s, %s or %s, got %q", ViewTable, ViewPicker, ViewHTML, a.View)
	case strings.TrimSpace(a.Data.Command) == "":
		return errors.New("data.command is required")
	case a.Data.Format != FormatJSON && a.Data.Format != FormatLines:
		return fmt.Errorf("data.format must be %s or %s, got %q", FormatJSON, FormatLines, a.Data.Format)
	case a.Data.Refresh != 0 && a.Data.Refresh < minRefresh:
		return fmt.Errorf("data.refresh must be at least %s, got %s", minRefresh, a.Data.Refresh)
	case len(a.Columns) == 0:
		return errors.New("at least one column is required")
	case a.View == ViewPicker && len(a.Actions) == 0:
		return errors.New("a picker needs at least one action: Enter runs the first")
	}
	for _, c := range a.Columns {
		if err := c.validate(); err != nil {
			return err
		}
	}
	return validateActions(a.Actions)
}

func (a Applet) validateHTML() error {
	switch {
	case !filepath.IsLocal(a.Entry):
		return fmt.Errorf("entry must be a path inside the applet's directory, got %q", a.Entry)
	case a.Width < 0 || a.Height < 0:
		return errors.New("width and height must not be negative")
	case a.Data.Command != "" || len(a.Columns) > 0 || len(a.Actions) > 0:
		return errors.New("an html applet does not take data, columns or actions: the page uses window.hive")
	}
	return nil
}

func (c Column) validate() error {
	if strings.TrimSpace(c.Key) == "" {
		return errors.New("column: key is required")
	}
	for value, tone := range c.Tones {
		if !columnTones[tone] {
			return fmt.Errorf("column %q: tone for %q must be neutral, success, accent, muted or danger, got %q", c.Key, value, tone)
		}
	}
	return nil
}

func validateActions(actions []Action) error {
	ids, keys := map[string]bool{}, map[string]bool{}
	for _, action := range actions {
		switch {
		case !slug.Valid(action.ID):
			return fmt.Errorf("action %q: id is not a valid slug", action.ID)
		case ids[action.ID]:
			return fmt.Errorf("action %q: id is used twice", action.ID)
		case strings.TrimSpace(action.Label) == "":
			return fmt.Errorf("action %q: label is required", action.ID)
		case strings.TrimSpace(action.Run) == "":
			return fmt.Errorf("action %q: run is required", action.ID)
		case action.Mode != ModeBackground && action.Mode != ModeTerminal:
			return fmt.Errorf("action %q: mode must be %s or %s, got %q", action.ID, ModeBackground, ModeTerminal, action.Mode)
		case action.Key != "" && (!actionKey.MatchString(action.Key) || reservedKeys[action.Key]):
			return fmt.Errorf("action %q: key must be one lowercase letter or digit other than j, k or g, got %q", action.ID, action.Key)
		case action.Key != "" && keys[action.Key]:
			return fmt.Errorf("action %q: key %q is used twice", action.ID, action.Key)
		}
		ids[action.ID] = true
		if action.Key != "" {
			keys[action.Key] = true
		}
	}
	return nil
}

// Action finds a declared action by id.
func (a Applet) Action(id string) (Action, bool) {
	for _, action := range a.Actions {
		if action.ID == id {
			return action, true
		}
	}
	return Action{}, false
}
