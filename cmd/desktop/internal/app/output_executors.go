package app

import (
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
)

// outputExecutors builds the dispatch executor map: one Executor per action
// type the output worker can run. It is a function of its dependencies
// rather than a method so TestOutputExecutorsCoverEveryActionType can hold it
// against actions.Types() without standing up an App — the same split
// buildSources/sourceFactories uses for the connector registry.
//
// The map is not a mirror of the actions.yml catalog: dispatch.ActionTypeNotify
// and dispatch.ActionTypeCreateTask belong to flow terminals. Their config
// lives in the flow, not in actions.yml, so they have no registry entry in
// package actions and never will -- FlowActions resolves them from the live
// flow set instead (see buildOutputWorker). The invariant the test holds is
// therefore coverage of actions.Types(), not set equality with this map's
// keys.
//
// "shell" stays a literal: package actions has no exported name for it, only
// the registry key Types() derives from.
func outputExecutors(
	launcher dispatch.SessionLauncher,
	workspaceLauncher dispatch.WorkspaceSessionLauncher,
	messages func() dispatch.MessageService,
	tasks func() dispatch.TaskCreator,
	notifier dispatch.SystemNotifier,
	gate dispatch.NotificationGate,
	items dispatch.InboxItemLocator,
	env dispatch.ExecEnvironment,
	logger zerolog.Logger,
) map[string]dispatch.Executor {
	return map[string]dispatch.Executor{
		dispatch.ActionTypeLaunchSession:  dispatch.NewLaunchSessionExecutor(logger, launcher, workspaceLauncher, env),
		"shell":                           dispatch.NewShellExecutor(logger, env),
		dispatch.ActionTypePublishMessage: dispatch.NewPublishMessageExecutor(messages),
		dispatch.ActionTypeCreateTask:     dispatch.NewCreateTaskExecutor(tasks),
		"clipboard":                       dispatch.NewClipboardExecutor(),
		dispatch.ActionTypeNotify:         dispatch.NewNotifyExecutor(notifier, gate, items, logger),
	}
}
