// Package ops is the operation catalog. A Spec declares one operation once:
// its name, owner, effect, toolset, typed input, constraints and guidance.
// The desktop mounts each Spec as an MCP tool, and the hive CLI builds a
// command from the same Spec, so the two surfaces cannot drift apart.
package ops

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

// Owner says where an operation runs.
type Owner string

const (
	// OwnerEngine operations run on hive.Engine.
	OwnerEngine Owner = "engine"
	// OwnerDesktop operations need the running desktop; the CLI reaches them
	// over its MCP servers.
	OwnerDesktop Owner = "desktop"
)

// Effect classifies what a call changes.
type Effect string

const (
	EffectRead  Effect = "read"
	EffectWrite Effect = "write"
	// EffectExec runs a command the caller supplies.
	EffectExec Effect = "exec"
)

// Toolset groups operations. MCPPath is the loopback path of the desktop MCP
// server that serves them. A toolset is metadata: workspace grants stay per
// server (ADR a-workspace-declares-its-own-authority).
type Toolset struct {
	Name    string
	MCPPath string
}

// CallerField is the json name of the input field that says who is calling.
// The CLI fills it from --caller and gives it no flag of its own.
const CallerField = "caller"

// Verbs is the closed verb vocabulary. put creates or replaces.
var Verbs = []string{
	"list", "get", "create", "put", "update", "delete", "clear",
	"open", "close", "move", "focus", "resize", "turn", "preview", "refresh",
}

// Spec declares one operation with input type In. The output type is not part
// of the Spec: it belongs to the side that binds the handler.
//
// An In may implement Validator for rules a JSON schema cannot state.
type Spec[In any] struct {
	// Name is "<resource>.<verb>". The resource may contain "_", which the
	// CLI spells as "-".
	Name    string
	Owner   Owner
	Effect  Effect
	Toolset Toolset
	// Summary is one line with no final period: the MCP title and the CLI
	// usage.
	Summary string
	// Guidance is the behavior an agent or a person must know: the rest of
	// the MCP description and the CLI long help.
	Guidance string
	// Args are the json names of the fields the CLI takes as positional
	// arguments, in order.
	Args []string
	// Fields adds constraints to input fields, keyed by json name.
	Fields Constraints
}

// Validator is implemented by an In that has cross-field rules. Mount calls it
// before the handler, and the CLI calls it before it dials the desktop.
type Validator interface{ Validate() error }

// Info is a Spec with its type parameter erased.
type Info struct {
	Name     string
	Resource string
	Verb     string
	// ToolName is the MCP tool name: "<resource>_<verb>".
	ToolName string
	Owner    Owner
	Effect   Effect
	Toolset  Toolset
	Summary  string
	Guidance string
	Args     []string
	Fields   Constraints
}

// Description is the MCP tool description: the summary as a sentence, then
// the guidance.
func (i Info) Description() string {
	if i.Guidance == "" {
		return i.Summary + "."
	}
	return i.Summary + ". " + i.Guidance
}

// Op is a Spec seen without its type parameter, so a catalog can hold
// operations of different input types.
type Op interface {
	Info() Info
	InputType() reflect.Type
	// InputSchema is the inferred schema of In with Fields applied. A Fields
	// key that names no input field is an error.
	InputSchema() (*jsonschema.Schema, error)
}

// Info panics on a malformed Name or a verb outside Verbs: both are
// programming errors that the catalog test catches first.
func (s Spec[In]) Info() Info {
	i := strings.LastIndex(s.Name, ".")
	if i <= 0 || i == len(s.Name)-1 {
		panic(fmt.Sprintf("ops: name %q is not <resource>.<verb>", s.Name))
	}
	resource, verb := s.Name[:i], s.Name[i+1:]
	if !slices.Contains(Verbs, verb) {
		panic(fmt.Sprintf("ops: %s: verb %q is not in ops.Verbs", s.Name, verb))
	}
	return Info{
		Name: s.Name, Resource: resource, Verb: verb, ToolName: resource + "_" + verb,
		Owner: s.Owner, Effect: s.Effect, Toolset: s.Toolset,
		Summary: s.Summary, Guidance: s.Guidance, Args: s.Args, Fields: s.Fields,
	}
}

func (Spec[In]) InputType() reflect.Type { return reflect.TypeFor[In]() }

func (s Spec[In]) InputSchema() (*jsonschema.Schema, error) {
	return inputSchema(reflect.TypeFor[In](), s.Fields)
}
