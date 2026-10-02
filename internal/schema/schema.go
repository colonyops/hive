// Package schema embeds the JSON Schemas and the catalog that
// `mise run generate:schemas` exports from the CUE sources beside it, and
// validates instances against them. Every surface that needs to know the
// shape of a hive file (the CLI, doctor, the desktop) reads from here; cue
// itself runs only at generate time.
package schema

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/hay-kot/criterio"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

//go:embed catalog.json */*.schema.json
var files embed.FS

// Entry describes one published schema.
type Entry struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	// Doc is the documenting page's path on hivedesktop.com.
	Doc string `json:"doc"`
}

const (
	siteBase      = "https://hivedesktop.com"
	publishedBase = siteBase + "/schemas/"
)

var (
	catalogOnce sync.Once
	catalog     []Entry
	compiled    sync.Map // id -> *jsonschema.Schema
	printer     = message.NewPrinter(language.English)
)

// Catalog lists every schema in catalog order.
func Catalog() []Entry {
	catalogOnce.Do(func() {
		data, err := files.ReadFile("catalog.json")
		if err != nil {
			panic("schema: catalog.json is not embedded: " + err.Error())
		}
		if err := json.Unmarshal(data, &catalog); err != nil {
			panic("schema: catalog.json: " + err.Error())
		}
	})
	return append([]Entry(nil), catalog...)
}

// Lookup finds a catalog entry by id.
func Lookup(id string) (Entry, bool) {
	for _, e := range Catalog() {
		if e.ID == id {
			return e, true
		}
	}
	return Entry{}, false
}

// JSON returns the embedded JSON Schema for id.
func JSON(id string) ([]byte, error) {
	if _, ok := Lookup(id); !ok {
		return nil, fmt.Errorf("schema: unknown id %q", id)
	}
	// Dir is generator configuration and is not in the runtime catalog, so the
	// file is located by its unique name instead.
	matches, err := fsGlob(id + ".schema.json")
	if err != nil {
		return nil, err
	}
	return files.ReadFile(matches)
}

// PublishedURL is where the site serves the schema for id, and what a
// `# yaml-language-server: $schema=` modeline should name.
func PublishedURL(id string) string {
	return publishedBase + id + ".schema.json"
}

// DocURL is the full URL of the page that documents an entry's format.
func DocURL(e Entry) string {
	return siteBase + e.Doc
}

// Normalize converts a value decoded by yaml.v3 or encoding/json into the
// JSON value shape Validate accepts: string keys, float64 numbers, []any.
func Normalize(v any) (any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("schema: normalize: %w", err)
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("schema: normalize: %w", err)
	}
	return out, nil
}

// Validate checks instance, a JSON value as Normalize produces, against the
// schema for id. It returns nil or a criterio.FieldErrors with one entry per
// failure. Field uses the criterio path form: "children[1].blockers[0]",
// "tui.theem" for an unknown key, "" for the document root.
func Validate(id string, instance any) error {
	sch, err := compiledSchema(id)
	if err != nil {
		return err
	}
	err = sch.Validate(instance)
	if err == nil {
		return nil
	}
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return fmt.Errorf("schema %s: %w", id, err)
	}
	var errs criterio.FieldErrorsBuilder
	collect(ve, &errs)
	return errs.ToError()
}

func collect(ve *jsonschema.ValidationError, errs *criterio.FieldErrorsBuilder) {
	if len(ve.Causes) > 0 {
		for _, c := range ve.Causes {
			collect(c, errs)
		}
		return
	}
	path := fieldPath(ve.InstanceLocation)
	if ap, ok := ve.ErrorKind.(*kind.AdditionalProperties); ok {
		for _, p := range ap.Properties {
			*errs = errs.Append(join(path, p), errors.New("unknown key"))
		}
		return
	}
	*errs = errs.Append(path, errors.New(ve.ErrorKind.LocalizedString(printer)))
}

// fieldPath renders a JSON pointer's segments in criterio form: object keys
// joined with dots, list indices in brackets.
func fieldPath(segments []string) string {
	var b strings.Builder
	for _, s := range segments {
		if isIndex(s) {
			b.WriteString("[" + s + "]")
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(s)
	}
	return b.String()
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func isIndex(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func compiledSchema(id string) (*jsonschema.Schema, error) {
	if v, ok := compiled.Load(id); ok {
		return v.(*jsonschema.Schema), nil
	}
	raw, err := JSON(id)
	if err != nil {
		return nil, err
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("schema %s: %w", id, err)
	}
	name := id + ".schema.json"
	c := jsonschema.NewCompiler()
	if err := c.AddResource(name, doc); err != nil {
		return nil, fmt.Errorf("schema %s: %w", id, err)
	}
	sch, err := c.Compile(name)
	if err != nil {
		return nil, fmt.Errorf("schema %s: %w", id, err)
	}
	v, _ := compiled.LoadOrStore(id, sch)
	return v.(*jsonschema.Schema), nil
}

func fsGlob(name string) (string, error) {
	dirs, err := files.ReadDir(".")
	if err != nil {
		return "", err
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		candidate := d.Name() + "/" + name
		if _, err := files.Open(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("schema: %s is not embedded; run `mise run generate`", name)
}
