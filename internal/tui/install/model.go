package install

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"gopkg.in/yaml.v3"

	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/tui/components/form"
)

// ShellConfig represents a detected shell configuration file.
type ShellConfig struct {
	Name string // e.g., "zsh", "bash", "fish"
	Path string // absolute path to config file
}

// Result contains the wizard outputs.
type Result struct {
	Workspaces     []string
	ConfigPath     string
	SelectedShells []ShellConfig
	SkillAgents    []string // agent names selected for skills installation
	Cancelled      bool
}

// Model is the Bubble Tea model for the install wizard.
type Model struct {
	width, height    int
	dialog           *form.Dialog
	shellField       *form.MultiSelectField
	skillAgentsField *form.MultiSelectField
	detectedShells   []ShellConfig
	configPath       string
	configExists     bool
	homeDir          string
	result           Result
	quitting         bool
}

// existingConfig holds minimal config structure for reading workspaces.
type existingConfig struct {
	Workspaces []string `yaml:"workspaces"`
}

// New creates a new install wizard model.
func New() Model {
	homeDir, _ := os.UserHomeDir()

	configPath, configExists := findConfigPath(homeDir)

	var existingWorkspaces []string
	if configExists {
		existingWorkspaces = loadExistingWorkspaces(configPath)
	}

	var defaultWorkspaces string
	if len(existingWorkspaces) > 0 {
		for i, ws := range existingWorkspaces {
			existingWorkspaces[i] = shortenPath(ws, homeDir)
		}
		defaultWorkspaces = strings.Join(existingWorkspaces, "\n")
	} else {
		defaultWorkspaces = "~/code"
	}

	detectedShells := detectShellConfigs()
	shellOptions := make([]string, len(detectedShells))
	for i, s := range detectedShells {
		shellOptions[i] = s.Name + " (" + shortenPath(s.Path, homeDir) + ")"
	}

	var fields []form.Field
	var variables []string

	workspacesField := form.NewTextAreaField(
		"Workspace Directories (one per line)",
		"~/code\n~/projects",
		defaultWorkspaces,
	)
	fields = append(fields, workspacesField)
	variables = append(variables, "workspaces")

	var shellField *form.MultiSelectField
	if len(shellOptions) > 0 {
		shellField = form.NewMultiSelectFormField("Install 'hv' alias to:", shellOptions)
		// Pre-select all shells by default
		for i := range detectedShells {
			shellField.Check(i)
		}
		fields = append(fields, shellField)
		variables = append(variables, "shells")
	}

	skillAgentOptions := []string{
		"Claude Code (~/.claude)",
		"Codex (~/.codex)",
	}
	skillAgentsField := form.NewMultiSelectFormField("Install AI skills for:", skillAgentOptions)
	// Pre-check agents that are available in PATH.
	if _, err := exec.LookPath("claude"); err == nil {
		skillAgentsField.Check(0)
	}
	if _, err := exec.LookPath("codex"); err == nil {
		skillAgentsField.Check(1)
	}
	fields = append(fields, skillAgentsField)
	variables = append(variables, "skillAgents")

	dialog := form.NewDialog("", fields, variables)

	return Model{
		dialog:           dialog,
		shellField:       shellField,
		skillAgentsField: skillAgentsField,
		detectedShells:   detectedShells,
		configPath:       configPath,
		configExists:     configExists,
		homeDir:          homeDir,
	}
}

func findConfigPath(homeDir string) (string, bool) {
	// If HIVE_CONFIG is explicitly set, use it — but only if it points inside
	// the user's home directory. System paths like /etc/hive/config.yaml are
	// runtime defaults shipped with the software, not the user's config.
	if envPath := os.Getenv("HIVE_CONFIG"); envPath != "" && strings.HasPrefix(envPath, homeDir) {
		_, err := os.Stat(envPath)
		return envPath, err == nil
	}

	configDir := filepath.Join(homeDir, ".config", "hive")
	configNames := []string{"config.yaml", "config.yml", "hive.yaml", "hive.yml"}

	for _, name := range configNames {
		path := filepath.Join(configDir, name)
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}

	return filepath.Join(configDir, "config.yaml"), false
}

func loadExistingWorkspaces(configPath string) []string {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}

	var cfg existingConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil
	}

	return cfg.Workspaces
}

func detectShellConfigs() []ShellConfig {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	candidates := []struct {
		name string
		path string
	}{
		{"zsh", filepath.Join(homeDir, ".zshrc")},
		{"bash", filepath.Join(homeDir, ".bashrc")},
		{"bash_profile", filepath.Join(homeDir, ".bash_profile")},
		{"fish", filepath.Join(homeDir, ".config", "fish", "config.fish")},
	}

	var found []ShellConfig
	for _, c := range candidates {
		if _, err := os.Stat(c.path); err == nil {
			found = append(found, ShellConfig{Name: c.name, Path: c.path})
		}
	}
	return found
}

func shortenPath(path, homeDir string) string {
	if strings.HasPrefix(path, homeDir) {
		return "~" + path[len(homeDir):]
	}
	return path
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			m.quitting = true
			m.result.Cancelled = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.dialog, cmd = m.dialog.Update(msg)

	if m.dialog.Submitted() {
		m.result = m.buildResult()
		return m, tea.Quit
	}

	if m.dialog.Cancelled() {
		m.quitting = true
		m.result.Cancelled = true
		return m, tea.Quit
	}

	return m, cmd
}

func (m Model) buildResult() Result {
	values := m.dialog.FormValues()

	var workspaces []string
	if ws, ok := values["workspaces"].(string); ok {
		for _, line := range strings.Split(ws, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "~/") {
				line = filepath.Join(m.homeDir, line[2:])
			} else if line == "~" {
				line = m.homeDir
			}
			workspaces = append(workspaces, line)
		}
	}

	var selectedShells []ShellConfig
	if m.shellField != nil {
		for _, idx := range m.shellField.SelectedIndices() {
			if idx < len(m.detectedShells) {
				selectedShells = append(selectedShells, m.detectedShells[idx])
			}
		}
	}

	agentNames := []string{"claude", "codex"}
	var skillAgents []string
	for _, idx := range m.skillAgentsField.SelectedIndices() {
		if idx < len(agentNames) {
			skillAgents = append(skillAgents, agentNames[idx])
		}
	}

	return Result{
		Workspaces:     workspaces,
		ConfigPath:     m.configPath,
		SelectedShells: selectedShells,
		SkillAgents:    skillAgents,
	}
}

func (m Model) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	title := styles.TextPrimaryBoldStyle.Render("Hive Setup")

	var configInfo string
	if m.configExists {
		configInfo = styles.TextSuccessStyle.Render("Found config: ") +
			styles.TextMutedStyle.Render(shortenPath(m.configPath, m.homeDir))
	} else {
		configInfo = styles.TextMutedStyle.Render("Config will be created at: " +
			shortenPath(m.configPath, m.homeDir))
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		configInfo,
		"",
		m.dialog.View(),
	)

	return tea.NewView(content)
}

// Result returns the wizard result. Call after the program exits.
func (m Model) Result() Result {
	return m.result
}
