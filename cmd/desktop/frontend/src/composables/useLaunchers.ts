import { ref, type Ref } from 'vue'
import { Launchers } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/popupterminalservice'
import type { PopupLauncher } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import { launcherCommandID, setLauncherCommands } from '../keybindings/catalog'
import { launcherIconComponent } from '../lib/launcherIcons'

// The configured launchers — the `terminal-popup` actions in actions.yml. Each
// one is a bindable command of its own, so this is what turns a line of config
// into something the palette lists and a chord can reach.
//
// Only identity crosses the bridge: what a launcher runs stays in the core and
// is resolved when it is opened by id, so a stale catalog here cannot run a
// command the user has since changed. Where it may run is the core's answer
// too — `requiresSession` is what a launcher with no configured cwd reports,
// and it becomes the command's context so the palette and the keymap gate on
// the same thing the core enforces (ADR quick-terminal-launchers-are-session-scoped).

const launchers = ref<PopupLauncher[]>([])

export function useLaunchers(): {
  launchers: Ref<PopupLauncher[]>
  refresh: () => Promise<void>
} {
  return { launchers, refresh }
}

async function refresh(): Promise<void> {
  try {
    launchers.value = (await Launchers()) ?? []
  } catch (error) {
    // A catalog that cannot be read leaves the launchers that were already
    // registered alone: losing every shortcut over one failed read would be
    // worse than serving the set from a moment ago.
    console.warn('Unable to read the configured launchers', error)
    return
  }
  setLauncherCommands(launchers.value.map((launcher) => ({
    id: launcherCommandID(launcher.id),
    title: launcher.label || launcher.id,
    group: 'Quick terminals',
    keywords: ['launcher', 'terminal', 'popup', launcher.id],
    icon: launcherIconComponent(launcher.icon),
    // Unbound until the user says otherwise: a config file must not claim a
    // chord the app never offered to give it.
    defaultCombos: [],
    context: launcher.requiresSession ? 'terminal-session' : 'global',
  })))
}

export function resetLaunchersForTests(): void {
  launchers.value = []
  setLauncherCommands([])
}
