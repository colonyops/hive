package install

import (
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/tui/components/form"
	"gopkg.in/yaml.v3"
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
	Cancelled      bool
}

// Model is the Bubble Tea model for the install wizard.
type Model struct {
	width, height  int
	dialog         *form.Dialog
	shellField     *form.MultiSelectField // stored to access SelectedIndices
	detectedShells []ShellConfig
	configPath     string
	configExists   bool
	homeDir        string
	result         Result
	quitting       bool
}

// existingConfig holds minimal config structure for reading workspaces.
type existingConfig struct {
	Workspaces []string `yaml:"workspaces"`
}

// New creates a new install wizard model.
func New() Model {
	homeDir, _ := os.UserHomeDir()

	// Find existing config
	configPath, configExists := findConfigPath(homeDir)

	// Load existing workspaces if config exists
	var existingWorkspaces []string
	if configExists {
		existingWorkspaces = loadExistingWorkspaces(configPath)
	}

	// Build default value for workspaces field
	var defaultWorkspaces string
	if len(existingWorkspaces) > 0 {
		// Show existing workspaces
		for i, ws := range existingWorkspaces {
			existingWorkspaces[i] = shortenPath(ws, homeDir)
		}
		defaultWorkspaces = strings.Join(existingWorkspaces, "\n")
	} else {
		defaultWorkspaces = "~/code"
	}

	// Detect shell configs
	detectedShells := detectShellConfigs()
	shellOptions := make([]string, len(detectedShells))
	for i, s := range detectedShells {
		shellOptions[i] = s.Name + " (" + shortenPath(s.Path, homeDir) + ")"
	}

	// Build form fields
	var fields []form.Field
	var variables []string

	// Workspaces field - textarea for multiple paths
	workspacesField := form.NewTextAreaField(
		"Workspace Directories (one per line)",
		"~/code\n~/projects",
		defaultWorkspaces,
	)
	fields = append(fields, workspacesField)
	variables = append(variables, "workspaces")

	// Shell aliases field - pre-select all by default
	var shellField *form.MultiSelectField
	if len(shellOptions) > 0 {
		shellField = form.NewMultiSelectFormField("Install 'hv' alias to:", shellOptions)
		shellField.SelectAll()
		fields = append(fields, shellField)
		variables = append(variables, "shells")
	}

	dialog := form.NewDialog("", fields, variables)

	return Model{
		dialog:         dialog,
		shellField:     shellField,
		detectedShells: detectedShells,
		configPath:     configPath,
		configExists:   configExists,
		homeDir:        homeDir,
	}
}

// findConfigPath checks HIVE_CONFIG env var, then probes default locations.
// Returns the config path and whether it exists.
func findConfigPath(homeDir string) (string, bool) {
	// Check env var first
	if envPath := os.Getenv("HIVE_CONFIG"); envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			return envPath, true
		}
		// Env var set but file doesn't exist - use it as the path to create
		return envPath, false
	}

	// Check default locations
	configDir := filepath.Join(homeDir, ".config", "hive")
	configNames := []string{"config.yaml", "config.yml", "hive.yaml", "hive.yml"}

	for _, name := range configNames {
		path := filepath.Join(configDir, name)
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}

	// No config found - return default path for creation
	return filepath.Join(configDir, "config.yaml"), false
}

// loadExistingWorkspaces reads workspaces from an existing config file.
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

// detectShellConfigs finds shell configuration files.
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

// shortenPath replaces home directory with ~
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
		switch msg.String() {
		case "ctrl+c":
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

	// Parse workspaces from textarea (one per line)
	var workspaces []string
	if ws, ok := values["workspaces"].(string); ok {
		for _, line := range strings.Split(ws, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				// Expand ~ to home directory for storage
				if strings.HasPrefix(line, "~/") {
					line = filepath.Join(m.homeDir, line[2:])
				} else if line == "~" {
					line = m.homeDir
				}
				workspaces = append(workspaces, line)
			}
		}
	}

	// Get selected shells by index
	var selectedShells []ShellConfig
	if m.shellField != nil {
		for _, idx := range m.shellField.SelectedIndices() {
			if idx < len(m.detectedShells) {
				selectedShells = append(selectedShells, m.detectedShells[idx])
			}
		}
	}

	return Result{
		Workspaces:     workspaces,
		ConfigPath:     m.configPath,
		SelectedShells: selectedShells,
	}
}

func (m Model) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	title := styles.TextPrimaryBoldStyle.Render("Hive Setup")

	// Config info message
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
