import { computed, onMounted, ref, watch } from 'vue'
import type { Router } from 'vue-router'
import { useFirstRun } from '../composables/useFirstRun'
import type { useGitHubConnection } from '../composables/useGitHubConnection'
import { useHiveSetup } from '../composables/useHiveSetup'
import { useNotificationSettings } from '../composables/useNotificationSettings'
import { useAgentWorkspaces } from '../stores/useAgentWorkspaces'
import { useHiveCommand } from '../stores/useHiveCommand'
import type { FeedState } from './useAppNavigation'

/**
 * The first-run walk: hive setup -> the hive command -> connect GitHub ->
 * notifications -> meet the agent. Hive setup goes first because the new-session picker is built from it
 * and abandoning it costs nothing that early. The agent hand-off is last
 * because it needs the agent the Hive step chose and the profile connecting
 * seeded. A profile is not a step: one exists before the app opens
 * (FlowsService.EnsureProfile).
 *
 * The walk is gated on a persisted marker, because with a profile always
 * present nothing else says whether the hand-off happened. Steps with their own
 * signal (a usable config, a connected account, a resolved grant) skip
 * themselves.
 */
export function useOnboarding(feed: FeedState, github: ReturnType<typeof useGitHubConnection>, router: Router) {
  const hive = useHiveSetup()
  const firstRun = useFirstRun()
  const notifications = useNotificationSettings()
  const command = useHiveCommand()
  const { startFirstRunChat } = useAgentWorkspaces()

  const hiveStepDone = ref(false)
  // A config this app could not read does not hold first run in front of the
  // whole app: Settings > Hive CLI shows the parse error. Whether the config is
  // usable picks which body the step renders, not whether it is up.
  const hiveStepActive = computed(
    () => firstRun.completed.value === false && !!hive.setup.value && !hive.unreadable.value && !hiveStepDone.value,
  )
  // A recorded choice is the step's own signal, so an install that already
  // answered (or a build that cannot install) never sees it.
  const commandStep = ref(false)
  const connectStep = ref(false)
  // The only place onboarding pops the OS prompt; advanceToPermissions skips a
  // permission that is already resolved.
  const permissionsStep = ref(false)
  const agentStep = ref(false)
  const agentError = ref<string | null>(null)
  const startingAgent = ref(false)
  const active = computed(
    () => hiveStepActive.value || commandStep.value || connectStep.value || permissionsStep.value || agentStep.value,
  )
  const loaded = computed(() => hive.loaded.value && firstRun.completed.value !== null)

  // Read alongside the profiles: the shell holds its empty frame until they
  // land, and a step that resolved later would flash in behind it.
  onMounted(() => {
    void hive.load()
    void firstRun.load()
  })

  // Starting before the GitHub status lands would put the connect card up for
  // an account that is connected.
  const ready = computed(
    () =>
      firstRun.completed.value !== null && hive.loaded.value && command.loaded.value && github.status.value !== null,
  )
  let started = false
  watch(
    ready,
    (isReady) => {
      if (!isReady || started || firstRun.completed.value !== false) return
      started = true
      if (!hiveStepActive.value) advanceToCommand()
    },
    { immediate: true },
  )

  function advanceToCommand(): void {
    const status = command.status.value
    commandStep.value = !!status && !status.asked && !status.unsupported
    if (!commandStep.value) advanceToConnect()
  }

  async function saveCommand(install: boolean): Promise<void> {
    if (!(await command.setInstall(install))) return
    commandStep.value = false
    advanceToConnect()
  }

  function advanceToConnect(): void {
    connectStep.value = !github.connected.value
    if (!connectStep.value) advanceToPermissions()
  }

  function advanceToPermissions(): void {
    permissionsStep.value = notifications.permission.value === 'not-requested'
    if (!permissionsStep.value) agentStep.value = true
  }

  // Saved, confirmed, or skipped, leaving the step ends it. The repository
  // picker's empty state points at Settings > Hive CLI afterwards.
  function finishHive(): void {
    hiveStepDone.value = true
    advanceToCommand()
  }

  async function finishFirstRun(): Promise<void> {
    agentStep.value = false
    await firstRun.complete()
  }

  // First run ends before the route changes so the Agents area is not gated
  // behind the screen handing off to it.
  async function startAgent(): Promise<void> {
    startingAgent.value = true
    agentError.value = null
    try {
      const chat = await startFirstRunChat()
      await finishFirstRun()
      await router.push({ name: 'agents', params: { workspace: chat.workspace }, query: { chat: String(chat.id) } })
    } catch (error) {
      agentError.value = error instanceof Error && error.message ? error.message : 'Could not start the chat.'
    } finally {
      startingAgent.value = false
    }
  }

  // Connecting seeds the default profile, which was made empty because a
  // source node names the account it fetches as. Only an empty profile is
  // seeded, so a replayed walk cannot append a second starter graph. The
  // connect card stays up until the seed lands, so the feed never renders
  // sourceless on the way through.
  watch(github.connected, async (connected) => {
    if (!connected || !connectStep.value) return
    const profile = feed.profiles.value.find((p) => p.id === feed.activeProfileId.value)
    try {
      if (profile && profile.nodes === 0) await feed.seedStarterFlow(profile.id)
    } catch (error) {
      console.warn('Unable to seed the starter flow', error)
      feed.showToast('Starter feeds were not added', {
        body: 'This profile has no sources yet — add one in the flow editor.',
        severity: 'error',
      })
    } finally {
      connectStep.value = false
      advanceToPermissions()
    }
  })

  const step = computed(() => {
    if (hiveStepActive.value) return 'hive'
    if (commandStep.value) return 'command'
    if (connectStep.value) return 'connect'
    if (permissionsStep.value) return 'permissions'
    return 'agent'
  })

  const screen = computed(() => {
    const byStep = {
      hive: { card: 'hive', error: hive.error.value, busy: hive.saving.value },
      command: { card: 'command', error: command.error.value, busy: command.saving.value },
      connect: { card: github.card.value, error: github.error.value, busy: github.busy.value },
      permissions: {
        card: 'permissions',
        error: notifications.error.value,
        busy: notifications.requestingPermission.value,
      },
      agent: { card: 'agent', error: agentError.value, busy: startingAgent.value },
    } as const
    return {
      ...byStep[step.value],
      deviceFlow: github.deviceFlow.value,
      githubConnected: github.connected.value,
      permission: notifications.permission.value,
      hive,
      command: command.status.value,
    }
  })

  const screenEvents = {
    saveHive: async () => {
      if (await hive.save()) finishHive()
    },
    finishHive,
    saveCommand,
    startDeviceFlow: github.startDeviceFlow,
    useTokenInstead: github.useTokenInstead,
    backToStart: github.backToStart,
    submitToken: github.submitToken,
    skipConnect: () => {
      connectStep.value = false
      advanceToPermissions()
    },
    requestPermission: notifications.requestPermission,
    finishPermissions: () => {
      permissionsStep.value = false
      agentStep.value = true
    },
    startAgent,
    finishAgent: finishFirstRun,
  }

  return { active, loaded, screen, screenEvents }
}
