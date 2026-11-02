// Package applets holds the on-disk catalog of applets: web pages an agent or a user
// writes, which Hive opens with full access to the machine through
// window.hive (ADR applets-are-web-pages-with-full-access-to-the-machine).
//
// This package parses and validates. It never executes a command.
package applets

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/colonyops/hive/cmd/desktop/internal/app/slug"
)

const (
	// FileName is the file that makes a directory under the applets root an applet.
	FileName = "applet.yaml"
	// DefaultEntry is the page an applet opens on.
	DefaultEntry = "index.html"
	// DefaultTitlebarWidth matches the bar's square icon buttons.
	DefaultTitlebarWidth = 28
)

// Applet is one applet.yaml. ID is the name of its directory, so an applet can
// be copied or renamed by moving the directory.
type Applet struct {
	ID string `yaml:"-"`
	// Dir is the applet's directory, exported to its commands as HIVE_APPLET_DIR. A
	// built-in has its page in the binary, and keeps only its saved storage here.
	Dir string `yaml:"-"`
	// Builtin marks an applet Hive ships. A directory of the same name replaces it.
	Builtin bool `yaml:"-"`
	// Warnings names what this Hive skipped in the file, such as a key from a later
	// release. The applet still runs.
	Warnings []string `yaml:"-"`
	// Version is the format of this file, not a release of the applet.
	Version int `yaml:"version"`
	// Hive is the oldest Hive that can run this applet, as ">=0.60".
	Hive  string `yaml:"hive,omitempty"`
	Title string `yaml:"title"`
	Icon  string `yaml:"icon,omitempty"`
	// Cwd pins the applet's commands to one directory. Empty follows the session on
	// screen, and the applet's own directory without one.
	Cwd string `yaml:"cwd,omitempty"`
	// Entry is the page the applet opens on, relative to its directory.
	Entry string `yaml:"entry,omitempty"`
	// Surfaces says where the applet shows. A file that names none is a pop-up.
	Surfaces Surfaces `yaml:"surfaces,omitempty"`
	// Commands are the applet's palette rows that a user can bind to a key, as
	// applet.<id>.<command>. They carry no default chord, so a file cannot claim a key.
	Commands []Command `yaml:"commands,omitempty"`
}

// Command is one thing an applet offers the palette. Running it reaches the page through
// hive.on('command').
type Command struct {
	// ID is permanent once a user has bound it: it is what settings.yaml names.
	ID       string   `yaml:"id"`
	Title    string   `yaml:"title"`
	Icon     string   `yaml:"icon,omitempty"`
	Keywords []string `yaml:"keywords,omitempty"`
}

// Surfaces is the registration: an applet shows where its file says, and nowhere else. A
// surface takes a map so a later release can add to it without moving a key.
type Surfaces struct {
	// Tab puts the applet in a session's + menu.
	Tab *TabSurface `yaml:"tab,omitempty"`
	// Popup opens the applet over the app, from the palette or a chord.
	Popup *PopupSurface `yaml:"popup,omitempty"`
	// Titlebar runs a page in the title bar from the moment the file loads.
	Titlebar *TitlebarSurface `yaml:"titlebar,omitempty"`
}

// UnmarshalYAML reads a bare "tab:" as asking for the surface. The decoder leaves a pointer
// nil for a null value, which would drop the surface with no warning.
func (s *Surfaces) UnmarshalYAML(node *yaml.Node) error {
	type plain Surfaces
	if err := node.Decode((*plain)(s)); err != nil {
		return err
	}
	if s.Tab == nil && hasKey(node, "tab") {
		s.Tab = &TabSurface{}
	}
	if s.Popup == nil && hasKey(node, "popup") {
		s.Popup = &PopupSurface{}
	}
	if s.Titlebar == nil && hasKey(node, "titlebar") {
		s.Titlebar = &TitlebarSurface{}
	}
	return nil
}

func (s Surfaces) none() bool { return s.Tab == nil && s.Popup == nil && s.Titlebar == nil }

// ShowsTab, ShowsPopup and ShowsTitlebar say whether the surface is listed and not turned off.
func (s Surfaces) ShowsTab() bool      { return s.Tab != nil && isOn(s.Tab.Enabled) }
func (s Surfaces) ShowsPopup() bool    { return s.Popup != nil && isOn(s.Popup.Enabled) }
func (s Surfaces) ShowsTitlebar() bool { return s.Titlebar != nil && isOn(s.Titlebar.Enabled) }

func isOn(enabled *bool) bool { return enabled == nil || *enabled }

// TabSurface takes its size from its panel.
type TabSurface struct {
	// Enabled false turns the surface off and keeps its settings. Unset is on.
	Enabled *bool `yaml:"enabled,omitempty"`
}

// PopupSurface sizes the pop-up in pixels. Zero fills the window.
type PopupSurface struct {
	Enabled *bool `yaml:"enabled,omitempty"`
	Width   int   `yaml:"width,omitempty"`
	Height  int   `yaml:"height,omitempty"`
}

// TitlebarSurface is the page an applet shows in the title bar.
type TitlebarSurface struct {
	Enabled *bool `yaml:"enabled,omitempty"`
	// Entry is relative to the applet's directory. Empty uses the applet's Entry, and
	// the page tells the two apart with hive.surface.
	Entry string `yaml:"entry,omitempty"`
	// Width is the slot's width in pixels until the page asks for another. Zero is
	// DefaultTitlebarWidth, one icon button.
	Width int `yaml:"width,omitempty"`
}

// Parse decodes one applet.yaml for the Hive at hostVersion. It reads forward: a later
// release's key or icon is a warning, so an applet written there still opens here. A wrong
// shape under a key this Hive knows stays an error, and so does a file that says this Hive
// is too old (ADR applets-are-web-pages-with-full-access-to-the-machine).
func Parse(id string, raw []byte, hostVersion string) (Applet, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	var doc yaml.Node
	if err := decoder.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return Applet{}, errors.New("the file is empty")
		}
		return Applet{}, err
	}
	if err := checkCompatibility(&doc, hostVersion); err != nil {
		return Applet{}, err
	}
	var a Applet
	if err := doc.Decode(&a); err != nil {
		return Applet{}, err
	}
	a.ID = id
	warnings := ignoredKeys(&doc)
	var next yaml.Node
	if err := decoder.Decode(&next); !errors.Is(err, io.EOF) {
		warnings = append(warnings, "this file has a second YAML document, and Hive reads only the first")
	}
	a.Warnings = a.dropUnknownIcons(warnings)
	if a.Entry == "" {
		a.Entry = DefaultEntry
	}
	if a.Surfaces.none() {
		a.Surfaces.Popup = &PopupSurface{}
	}
	if bar := a.Surfaces.Titlebar; bar != nil {
		if bar.Entry == "" {
			bar.Entry = a.Entry
		}
		if bar.Width == 0 {
			bar.Width = DefaultTitlebarWidth
		}
	}
	return a, a.Validate()
}

// Validate reports the first problem with the file.
func (a Applet) Validate() error {
	switch {
	case a.Version != 1:
		return fmt.Errorf("version must be 1, got %d", a.Version)
	case !slug.Valid(a.ID):
		return fmt.Errorf("directory name %q is not a valid slug (lowercase letters, digits, hyphens, starting with a letter or digit, max %d chars)", a.ID, slug.MaxLen)
	case strings.TrimSpace(a.Title) == "":
		return errors.New("title is required")
	case !filepath.IsLocal(a.Entry):
		return fmt.Errorf("entry must be a path inside the applet's directory, got %q", a.Entry)
	case a.Surfaces.Popup != nil && (a.Surfaces.Popup.Width < 0 || a.Surfaces.Popup.Height < 0):
		return errors.New("surfaces.popup.width and height must not be negative")
	case a.Surfaces.Titlebar != nil && !filepath.IsLocal(a.Surfaces.Titlebar.Entry):
		return fmt.Errorf("surfaces.titlebar.entry must be a path inside the applet's directory, got %q", a.Surfaces.Titlebar.Entry)
	case a.Surfaces.Titlebar != nil && a.Surfaces.Titlebar.Width < 0:
		return errors.New("surfaces.titlebar.width must not be negative")
	}
	return a.validateCommands()
}

func (a Applet) validateCommands() error {
	seen := make(map[string]bool, len(a.Commands))
	for i, command := range a.Commands {
		switch {
		case !slug.Valid(command.ID):
			return fmt.Errorf("commands[%d].id %q is not a valid slug", i, command.ID)
		case seen[command.ID]:
			return fmt.Errorf("commands[%d].id %q is used twice", i, command.ID)
		case strings.TrimSpace(command.Title) == "":
			return fmt.Errorf("commands[%d].title is required", i)
		}
		seen[command.ID] = true
	}
	return nil
}
