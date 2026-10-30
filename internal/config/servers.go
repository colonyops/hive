package config

import (
	"errors"
	"fmt"

	"github.com/hay-kot/criterio"

	"github.com/colonyops/hive/internal/domain/server"
	"github.com/colonyops/hive/internal/domain/session"
)

// ServerConfig names a remote hive server whose daemon a client reaches over
// ssh.
type ServerConfig struct {
	Name      string `json:"name"       yaml:"name"`
	SSH       string `json:"ssh"        yaml:"ssh"`
	SSHConfig string `json:"ssh_config" yaml:"ssh_config"`
	Binary    string `json:"binary"     yaml:"binary"`
}

// Host is the ssh target for s.
func (s ServerConfig) Host() server.Host {
	return server.Host{Name: s.SSH, ConfigFile: s.SSHConfig, Binary: s.Binary}
}

func (c *Config) validateServers() error {
	var errs criterio.FieldErrorsBuilder
	seen := map[string]bool{}
	for i, server := range c.Servers {
		field := fmt.Sprintf("servers[%d]", i)
		switch {
		case server.Name == "":
			errs = errs.Append(field+".name", errors.New("required"))
		case seen[server.Name]:
			errs = errs.Append(field+".name", fmt.Errorf("duplicate server name %q", server.Name))
		default:
			if err := session.ValidateServerName(server.Name); err != nil {
				errs = errs.Append(field+".name", err)
			}
		}
		seen[server.Name] = true
		if err := server.Host().Validate(); err != nil {
			errs = errs.Append(field+".ssh", err)
		}
	}
	return errs.ToError()
}
