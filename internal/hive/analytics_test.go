package hive_test

import (
	"context"
	"testing"

	domain "github.com/colonyops/hive/internal/domain/usageanalytics"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/events/testbus"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/pkg/executil/executiltest"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

type captureUsage struct{ events []domain.Event }

func (c *captureUsage) Record(_ context.Context, event domain.Event) {
	c.events = append(c.events, event)
}

func TestEngineKeepsProcessRecorderAcrossReload(t *testing.T) {
	cfg := loadConfig(t, t.TempDir())
	database, err := hive.OpenDB(t.Context(), cfg.DataDir, cfg.Database)
	require.NoError(t, err)
	defer func() { require.NoError(t, database.Close()) }()
	capture := &captureUsage{}
	engine, err := hive.New(cfg, hive.Ports{DB: database, Bus: testbus.New(t).EventBus, Executor: &executiltest.Exec{}, Mux: stubMux{}, Logger: zerolog.Nop(), DataDir: cfg.DataDir, Analytics: capture})
	require.NoError(t, err)
	for _, name := range []string{"before", "after"} {
		_, err := engine.Sessions().CreateSession(t.Context(), sessionsvc.CreateOptions{Name: name, Remote: "https://github.com/test/repo", SkipSpawn: true})
		require.NoError(t, err)
		require.NoError(t, engine.Reload(cfg))
	}
	require.Len(t, capture.events, 2)
	for _, event := range capture.events {
		require.Equal(t, domain.SessionCreatedName, event.Name())
	}
}
