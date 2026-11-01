package applets

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/colonyops/hive/cmd/desktop/internal/app/icons"
)

const (
	// FormatVersion is the applet.yaml format this Hive reads. It rises only for a change an
	// older Hive would misread, which is what keeps files working across releases.
	FormatVersion = 1

	maxWarnings = 20
)

// UpdateHiveError reports a file this Hive is too old for. It stands on its own, so an
// import shows it as is rather than calling the file invalid.
type UpdateHiveError struct{ message string }

func (e *UpdateHiveError) Error() string { return e.message }

var (
	errBadMinimumHive = errors.New(`hive must look like ">=0.60"`)

	minimumHive = regexp.MustCompile(`^>=\s*(\d+)\.(\d+)(?:\.(\d+))?$`)
	releaseOf   = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)`)
	// plainValue is what a warning may quote. A warning reaches the import dialog, so a
	// name from a hostile file must not bring control or bidi characters with it.
	plainValue = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
)

// checkCompatibility runs before the file's shape is decoded: under a later format or a
// later Hive, a key this one knows could mean something else, and the useful message is
// "update Hive" rather than a type error. An unreadable hive: refuses, because it is the
// one key an author can use to keep an older Hive from running the file.
func checkCompatibility(doc *yaml.Node, hostVersion string) error {
	// Decoded rather than read off the keys, so a merge or an alias key means here what it
	// means to the decode that follows. A type error still fills the fields that fit, and
	// that decode reports it.
	var gate struct {
		Version int    `yaml:"version"`
		Hive    string `yaml:"hive"`
	}
	_ = doc.Decode(&gate)
	if gate.Version > FormatVersion {
		return &UpdateHiveError{fmt.Sprintf("this applet uses file format %d, and this version of Hive reads format %d. Update Hive to use it", gate.Version, FormatVersion)}
	}
	if gate.Hive == "" {
		if hasRootKey(doc, "hive") {
			return errBadMinimumHive
		}
		return nil
	}
	match := minimumHive.FindStringSubmatch(strings.TrimSpace(gate.Hive))
	if match == nil {
		return errBadMinimumHive
	}
	needed, ok := releaseNumbers(match[1:])
	if !ok {
		return errBadMinimumHive
	}
	running := releaseOf.FindStringSubmatch(hostVersion)
	if running == nil {
		// A development build has no release to compare, and stands for the newest.
		return nil
	}
	if have, ok := releaseNumbers(running[1:]); ok && slices.Compare(have, needed) < 0 {
		minimum := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(gate.Hive), ">="))
		return &UpdateHiveError{fmt.Sprintf("this applet needs Hive %s or later, and this is Hive %s. Update Hive to use it", minimum, strings.TrimPrefix(running[0], "v"))}
	}
	return nil
}

func releaseNumbers(parts []string) ([]int, bool) {
	numbers := make([]int, 3)
	for i, part := range parts {
		if part == "" {
			continue
		}
		number, err := strconv.Atoi(part)
		if err != nil {
			return nil, false
		}
		numbers[i] = number
	}
	return numbers, true
}

// hasRootKey finds a key the gate read as empty, such as a null or a list, which must
// refuse rather than count as no requirement.
func hasRootKey(doc *yaml.Node, key string) bool {
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return false
	}
	root := doc.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == key {
			return true
		}
	}
	return false
}

// ignoredKeys lists the keys Applet has no field for, at any depth, read from the struct's
// own tags so a new field needs no change here. The decode that follows skips them.
func ignoredKeys(doc *yaml.Node) []string {
	walk := ignoredKeyWalk{seen: map[string]bool{}, visited: map[ignoredKeyVisit]bool{}}
	walk.visit(doc, reflect.TypeFor[Applet](), "")
	keys := make([]string, 0, len(walk.seen))
	for key := range walk.seen {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	warnings := make([]string, 0, len(keys))
	for _, key := range keys {
		warnings = append(warnings, fmt.Sprintf("this version of Hive ignores %q", key))
	}
	return warnings
}

type ignoredKeyVisit struct {
	node *yaml.Node
	kind reflect.Type
	path string
}

// ignoredKeyWalk remembers what it has been through, because aliases can reach one node
// many times over.
type ignoredKeyWalk struct {
	seen    map[string]bool
	visited map[ignoredKeyVisit]bool
}

func (w ignoredKeyWalk) visit(node *yaml.Node, t reflect.Type, path string) {
	for node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		node = node.Content[0]
	}
	if node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	step := ignoredKeyVisit{node, t, path}
	if w.visited[step] {
		return
	}
	w.visited[step] = true

	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() == reflect.Slice {
		if node.Kind == yaml.SequenceNode {
			for _, item := range node.Content {
				w.visit(item, t.Elem(), path)
			}
		}
		return
	}
	if t.Kind() != reflect.Struct || node.Kind != yaml.MappingNode {
		return
	}
	fields := yamlFields(t)
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode, value := node.Content[i], node.Content[i+1]
		if keyNode.ShortTag() == "!!merge" {
			w.visitMerged(value, t, path)
			continue
		}
		key := mappingKey(keyNode)
		if field, known := fields[key]; known {
			w.visit(value, field, path+key+".")
			continue
		}
		if !plainValue.MatchString(key) {
			key = "a key with an unusual name"
		}
		w.seen[path+key] = true
	}
}

func (w ignoredKeyWalk) visitMerged(value *yaml.Node, t reflect.Type, path string) {
	if value.Kind == yaml.AliasNode {
		value = value.Alias
	}
	if value.Kind == yaml.SequenceNode {
		for _, item := range value.Content {
			w.visitMerged(item, t, path)
		}
		return
	}
	w.visit(value, t, path)
}

// mappingKey reads a key as the decoder does, through an alias or a binary tag.
func mappingKey(node *yaml.Node) string {
	if node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	var key string
	if node.Kind != yaml.ScalarNode || node.Decode(&key) != nil {
		return ""
	}
	return key
}

func yamlFields(t reflect.Type) map[string]reflect.Type {
	fields := make(map[string]reflect.Type, t.NumField())
	for field := range t.Fields() {
		name, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
		if name != "" && name != "-" {
			fields[name] = field.Type
		}
	}
	return fields
}

// dropUnknownIcons falls back to the default glyph for an icon from a later release. It
// returns warnings with its own added, capped so a hostile file cannot fill the dialog.
func (a *Applet) dropUnknownIcons(warnings []string) []string {
	unknown := func(icon string) string {
		if !plainValue.MatchString(icon) {
			icon = "with an unusual name"
		}
		return fmt.Sprintf("this version of Hive has no icon %q, so it shows the default", icon)
	}
	if !icons.ValidLauncher(a.Icon) {
		warnings = append(warnings, unknown(a.Icon))
		a.Icon = ""
	}
	for i := range a.Commands {
		if icons.ValidLauncher(a.Commands[i].Icon) {
			continue
		}
		if warning := unknown(a.Commands[i].Icon); !slices.Contains(warnings, warning) {
			warnings = append(warnings, warning)
		}
		a.Commands[i].Icon = ""
	}
	if len(warnings) > maxWarnings {
		more := len(warnings) - maxWarnings
		warnings = append(warnings[:maxWarnings:maxWarnings], fmt.Sprintf("and %d more", more))
	}
	return warnings
}
