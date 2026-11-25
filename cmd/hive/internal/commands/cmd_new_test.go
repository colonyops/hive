package commands

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/hive/internal/app"
	"github.com/colonyops/hive/cmd/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/internal/hive/hivetest"
	"github.com/colonyops/hive/internal/hive/servers"
)

type attachCountingMux struct {
	hivetest.Mux
	attaches int
}

func (m *attachCountingMux) AttachOrSwitch(context.Context, multiplexer.Target, multiplexer.AttachStreams) error {
	m.attaches++
	return nil
}

func TestCreateSessionFromFlagsAttaches(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-test,1,0")
	cfg := &config.Config{Config: *hivetest.LoadConfig(t, t.TempDir())}
	mux := &attachCountingMux{}
	h := hivetest.NewEngine(t, hivetest.Options{Config: &cfg.Config, Mux: mux})
	a := app.NewApp(zerolog.Nop(), h.Engine, cfg, nil, nil, nil, nil, "")
	a.Servers = servers.New(zerolog.Nop(), a.SessionAPI(), nil, "test")

	flags := &createSessionFlags{remote: "https://github.com/acme/site", source: t.TempDir()}
	sess, err := createSessionFromFlags(t.Context(), a, "front", flags, nil)
	require.NoError(t, err)
	require.NotNil(t, sess)
	assert.Equal(t, 1, mux.attaches)
}
