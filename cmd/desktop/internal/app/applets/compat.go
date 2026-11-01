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
	root := doc
	if root.Kind == yaml.DocumentNode && len(root.Content) == 1 {
		root = root.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		return nil
	}
	if value := mappingValue(root, "version"); value != nil {
		var version int
		if value.Decode(&version) == nil && version > FormatVersion {
			return &UpdateHiveError{fmt.Sprintf("this applet uses file format %d, and this version of Hive reads format %d. Update Hive to use it", version, FormatVersion)}
		}
	}
	value := mappingValue(root, "hive")
	if value == nil {
		return nil
	}
	var requirement string
	if value.Decode(&requirement) != nil {
		return errBadMinimumHive
	}
	match := minimumHive.FindStringSubmatch(strings.TrimSpace(requirement))
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
		minimum := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(requirement), ">="))
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

func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// ignoredKeys lists the keys Applet has no field for, at any depth, read from the struct's
// own tags so a new field needs no change here. The decode that follows skips them.
func ignoredKeys(doc *yaml.Node) []string {
	seen := map[string]bool{}
	collectIgnored(doc, reflect.TypeFor[Applet](), "", seen)
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	warnings := make([]string, 0, len(keys))
	for _, key := range keys {
		warnings = append(warnings, fmt.Sprintf("this version of Hive ignores %q", key))
	}
	return warnings
}

func collectIgnored(node *yaml.Node, t reflect.Type, path string, seen map[string]bool) {
	for node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		node = node.Content[0]
	}
	if node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() == reflect.Slice {
		if node.Kind == yaml.SequenceNode {
			for _, item := range node.Content {
				collectIgnored(item, t.Elem(), path, seen)
			}
		}
		return
	}
	if t.Kind() != reflect.Struct || node.Kind != yaml.MappingNode {
		return
	}
	fields := yamlFields(t)
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		if key == "<<" {
			continue
		}
		if field, known := fields[key]; known {
			collectIgnored(node.Content[i+1], field, path+key+".", seen)
			continue
		}
		if !plainValue.MatchString(key) {
			key = "a key with an unusual name"
		}
		seen[path+key] = true
	}
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
