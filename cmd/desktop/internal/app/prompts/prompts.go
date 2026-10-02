// Package prompts owns every paste-ready LLM prompt Hive Desktop offers, and
// the templates they are composed from.
//
// The app's configuration surfaces — flows/*.yaml, actions.yml, settings.yaml —
// are things a user is expected to hand to a coding agent rather than
// hand-write, so each one ships a prompt that is self-contained: an agent given
// only that text has the schema, the rules, a worked example, and this
// install's real file paths. The "LLM prompts" settings section lists all of
// them in one place, and a few context-scoped surfaces (a webhook node's
// transform prompt) render from the same registry with instance data mixed in.
//
// Two rules keep the collection maintainable:
//
//   - Prompt text lives in templates/, never in a Go string literal or a
//     frontend component. Shared wording (what the app is, strict-YAML rules,
//     Go template data, the closing task line) lives in fragments.tmpl and is
//     composed via {{template}}, so a change to how flows hot-reload is
//     written once.
//   - What a prompt says about a *type* comes from that type's own
//     documentation — flow.NodeDoc for node types, actions.Doc for action
//     types — never from prose kept here. Adding a node or action type extends
//     the relevant prompt with no edit to this package.
//
// Adding a prompt: drop a template in templates/, add one entry to
// definitions. The settings page renders whatever the registry reports.
package prompts

import (
	"embed"
	"fmt"
	"maps"
	"sort"
	"strings"
	"text/template"

	hivetmpl "github.com/colonyops/hive/pkg/tmpl"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/agentws"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
	"github.com/colonyops/hive/cmd/desktop/internal/app/mcpcatalog"
)

//go:embed templates/*.tmpl
var templatesFS embed.FS

// Env is the install-specific context prompts are rendered against: the real
// paths and URLs on *this* machine, so a copied prompt names the file the
// agent should actually edit instead of a placeholder. The caller supplies it
// (see desktop/promptsservice.go) — this package deliberately does not import
// cmd/desktop/internal/app/settings, so it stays testable without touching the user's config.
type Env struct {
	// FlowsDir is where flows/<id>.yaml documents live.
	FlowsDir string
	// ActionsPath is the actions.yml location.
	ActionsPath string
	// SettingsPath is the settings.yaml location.
	SettingsPath string
	// WebhookBaseURL is the local listener's base URL, already including the
	// /hooks path prefix. Empty when the listener has no resolved port.
	WebhookBaseURL string
	// WebhookEnabled reports whether the listener is configured on.
	WebhookEnabled bool
	// MCPEndpoint is the agent MCP server's URL — the same loopback server as
	// the webhook listener, at the /mcp path. Empty when no port is resolved.
	MCPEndpoint string
	// MCPEnabled reports whether the loopback HTTP server is on. It is the same
	// http.enabled flag as the webhook listener (one server, ADR agent-http-api).
	MCPEnabled bool
	// AgentWorkspacesDir is the agent-workspace root: mcps.yaml, .shared/, and
	// one directory per workspace live under it.
	AgentWorkspacesDir string
}

// Input carries the caller-supplied context a prompt needs beyond Env. Every
// field is optional; a prompt that needs one documents it on its definition.
type Input struct {
	// WebhookPath and WebhookSample scope the webhook transform prompt to one
	// sources.webhook node: its configured path and its last captured delivery.
	WebhookPath   string `json:"webhookPath"`
	WebhookSample string `json:"webhookSample"`
}

// Prompt is one rendered, copyable prompt.
type Prompt struct {
	ID string `json:"id"`
	// Title and Description label the entry in the settings catalog.
	Title       string `json:"title"`
	Description string `json:"description"`
	// Target names what the prompt configures — a path for file-backed
	// prompts, a surface name otherwise — shown under the title.
	Target string `json:"target"`
	// Text is the prompt itself: what the copy button puts on the clipboard.
	Text string `json:"text"`
}

// definition is one registry entry. data assembles the template's context;
// returning an error rejects the render.
type definition struct {
	id          string
	title       string
	description string
	// target renders the Target field from the environment, so file-backed
	// prompts can point at this install's real path.
	target func(Env) string
	// listed reports whether this prompt appears in the settings catalog.
	// Context-scoped prompts (the webhook transform) need instance data that
	// only their own editor has, so they are rendered on demand instead.
	listed bool
	data   func(Env, Input) (map[string]any, error)
}

// definitions is the prompt registry, in catalog order.
var definitions = []definition{
	{
		id:          "flows",
		title:       "Flows",
		description: "Author or edit a pipeline flow: the graph of sources, filters, and destinations that decides what reaches your feeds and what fires an action.",
		target:      func(env Env) string { return env.FlowsDir + "/<id>.yaml" },
		listed:      true,
		data:        flowsData,
	},
	{
		id:          "actions",
		title:       "Actions",
		description: "Define the actions a feed item, a terminal session or window, or a flow node can trigger — launching an agent session, running a shell command, publishing a message, or copying text to the clipboard.",
		target:      func(env Env) string { return env.ActionsPath },
		listed:      true,
		data:        actionsData,
	},
	{
		id:          "webhook-sources",
		title:       "Webhook sources",
		description: "Wire an external system into Hive by POSTing JSON to this install's local webhook endpoint, and shape the payload so it renders well in a feed.",
		target:      func(env Env) string { return env.WebhookBaseURL },
		listed:      true,
		data:        webhookSourcesData,
	},
	{
		id:          "mcp",
		title:       "Agent MCP server",
		description: "Point a coding agent at this install's live MCP server — the tools it calls to drive the app directly, without editing config files.",
		target:      func(env Env) string { return env.MCPEndpoint },
		listed:      true,
		data:        mcpData,
	},
	{
		id:          "settings",
		title:       "App settings",
		description: "Tune polling, updates, notifications, appearance, and the webhook listener in settings.yaml.",
		target:      func(env Env) string { return env.SettingsPath },
		listed:      true,
		data:        settingsData,
	},
	{
		id:          "webhook-transform",
		title:       "Webhook payload transform",
		description: "Write the function node body that reshapes one webhook endpoint's payloads into the canonical feed item contract.",
		target:      func(Env) string { return "function node" },
		listed:      false,
		data:        webhookTransformData,
	},
	{
		id:          "agent-workspaces",
		title:       "Agent workspaces",
		description: "Author or edit an agent workspace: a named, durable directory where a CLI agent runs against a purpose-built MCP tool set, for work that has no repository.",
		target:      func(env Env) string { return env.AgentWorkspacesDir },
		listed:      true,
		data:        agentWorkspacesData,
	},
}

// Service renders prompts against one install's environment.
type Service struct {
	env       Env
	templates *template.Template
}

// frames are the templates rendered at runtime around text the app is about to
// hand an agent, as opposed to the copyable prompts the registry lists.
var frames = template.Must(template.New("frames").Funcs(funcs()).ParseFS(templatesFS, "templates/scheduled-run.tmpl", "templates/first-run.tmpl"))

type FirstRunData struct {
	// DefaultProfile is the name of the profile the app created, so the
	// interview builds on it instead of leaving it beside what it makes.
	DefaultProfile string
}

// FirstRun is the opening message of the chat first run hands off into: an
// interview about how the person works, then a proposal for their profiles
// and feeds.
func FirstRun(data FirstRunData) (string, error) {
	var buf strings.Builder
	if err := frames.ExecuteTemplate(&buf, "first-run.tmpl", data); err != nil {
		return "", fmt.Errorf("prompts: rendering the first-run frame: %w", err)
	}
	return buf.String(), nil
}

type ScheduledRunData struct {
	ScheduleName  string
	WorkspaceName string
	Prompt        string
	// CanEnd is false when the launch handed the process no end-session URL;
	// the closing instruction is then left out rather than pointing at nothing.
	CanEnd bool
}

// ScheduledRun frames a scheduled chat's rendered prompt as its opening message.
func ScheduledRun(data ScheduledRunData) (string, error) {
	var buf strings.Builder
	if err := frames.ExecuteTemplate(&buf, "scheduled-run.tmpl", data); err != nil {
		return "", fmt.Errorf("prompts: rendering the scheduled-run frame: %w", err)
	}
	return buf.String(), nil
}

// New parses the embedded templates and binds them to env. It fails only on a
// malformed template, which is a programming error caught by this package's
// tests.
func New(env Env) (*Service, error) {
	tmpl, err := template.New("prompts").Funcs(funcs()).ParseFS(templatesFS, "templates/*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("prompts: parsing templates: %w", err)
	}
	return &Service{env: env, templates: tmpl}, nil
}

// Catalog renders every prompt that belongs in the settings listing, in
// registry order. A prompt that cannot render is omitted rather than failing
// the whole listing — one broken entry must not empty the page.
func (s *Service) Catalog(in Input) []Prompt {
	out := make([]Prompt, 0, len(definitions))
	for _, def := range definitions {
		if !def.listed {
			continue
		}
		prompt, err := s.render(def, in)
		if err != nil {
			continue
		}
		out = append(out, prompt)
	}
	return out
}

// Render returns one prompt by id, including the context-scoped prompts
// Catalog omits.
func (s *Service) Render(id string, in Input) (Prompt, error) {
	for _, def := range definitions {
		if def.id == id {
			return s.render(def, in)
		}
	}
	return Prompt{}, fmt.Errorf("prompts: unknown prompt %q", id)
}

// IDs returns every registered prompt id, sorted. Tests use it to assert the
// registry and templates stay in step.
func IDs() []string {
	ids := make([]string, 0, len(definitions))
	for _, def := range definitions {
		ids = append(ids, def.id)
	}
	sort.Strings(ids)
	return ids
}

func (s *Service) render(def definition, in Input) (Prompt, error) {
	data, err := def.data(s.env, in)
	if err != nil {
		return Prompt{}, err
	}
	data["Env"] = s.env

	tmpl, err := s.templates.Clone()
	if err != nil {
		return Prompt{}, fmt.Errorf("prompts: cloning templates: %w", err)
	}
	tmpl.Funcs(hivetmpl.UntrustedFuncs())

	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, def.id+".tmpl", data); err != nil {
		return Prompt{}, fmt.Errorf("prompts: rendering %q: %w", def.id, err)
	}
	return Prompt{
		ID:          def.id,
		Title:       def.title,
		Description: def.description,
		Target:      def.target(s.env),
		Text:        strings.TrimSpace(buf.String()) + "\n",
	}, nil
}

// nodeTypeDoc pairs a registered node type with its documentation, for the
// flows template's range.
type nodeTypeDoc struct {
	Type string
	Doc  string
}

// flowsData assembles the flows prompt: every registered node type grouped by
// the category its port counts imply, plus the canonical worked example.
func flowsData(Env, Input) (map[string]any, error) {
	grouped := map[flow.NodeCategory][]nodeTypeDoc{}
	for _, nodeType := range flow.NodeTypes() {
		doc, err := flow.NodeDoc(nodeType)
		if err != nil {
			return nil, err
		}
		category, err := flow.CategoryOf(nodeType)
		if err != nil {
			return nil, err
		}
		grouped[category] = append(grouped[category], nodeTypeDoc{Type: nodeType, Doc: strings.TrimSpace(doc)})
	}

	type categoryGroup struct {
		Name  flow.NodeCategory
		Types []nodeTypeDoc
	}
	groups := make([]categoryGroup, 0, len(flow.CategoryOrder))
	for _, category := range flow.CategoryOrder {
		if types := grouped[category]; len(types) > 0 {
			groups = append(groups, categoryGroup{Name: category, Types: types})
		}
	}

	return map[string]any{
		"Groups":  groups,
		"Example": strings.TrimSpace(flow.WorkedExampleYAML),
	}, nil
}

// actionsData assembles the actions prompt from the action type registry.
func actionsData(Env, Input) (map[string]any, error) {
	docs := make([]nodeTypeDoc, 0, len(actions.Types()))
	for _, actionType := range actions.Types() {
		doc, err := actions.Doc(actionType)
		if err != nil {
			return nil, err
		}
		docs = append(docs, nodeTypeDoc{Type: actionType, Doc: strings.TrimSpace(doc)})
	}
	return map[string]any{
		"Types":       docs,
		"LauncherDoc": strings.TrimSpace(actions.LauncherDoc()),
		"Example":     strings.TrimSpace(actions.ExampleYAML()),
	}, nil
}

// agentWorkspacesData assembles the agent-workspaces prompt from the shipped
// MCP catalogue, the way actionsData assembles the actions prompt from the
// action type registry — so a new shipped MCP entry extends the prompt with
// no prose edit here.
func agentWorkspacesData(Env, Input) (map[string]any, error) {
	docs := make([]nodeTypeDoc, 0, len(mcpcatalog.Types()))
	for _, mcpType := range mcpcatalog.Types() {
		doc, err := mcpcatalog.Doc(mcpType)
		if err != nil {
			return nil, err
		}
		docs = append(docs, nodeTypeDoc{Type: mcpType, Doc: strings.TrimSpace(doc)})
	}
	return map[string]any{
		"Types":            docs,
		"WorkspaceExample": strings.TrimSpace(agentws.ExampleWorkspaceYAML()),
		"MCPsExample":      strings.TrimSpace(agentws.ExampleMCPsYAML()),
		"SkillsExample":    strings.TrimSpace(agentws.ExampleSkillsYAML()),
	}, nil
}

// webhookSourcesData assembles the webhook integration prompt. It reuses the
// sources.webhook node's own documentation rather than restating the delivery
// contract.
func webhookSourcesData(Env, Input) (map[string]any, error) {
	doc, err := flow.NodeDoc("sources.webhook")
	if err != nil {
		return nil, err
	}
	return map[string]any{"Doc": strings.TrimSpace(doc)}, nil
}

func settingsData(Env, Input) (map[string]any, error) {
	return map[string]any{}, nil
}

// mcpData needs nothing beyond Env — the prompt points at the live server,
// which describes its own tools, rather than restating them.
func mcpData(Env, Input) (map[string]any, error) {
	return map[string]any{}, nil
}

// webhookTransformData scopes the transform prompt to one sources.webhook node.
// A node with no captured delivery still gets a usable prompt — the sample
// becomes a placeholder the user pastes into.
func webhookTransformData(_ Env, in Input) (map[string]any, error) {
	path := strings.TrimSpace(in.WebhookPath)
	if path == "" {
		return nil, fmt.Errorf("prompts: the webhook transform prompt requires the node's endpoint path")
	}
	return map[string]any{
		"Path":   path,
		"Sample": strings.TrimSpace(in.WebhookSample),
	}, nil
}

func funcs() template.FuncMap {
	funcs := template.FuncMap{
		// section splices a type's own documentation into the prompt's outline
		// at the given heading level.
		"section": section,
	}
	maps.Copy(funcs, hivetmpl.UntrustedFuncs())
	return funcs
}

// maxHeadingLevel is markdown's deepest ATX heading.
const maxHeadingLevel = 6

// section rewrites a type's standalone documentation so it reads as one
// section of a larger prompt: the doc's H1 becomes a heading at level, tagged
// with the type key an author actually writes in YAML, and every heading below
// it shifts to match. Without this the docs' own H1/H2 would sit *under* the
// prompt's deeper headings and invert the outline.
//
// Headings inside fenced code blocks are left alone — a `#` starting a line of
// a shell snippet is a comment, not a heading.
func section(level int, typeKey, doc string) string {
	shift := level - 1
	lines := strings.Split(strings.TrimSpace(doc), "\n")
	fenced := false
	titled := false

	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}

		hashes := len(line) - len(strings.TrimLeft(line, "#"))
		if hashes == 0 || hashes > maxHeadingLevel || !strings.HasPrefix(line[hashes:], " ") {
			continue
		}

		if !titled && hashes == 1 {
			// The doc's own title, restated with the type key so an agent can
			// map the prose to the `type:` value it has to write.
			lines[i] = strings.Repeat("#", min(level, maxHeadingLevel)) +
				" " + strings.TrimSpace(line[1:]) + " — `" + typeKey + "`"
			titled = true
			continue
		}
		lines[i] = strings.Repeat("#", min(hashes+shift, maxHeadingLevel)) + line[hashes:]
	}
	return strings.Join(lines, "\n")
}
