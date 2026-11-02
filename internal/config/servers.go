package config

import (
	"errors"
	"fmt"
	"time"

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
	Tmux      string `json:"tmux"       yaml:"tmux"`
	// ReadTimeout and ConnectTimeout replace the limits on read-only calls, for
	// a link that needs more time than the defaults in internal/hive/servers.
	// Zero keeps the default. Hive Desktop also bounds its session lists itself
	// (serverReadTimeout), so a longer limit does not lengthen those.
	ReadTimeout    time.Duration `json:"read_timeout"    yaml:"read_timeout"`
	ConnectTimeout time.Duration `json:"connect_timeout" yaml:"connect_timeout"`
}

// Host is the ssh target for s.
func (s ServerConfig) Host() server.Host {
	return server.Host{Name: s.SSH, ConfigFile: s.SSHConfig, Binary: s.Binary, Tmux: s.Tmux}
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
		if server.ReadTimeout < 0 {
			errs = errs.Append(field+".read_timeout", errors.New("must not be negative"))
		}
		if server.ConnectTimeout < 0 {
			errs = errs.Append(field+".connect_timeout", errors.New("must not be negative"))
		}
	}
	return errs.ToError()
}
