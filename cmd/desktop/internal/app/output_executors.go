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
// is the flow notify terminal's own action type. A notify node's config lives
// in its flow, not in actions.yml, so it has no registry entry in package
// actions and never will — FlowNotifyActions resolves it from the live flow
// set instead (see buildOutputWorker). The invariant the test holds is
// therefore coverage of actions.Types(), not set equality with this map's
// keys.
//
// "shell" and "publish-message" stay as literals: package actions has no
// exported name for them, only the registry key Types() derives from.
func outputExecutors(
	launcher dispatch.SessionLauncher,
	workspaceLauncher dispatch.WorkspaceSessionLauncher,
	publisher dispatch.MessagePublisher,
	notifier dispatch.SystemNotifier,
	gate dispatch.NotificationGate,
	items dispatch.InboxItemLocator,
	env dispatch.ExecEnvironment,
	logger zerolog.Logger,
) map[string]dispatch.Executor {
	return map[string]dispatch.Executor{
		dispatch.ActionTypeLaunchSession: dispatch.NewLaunchSessionExecutor(logger, launcher, workspaceLauncher, env),
		"shell":                          dispatch.NewShellExecutor(logger, env),
		"publish-message":                dispatch.NewPublishMessageExecutor(publisher),
		"clipboard":                      dispatch.NewClipboardExecutor(),
		dispatch.ActionTypeNotify:        dispatch.NewNotifyExecutor(notifier, gate, items, logger),
	}
}
