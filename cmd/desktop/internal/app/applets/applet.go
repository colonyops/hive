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
	}
	return nil
}
