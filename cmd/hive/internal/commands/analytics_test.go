package commands

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/colonyops/hive/cmd/hive/internal/app"
	domain "github.com/colonyops/hive/internal/domain/usageanalytics"
	recorder "github.com/colonyops/hive/internal/hive/usageanalytics"
	store "github.com/colonyops/hive/internal/store/usageanalytics"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

type captureAnalytics struct{ events []domain.Event }

func (r *captureAnalytics) Record(_ context.Context, e domain.Event) { r.events = append(r.events, e) }

func TestCatalogCoversRegisteredCommands(t *testing.T) {
	flags, facade := &Flags{}, &app.App{}
	registrations := []interface {
		Register(*cli.Command) *cli.Command
	}{
		NewNewCmd(flags, facade), NewPruneCmd(flags, facade), NewDoctorCmd(flags, facade), NewBatchCmd(flags, facade), NewCtxCmd(flags, facade), NewMsgCmd(flags, facade), NewDocCmd(flags, facade), NewSessionCmd(flags, facade), NewReviewCmd(flags, facade), NewTodoCmd(flags, facade), NewConfigCmd(flags, facade), NewDetectCmd(flags, facade), NewHoneycombCmd(flags, facade), NewWorkspaceCmd(flags, facade), NewExperimentalCmd(flags, facade),
	}
	root := &cli.Command{Name: "hive"}
	for _, registration := range registrations {
		root = registration.Register(root)
	}
	var visit func(*cli.Command, string)
	visit = func(parent *cli.Command, prefix string) {
		for _, command := range parent.Commands {
			path := prefix + command.Name
			if command.Action != nil {
				_, ok := domain.LookupCommand(path)
				if !ok {
					t.Errorf("missing catalog path %s", path)
				}
			}
			visit(command, path+".")
		}
	}
	visit(root, "")
}

func TestCommandInstrumentationLateBinding(t *testing.T) {
	for _, success := range []bool{true, false} {
		facade := &app.App{}
		sentinel := errors.New("original failure")
		root := &cli.Command{Name: "hive", Writer: io.Discard, ErrWriter: io.Discard, Commands: []*cli.Command{{Name: "session", Commands: []*cli.Command{{Name: "list", Aliases: []string{"ls"}, Action: func(context.Context, *cli.Command) error {
			if success {
				return nil
			}
			return sentinel
		}}}}}}
		InstrumentCommands(root, facade)
		capture := &captureAnalytics{}
		root.Before = func(ctx context.Context, _ *cli.Command) (context.Context, error) {
			facade.Analytics = capture
			return ctx, nil
		}
		err := root.Run(t.Context(), []string{"hive", "session", "ls", "private-user-argument"})
		if success {
			require.NoError(t, err)
		} else {
			require.ErrorIs(t, err, sentinel)
		}
		require.Len(t, capture.events, 1)
		require.Contains(t, capture.events[0].Properties(), `"command":"session.list"`)
		require.NotContains(t, capture.events[0].Properties(), "private-user-argument")
	}
}

func TestCommandAfterFlushesAndBeforeFailureCloses(t *testing.T) {
	for _, beforeFailure := range []bool{false, true} {
		dir := t.TempDir()
		facade := &app.App{}
		var manager *recorder.Service
		root := &cli.Command{Name: "hive", Writer: io.Discard, ErrWriter: io.Discard, Commands: []*cli.Command{{Name: "ls", Action: func(context.Context, *cli.Command) error { return nil }}}}
		InstrumentCommands(root, facade)
		root.Before = func(ctx context.Context, _ *cli.Command) (context.Context, error) {
			manager = recorder.New(ctx, zerolog.Nop(), recorder.Options{Enabled: true, DataDir: dir, Surface: "cli"})
			facade.Analytics = manager
			if beforeFailure {
				return ctx, errors.New("before failed")
			}
			return ctx, nil
		}
		root.After = func(context.Context, *cli.Command) error { manager.Close(); return nil }
		err := root.Run(t.Context(), []string{"hive", "ls"})
		if beforeFailure {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
		counts, err := store.ReadSummary(t.Context(), dir, time.Now())
		require.NoError(t, err)
		if beforeFailure {
			require.Zero(t, counts.CLICommands)
		} else {
			require.EqualValues(t, 1, counts.CLICommands)
		}
	}
}

func TestHelpInitAndCompletionDoNotRecord(t *testing.T) {
	capture := &captureAnalytics{}
	root := &cli.Command{Name: "hive", Writer: io.Discard, ErrWriter: io.Discard, Commands: []*cli.Command{
		{Name: "init", Action: func(context.Context, *cli.Command) error { return nil }},
		{Name: "completion", Action: func(context.Context, *cli.Command) error { return nil }},
	}}
	InstrumentCommands(root, &app.App{Analytics: capture})
	for _, path := range []string{"init", "completion", "help"} {
		require.NoError(t, root.Run(t.Context(), []string{"hive", path}))
	}
	require.Empty(t, capture.events)
}
