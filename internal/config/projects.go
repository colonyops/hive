package config

import (
	"fmt"
	"strings"

	"github.com/hay-kot/criterio"
)

// ProjectConfig defines a project: the repositories a project session checks
// out side by side, and the instructions its root AGENTS.md carries.
type ProjectConfig struct {
	Name         string              `json:"name"         yaml:"name"`
	Instructions string              `json:"instructions" yaml:"instructions"`
	Agent        string              `json:"agent"        yaml:"agent"`
	Repos        []ProjectRepoConfig `json:"repos"        yaml:"repos"`
}

// ProjectRepoConfig is one repository of a project. Dir defaults to the
// repository name.
type ProjectRepoConfig struct {
	Remote string `json:"remote" yaml:"remote"`
	Dir    string `json:"dir"    yaml:"dir"`
}

// Project returns the project definition named name.
func (c *Config) Project(name string) (ProjectConfig, bool) {
	for _, p := range c.Projects {
		if strings.EqualFold(p.Name, name) {
			return p, true
		}
	}
	return ProjectConfig{}, false
}

func (c *Config) validateProjects() error {
	var errs criterio.FieldErrorsBuilder
	seen := make(map[string]bool, len(c.Projects))
	for i, p := range c.Projects {
		field := fmt.Sprintf("projects[%d]", i)
		key := strings.ToLower(strings.TrimSpace(p.Name))
		switch {
		case key == "":
			errs = errs.Append(field+".name", fmt.Errorf("is required"))
		case seen[key]:
			errs = errs.Append(field+".name", fmt.Errorf("duplicate project %q", p.Name))
		}
		seen[key] = true
		if len(p.Repos) == 0 {
			errs = errs.Append(field+".repos", fmt.Errorf("needs at least one repo"))
		}
		if p.Agent != "" {
			if _, ok := c.Agents.Profiles[p.Agent]; !ok {
				errs = errs.Append(field+".agent", fmt.Errorf("profile %q not found in agents config", p.Agent))
			}
		}
		for j, r := range p.Repos {
			if strings.TrimSpace(r.Remote) == "" {
				errs = errs.Append(fmt.Sprintf("%s.repos[%d].remote", field, j), fmt.Errorf("is required"))
			}
		}
	}
	return errs.ToError()
}
