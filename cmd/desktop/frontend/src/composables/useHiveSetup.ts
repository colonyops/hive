import { computed, ref } from 'vue'
import {
  InspectWorkspace,
  Save,
  Setup,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/hiveconfigservice'
import { ChooseDirectory } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/systemservice'
import type { HiveSetup } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import type { AgentOption, Profile, Workspace } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/hiveconf/models'

function errText(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

function flagRunAt(flags: string[], run: string[], at: number): boolean {
  return run.every((flag, j) => flags[at + j] === flag)
}

function hasFlagRun(flags: string[] | null | undefined, run: string[]): boolean {
  if (!flags || !run.length) return false
  return flags.some((_, i) => flagRunAt(flags, run, i))
}

function withoutFlagRun(flags: string[], run: string[]): string[] {
  if (!run.length) return flags
  const out = [...flags]
  for (let i = 0; i + run.length <= out.length;) {
    if (flagRunAt(out, run, i)) out.splice(i, run.length)
    else i++
  }
  return out
}

/**
 * A workspace in the draft. `repos` is the count the backend found when the
 * folder was added or loaded; it is a confirmation that the folder is the one
 * the user meant, not a promise about the launcher's list.
 */
export interface DraftWorkspace {
  path: string
  exists: boolean
  repos: number
}

/**
 * The draft is deliberately whole rather than a set of pending changes: the
 * backend reconciles the file to exactly what it is given, so the profiles
 * this never shows a control for (a hand-written one with a custom command)
 * ride along untouched instead of being dropped by a save about workspaces.
 *
 * First run is the only screen that edits the draft. Settings ▸ Hive CLI reads
 * one to report a config that would not parse, and points at the file for
 * every change. Every caller gets its own instance, so neither can see the
 * other's draft.
 */
export function useHiveSetup() {
  const setup = ref<HiveSetup | null>(null)
  // loaded is "the read finished", success or not; setup is "it succeeded".
  // A shell waiting on the first cannot flash a step in behind a rendered
  // feed, while a step gated on the second stays down for a config that
  // could not be read.
  const loaded = ref(false)
  const saving = ref(false)
  // Folders still being checked by the backend. A save sent while one is in
  // flight would land without it and then overwrite the draft it arrives in.
  const inspecting = ref(0)
  const error = ref('')

  const profiles = ref<Profile[]>([])
  const workspaces = ref<DraftWorkspace[]>([])
  const defaultAgent = ref('')
  const skipPermissions = ref(false)

  const agents = computed<AgentOption[]>(() => setup.value?.agents ?? [])
  const path = computed(() => setup.value?.config.path ?? '')
  const usable = computed(() => setup.value?.config.usable ?? false)
  const unreadable = computed(() => setup.value?.config.unreadable ?? '')
  const defaultAgentOverride = computed(() => setup.value?.defaultAgentOverride ?? '')

  const selectedAgents = computed(() => new Set(profiles.value.map(p => p.name)))

  /**
   * Profiles in the draft that the agent picker has no row for: a
   * hand-written profile, or one from a hive version that knows an agent this
   * build does not. They are listed so a user can see what a save will keep.
   */
  const customProfiles = computed(() => {
    const known = new Set(agents.value.map(a => a.name))
    return profiles.value.filter(p => !known.has(p.name))
  })

  // A file that did not parse is the user's to fix in an editor: the backend
  // refuses to rewrite it, so offering a save would offer a save that fails.
  const canSave = computed(() =>
    !unreadable.value
    && inspecting.value === 0
    && profiles.value.length > 0
    && workspaces.value.length > 0
    && profiles.value.some(p => p.name === defaultAgent.value),
  )

  async function load(): Promise<void> {
    error.value = ''
    try {
      const next = await Setup()
      setup.value = next
      resetDraft(next)
    } catch (err) {
      error.value = errText(err)
    } finally {
      loaded.value = true
    }
  }

  function resetDraft(next: HiveSetup): void {
    profiles.value = (next.config.profiles ?? []).map(p => ({ ...p, flags: p.flags ? [...p.flags] : null }))
    workspaces.value = (next.config.workspaces ?? []).map(toDraftWorkspace)
    defaultAgent.value = next.config.defaultAgent || profiles.value[0]?.name || ''
    // Reflect what the file already says rather than defaulting the toggle
    // off: a user who turned skip-permissions on should not have it silently
    // turned back off by opening the form and saving.
    //
    // Only the agent's own skip flags count. A profile carrying other flags
    // (`--model opus`) says nothing about this toggle, and letting them light
    // the box up would arm a control that then deletes them on the way off.
    const catalog = new Map((next.agents ?? []).map(a => [a.name, a]))
    skipPermissions.value = profiles.value.some((p) => {
      const agent = catalog.get(p.name)
      return !!agent && hasFlagRun(p.flags, agent.skipPermissionFlags ?? [])
    })
    if (!profiles.value.length) seedFromInstalledAgent(next)
  }

  // A first run with nothing configured starts on the agent this machine
  // actually has, so the common case is one click. With none installed it
  // starts on nothing and the user picks — guessing an agent that is not here
  // would be worse than asking.
  function seedFromInstalledAgent(next: HiveSetup): void {
    const installed = (next.agents ?? []).find(a => a.installed)
    if (!installed) return
    toggleAgent(installed, true)
    defaultAgent.value = installed.name
  }

  function toDraftWorkspace(w: Workspace): DraftWorkspace {
    return { path: w.path, exists: w.exists, repos: w.repos }
  }

  // Skip flags are handled as one contiguous run. For example, removing
  // `--agent free-permissions-runner` token by token could also strip an
  // unrelated `--agent` the user wrote.
  function flagsFor(agent: AgentOption, existing: string[] | null): string[] {
    const skip = agent.skipPermissionFlags ?? []
    const kept = withoutFlagRun(existing ?? [], skip)
    return skipPermissions.value && skip.length ? [...kept, ...skip] : kept
  }

  function toggleAgent(agent: AgentOption, on: boolean): void {
    if (on) {
      if (selectedAgents.value.has(agent.name)) return
      profiles.value = [...profiles.value, { name: agent.name, command: agent.name, flags: flagsFor(agent, []) }]
      if (!defaultAgent.value) defaultAgent.value = agent.name
      return
    }
    profiles.value = profiles.value.filter(p => p.name !== agent.name)
    // Removing the default would save a config hive refuses to load, so the
    // next remaining profile takes over rather than leaving it dangling.
    if (defaultAgent.value === agent.name) defaultAgent.value = profiles.value[0]?.name ?? ''
  }

  function setDefaultAgent(name: string): void {
    defaultAgent.value = name
  }

  // Custom profiles keep their flags because this toggle controls only the
  // presets this app offers.
  function setSkipPermissions(on: boolean): void {
    skipPermissions.value = on
    const catalog = new Map(agents.value.map(a => [a.name, a]))
    profiles.value = profiles.value.map((p) => {
      const agent = catalog.get(p.name)
      return agent ? { ...p, flags: flagsFor(agent, p.flags) } : p
    })
  }

  async function addWorkspace(): Promise<void> {
    error.value = ''
    let chosen = ''
    try {
      chosen = await ChooseDirectory('Choose the folder that holds your repositories')
    } catch (err) {
      error.value = errText(err)
      return
    }
    if (!chosen) return
    await addWorkspacePath(chosen)
  }

  async function addWorkspacePath(path: string): Promise<void> {
    const trimmed = path.trim()
    if (!trimmed) return
    if (workspaces.value.some(w => w.path === trimmed)) return
    error.value = ''
    inspecting.value++
    try {
      const found = await InspectWorkspace(trimmed)
      workspaces.value = [...workspaces.value, toDraftWorkspace(found)]
    } catch (err) {
      error.value = errText(err)
    } finally {
      inspecting.value--
    }
  }

  function removeWorkspace(path: string): void {
    workspaces.value = workspaces.value.filter(w => w.path !== path)
  }

  /**
   * Writes the draft and reloads the running Hive services from it. Returns
   * whether the save landed, so a caller driving a wizard can advance only on
   * success.
   */
  async function save(): Promise<boolean> {
    if (saving.value) return false
    saving.value = true
    error.value = ''
    try {
      const next = await Save({
        defaultAgent: defaultAgent.value,
        profiles: profiles.value,
        workspaces: workspaces.value.map(w => w.path),
      })
      setup.value = next
      resetDraft(next)
      return true
    } catch (err) {
      error.value = errText(err)
      return false
    } finally {
      saving.value = false
    }
  }

  return {
    setup,
    loaded,
    saving,
    error,
    agents,
    profiles,
    workspaces,
    defaultAgent,
    skipPermissions,
    selectedAgents,
    customProfiles,
    canSave,
    path,
    usable,
    unreadable,
    defaultAgentOverride,
    load,
    toggleAgent,
    setDefaultAgent,
    setSkipPermissions,
    addWorkspace,
    addWorkspacePath,
    removeWorkspace,
    save,
  }
}
