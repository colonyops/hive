package app

import (
	"context"
	"os"
	"path/filepath"
	"reflect"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/drive-server/internal/config"
	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/internal/platform/dirwatch"
	"github.com/colonyops/hive/pkg/logutils"
)

// LoadConfig resolves and validates the config with the inputs used at
// startup, so a reload sees the same flags and env.
type LoadConfig func() (cfg config.Config, warnings []string, err error)

// watchConfig swaps in new user_policies when drive-server.yaml changes. It
// does not create the config directory: that fails when the server runs as a
// user who cannot write its parent.
func watchConfig(ctx context.Context, log zerolog.Logger, running config.Config, policies *core.Policies, load LoadConfig) error {
	log = logutils.Component(log, "reload")
	if _, err := os.Stat(running.ConfigDir); err != nil {
		log.Info().Str("dir", running.ConfigDir).Msg("config directory missing; policies will not reload")
		<-ctx.Done()
		return nil
	}

	w, err := dirwatch.New(running.ConfigDir, isConfigFile, func() { reloadPolicies(log, running, policies, load) }, log, dirwatch.WithComponent("config-watcher"))
	if err != nil {
		return err
	}
	w.Start()
	<-ctx.Done()
	w.Close()
	return nil
}

func isConfigFile(path string) bool {
	return filepath.Base(path) == config.ConfigFileName
}

func reloadPolicies(log zerolog.Logger, running config.Config, policies *core.Policies, load LoadConfig) {
	next, warnings, err := load()
	if err != nil {
		log.Warn().Err(err).Msg("config reload rejected; keeping the running policies")
		return
	}
	policies.Swap(next.UserPolicies)
	for _, w := range warnings {
		log.Warn().Msg(w)
	}
	log.Info().Int("policies", len(next.UserPolicies)).Msg("user policies reloaded")

	for _, field := range restartFields(running, next) {
		log.Warn().Str("field", field).Msg("restart required")
	}
}

func restartFields(running, next config.Config) []string {
	checks := []struct {
		name string
		a, b any
	}{
		{"base_url", running.BaseURL, next.BaseURL},
		{"listen", running.Listen, next.Listen},
		{"data_dir", running.DataDir, next.DataDir},
		{"config_dir", running.ConfigDir, next.ConfigDir},
		{"max_document_size", running.MaxDocumentSize, next.MaxDocumentSize},
		{"smtp", running.SMTP, next.SMTP},
		{"dev_log_mail", running.DevLogMail, next.DevLogMail},
		{"trusted_proxies", running.TrustedProxies, next.TrustedProxies},
	}
	var changed []string
	for _, c := range checks {
		if !reflect.DeepEqual(c.a, c.b) {
			changed = append(changed, c.name)
		}
	}
	return changed
}
