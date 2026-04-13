package commands

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/data/stores"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
)

// hookPayload is the JSON payload sent by Claude Code to the hook handler via stdin.
type hookPayload struct {
	HookEventName string `json:"hook_event_name"`
	SessionID     string `json:"session_id"` // Claude's internal UUID, not Hive session ID
	Source        string `json:"source"`
}

// HookHandlerCmd handles the `hive hook-handler` subcommand.
type HookHandlerCmd struct {
	flags *Flags
}

// NewHookHandlerCmd creates a new hook-handler command.
func NewHookHandlerCmd(flags *Flags) *HookHandlerCmd {
	return &HookHandlerCmd{flags: flags}
}

// Register adds the hook-handler command to the application.
func (cmd *HookHandlerCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:   "hook-handler",
		Usage:  "Handle Claude Code lifecycle hook events (called by Claude, not users)",
		Hidden: true,
		Action: cmd.run,
	})
	return app
}

func (cmd *HookHandlerCmd) run(_ context.Context, _ *cli.Command) error {
	// 10s overall deadline — caps total runtime including DB operations.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Decode JSON payload from stdin (5s deadline for read).
	readCtx, readCancel := context.WithTimeout(ctx, 5*time.Second)
	defer readCancel()

	var payload hookPayload
	done := make(chan error, 1)
	go func() {
		done <- json.NewDecoder(os.Stdin).Decode(&payload)
	}()
	select {
	case err := <-done:
		if err != nil {
			log.Debug().Err(err).Msg("hook-handler: failed to decode stdin payload")
			return nil
		}
	case <-readCtx.Done():
		log.Debug().Msg("hook-handler: stdin read timeout")
		return nil
	}

	// Map event to status. Unknown events are silently ignored.
	status, ok := terminal.MapEventToStatus(payload.HookEventName)
	if !ok {
		log.Debug().Str("event", payload.HookEventName).Msg("hook-handler: unknown event, ignoring")
		return nil
	}

	// Open DB with minimal options — no need for a full connection pool.
	database, err := db.Open(cmd.flags.DataDir, db.OpenOptions{
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	if err != nil {
		log.Warn().Err(err).Msg("hook-handler: failed to open database")
		return nil
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Debug().Err(err).Msg("hook-handler: failed to close database")
		}
	}()

	sessionStore := stores.NewSessionStore(database)
	kvStore := stores.NewKVStore(database)

	resolver := terminal.NewHookResolver(sessionStore)
	sessionID, windowIndex, err := resolver.Resolve(ctx)
	if err != nil {
		log.Debug().Err(err).Msg("hook-handler: session resolution error")
		return nil
	}
	if sessionID == "" {
		log.Debug().Str("event", payload.HookEventName).Msg("hook-handler: no matching Hive session, ignoring")
		return nil
	}

	hookKV := kv.Scoped[terminal.HookStatus](kvStore, "hook.status")
	key := sessionID + ":" + windowIndex

	if status == terminal.StatusMissing {
		// SessionEnd: remove the key so the TUI falls back to tmux.
		if err := hookKV.Delete(ctx, key); err != nil {
			log.Debug().Err(err).Msg("hook-handler: failed to delete hook status")
		}
		return nil
	}

	hs := terminal.HookStatus{
		Status:      status,
		Event:       payload.HookEventName,
		WindowIndex: windowIndex,
		WrittenAt:   time.Now(),
	}
	// 24h TTL for orphan cleanup. Staleness is checked application-side via WrittenAt.
	if err := hookKV.SetTTL(ctx, key, hs, 24*time.Hour); err != nil {
		log.Warn().Err(err).Msg("hook-handler: failed to write hook status")
	}

	return nil
}
