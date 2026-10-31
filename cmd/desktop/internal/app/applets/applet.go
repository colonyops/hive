// Package applets holds the on-disk catalog of applets: web pages an agent or a user
// writes, which Hive opens over the app with full access to the machine through
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

	"github.com/colonyops/hive/cmd/desktop/internal/app/icons"
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
	Builtin bool   `yaml:"-"`
	Version int    `yaml:"version"`
	Title   string `yaml:"title"`
	Icon    string `yaml:"icon,omitempty"`
	// Cwd pins the applet's commands to one directory. Empty follows the session on
	// screen, and the applet's own directory without one.
	Cwd string `yaml:"cwd,omitempty"`
	// Entry is the page the applet opens on, relative to its directory.
	Entry string `yaml:"entry,omitempty"`
	// Width and Height size the applet's panel in pixels. Zero fills the window.
	Width  int `yaml:"width,omitempty"`
	Height int `yaml:"height,omitempty"`
	// Titlebar offers a page for the title bar. It shows once the user turns it on, so a
	// file on disk neither starts running nor claims room in the bar by itself.
	Titlebar *Titlebar `yaml:"titlebar,omitempty"`
	// Commands are the applet's palette rows that a user can bind to a key, as
	// applet.<id>.<command>. They carry no default chord, for the reason applet.<id> has none.
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

// Titlebar is the page an applet can show in the title bar.
type Titlebar struct {
	// Entry is relative to the applet's directory. Empty uses the applet's Entry, and
	// the page tells the two apart with hive.surface.
	Entry string `yaml:"entry,omitempty"`
	// Width is the slot's width in pixels until the page asks for another. Zero is
	// DefaultTitlebarWidth, one icon button.
	Width int `yaml:"width,omitempty"`
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
	if a.Entry == "" {
		a.Entry = DefaultEntry
	}
	if a.Titlebar != nil {
		if a.Titlebar.Entry == "" {
			a.Titlebar.Entry = a.Entry
		}
		if a.Titlebar.Width == 0 {
			a.Titlebar.Width = DefaultTitlebarWidth
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
	case !icons.ValidLauncher(a.Icon):
		return fmt.Errorf("unknown icon %q", a.Icon)
	case !filepath.IsLocal(a.Entry):
		return fmt.Errorf("entry must be a path inside the applet's directory, got %q", a.Entry)
	case a.Width < 0 || a.Height < 0:
		return errors.New("width and height must not be negative")
	case a.Titlebar != nil && !filepath.IsLocal(a.Titlebar.Entry):
		return fmt.Errorf("titlebar.entry must be a path inside the applet's directory, got %q", a.Titlebar.Entry)
	case a.Titlebar != nil && a.Titlebar.Width < 0:
		return errors.New("titlebar.width must not be negative")
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
		case !icons.ValidLauncher(command.Icon):
			return fmt.Errorf("commands[%d].icon: unknown icon %q", i, command.Icon)
		}
		seen[command.ID] = true
	}
	return nil
}
