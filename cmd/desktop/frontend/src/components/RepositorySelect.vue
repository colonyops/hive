<script setup lang="ts">
// The repository field of both session dialogs. It is not an AppSelect because
// the values it picks between are git remotes: too long to read as labels, too
// similar to tell apart by prefix, and open-ended — a remote that has never
// been cloned has to stay typeable. So rows read as "owner/name", the query
// matches them as a subsequence, and anything that looks like a remote can be
// entered verbatim.
import { onClickOutside } from '@vueuse/core'
import { computed, nextTick, ref, useId, watch } from 'vue'
import IconCheck from '~icons/lucide/check'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconGitBranch from '~icons/lucide/git-branch'
import IconLayers from '~icons/lucide/layers'
import IconSearch from '~icons/lucide/search'
import IconX from '~icons/lucide/x'
import type { SessionLaunchRepository } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'
import { useAnchoredPopover } from '@hive/ui/composables/useAnchoredPopover'
import { useListKeyboardNav } from '@hive/ui/composables/useListKeyboardNav'
import { highlightSegments, rankRepositories, remoteIdentity, repoDisplayName, toChoices } from '../lib/repositories'

export interface RepositoryPreset {
  name: string
  remotes: readonly string[]
}

const props = withDefaults(
  defineProps<{
    modelValue?: string
    repositories: SessionLaunchRepository[] | null | undefined
    testid?: string
    id?: string
    /** The server the list comes from, '' for this machine. */
    server?: string
    /** Pick any number of repositories into `selected`; the list stays open between picks. */
    multiple?: boolean
    selected?: readonly string[]
    /** Named sets of repositories, listed above the repositories; one replaces the selection. */
    presets?: readonly RepositoryPreset[]
  }>(),
  {
    modelValue: '',
    testid: undefined,
    id: undefined,
    server: '',
    multiple: false,
    selected: () => [],
    presets: () => [],
  },
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
  'update:selected': [value: string[]]
  preset: [name: string]
}>()

const listboxId = useId()
const root = ref<HTMLElement | null>(null)
const trigger = ref<HTMLElement | null>(null)
const popover = ref<HTMLElement | null>(null)
const list = ref<HTMLElement | null>(null)
const searchInput = ref<HTMLInputElement | null>(null)
const open = ref(false)
const query = ref('')
const active = ref(0)

const choices = computed(() => toChoices(props.repositories))
const ranked = computed(() => rankRepositories(choices.value, query.value, props.multiple ? '' : props.modelValue))
// In multiple mode another spelling of a selected remote is the same
// repository, so it reads as selected and a click removes the selected one.
const selectedIdentities = computed(() => new Set(props.selected.map(remoteIdentity)))
const isSelected = (remote: string): boolean =>
  props.multiple ? selectedIdentities.value.has(remoteIdentity(remote)) : remote === props.modelValue
// Presets name sets, not repositories, so a query filters them by name only.
const shownPresets = computed(() => {
  const q = query.value.trim().toLowerCase()
  return props.presets.filter((preset) => !q || preset.name.toLowerCase().includes(q))
})
const presetHint = (preset: RepositoryPreset): string =>
  preset.remotes.map((remote) => repoDisplayName(remote).split('/').pop() || remote).join(', ')

// A remote that is not on disk yet is still a valid answer, so the typed query
// stays selectable — but only once it looks like one, or every half-typed name
// would offer to become a repository.
const custom = computed(() => {
  const value = query.value.trim()
  if (!value || !/[/:.]/.test(value)) return ''
  return choices.value.some((choice) => choice.remote === value) || isSelected(value) ? '' : value
})
const presetCount = computed(() => shownPresets.value.length)
const rowCount = computed(() => presetCount.value + ranked.value.length + (custom.value ? 1 : 0))

const selectedLabel = computed(() => repoDisplayName(props.modelValue) || props.modelValue)

const nav = useListKeyboardNav({
  active,
  count: () => rowCount.value,
  open: () => open.value,
  homeEnd: () => true,
  row: (index) => list.value?.querySelectorAll('[role=option]')[index]?.firstElementChild,
})

const { style: popoverStyle, measure } = useAnchoredPopover(root, popover, open)

function openList(): void {
  open.value = true
  query.value = ''
  active.value = 0
  measure()
  void nextTick(() => {
    measure() // now that the list has rendered and its natural width is known
    searchInput.value?.focus()
  })
}

// The search box lives in the popover this removes, so closing while it holds
// focus would strand focus on <body> and the next Tab would restart at the top
// of the app instead of moving to the next field.
function close(): void {
  const reclaim = popover.value?.contains(document.activeElement) ?? false
  open.value = false
  if (reclaim) trigger.value?.focus()
}
function toggle(): void {
  open.value ? close() : openList()
}

function choose(remote: string): void {
  if (props.multiple) {
    const identity = remoteIdentity(remote)
    emit(
      'update:selected',
      isSelected(remote)
        ? props.selected.filter((item) => remoteIdentity(item) !== identity)
        : [...props.selected, remote],
    )
    query.value = ''
    searchInput.value?.focus()
    return
  }
  if (remote !== props.modelValue) emit('update:modelValue', remote)
  close()
}

function choosePreset(preset: RepositoryPreset): void {
  emit('preset', preset.name)
  close()
}

function remove(remote: string): void {
  emit(
    'update:selected',
    props.selected.filter((item) => item !== remote),
  )
}

function commitActive(): void {
  const preset = shownPresets.value[active.value]
  if (preset) {
    choosePreset(preset)
    return
  }
  const row = ranked.value[active.value - presetCount.value]
  if (row) {
    choose(row.remote)
    return
  }
  if (custom.value) choose(custom.value)
}

function onKeydown(event: KeyboardEvent): void {
  if (!open.value) {
    if (event.key === 'ArrowDown' || event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      openList()
    }
    return
  }
  if (nav.onKeydown(event)) return
  if (event.key === 'Escape') {
    event.preventDefault()
    event.stopPropagation()
    close()
  } else if (event.key === 'Enter') {
    event.preventDefault()
    commitActive()
  }
  // Not prevented: close() puts focus back on the trigger first, so the default
  // Tab carries on from there into the next field.
  else if (event.key === 'Tab') close()
}

// Typing re-ranks, so the previous active row is meaningless; the best match is.
watch(query, () => {
  active.value = 0
  measure()
})

onClickOutside(
  root,
  () => {
    if (open.value) close()
  },
  { ignore: [popover] },
)
</script>

<template>
  <div ref="root" class="relative">
    <!-- Keydown is bound where focus actually is — the trigger while closed,
         the search box while open — rather than on this root, which would
         double-handle every key the search box lets bubble. -->
    <div
      v-if="multiple"
      class="flex min-h-10.5 w-full flex-wrap items-center gap-1.5 rounded-lg border bg-app px-2 py-1.75"
      :class="open ? 'border-accent' : 'border-strong'"
      :data-testid="testid ? `${testid}-field` : undefined"
    >
      <span
        v-for="remote in selected"
        :key="remote"
        class="flex max-w-full min-w-0 items-center gap-1 rounded-md border border-card bg-raised py-0.5 pr-0.5 pl-2 text-small text-text"
        :title="remote"
        :data-testid="testid ? `${testid}-chip` : undefined"
        :data-remote="remote"
      >
        <span class="min-w-0 truncate">{{ repoDisplayName(remote) || remote }}</span>
        <button
          type="button"
          class="flex size-4.5 shrink-0 items-center justify-center rounded text-text-4 hover:bg-hover hover:text-text"
          :aria-label="`Remove ${repoDisplayName(remote) || remote}`"
          :data-testid="testid ? `${testid}-chip-remove` : undefined"
          @click="remove(remote)"
        >
          <IconX class="size-3" />
        </button>
      </span>
      <button
        :id="id"
        ref="trigger"
        type="button"
        class="flex min-w-32 flex-1 items-center gap-1.5 rounded px-1 py-0.5 text-left text-body text-text-4 outline-none"
        :data-testid="testid"
        aria-label="Add a repository"
        aria-haspopup="listbox"
        :aria-expanded="open"
        :aria-controls="open ? listboxId : undefined"
        @click="toggle"
        @keydown="onKeydown"
      >
        <IconGitBranch v-if="!selected.length" class="size-4 shrink-0 text-text-3" />
        <span class="min-w-0 flex-1 truncate">{{ selected.length ? 'Add another…' : 'Choose repositories' }}</span>
        <IconChevronDown class="size-4 shrink-0 text-text-3 transition-transform" :class="open ? 'rotate-180' : ''" />
      </button>
    </div>
    <button
      v-else
      :id="id"
      ref="trigger"
      type="button"
      class="flex w-full items-center gap-2 rounded-lg border bg-app px-3 py-2.5 text-left text-body text-text outline-none"
      :class="open ? 'border-accent' : 'border-strong'"
      :data-testid="testid"
      aria-label="Repository"
      aria-haspopup="listbox"
      :aria-expanded="open"
      :aria-controls="open ? listboxId : undefined"
      :title="modelValue"
      @click="toggle"
      @keydown="onKeydown"
    >
      <IconGitBranch class="size-4 shrink-0 text-text-3" />
      <span class="min-w-0 flex-1 truncate" :class="modelValue ? '' : 'text-text-4'">{{
        modelValue ? selectedLabel : 'Choose a repository'
      }}</span>
      <IconChevronDown class="size-4 shrink-0 text-text-3 transition-transform" :class="open ? 'rotate-180' : ''" />
    </button>

    <Teleport to="body">
      <div
        v-if="open"
        ref="popover"
        class="fixed z-50 flex flex-col overflow-hidden rounded-lg border border-card bg-raised shadow-popover"
        :style="popoverStyle"
        :data-testid="testid ? `${testid}-popover` : undefined"
      >
        <div class="flex shrink-0 items-center gap-2 border-b border-row px-2.5 py-2">
          <IconSearch class="size-3.5 shrink-0 text-text-4" />
          <!-- w-0: an input's default intrinsic width would otherwise set the popover's width. -->
          <input
            ref="searchInput"
            v-model="query"
            type="text"
            :placeholder="
              presets.length ? 'Search projects and repositories…' : 'Search repositories or paste a remote…'
            "
            class="w-0 min-w-0 flex-1 bg-transparent text-body text-text outline-none placeholder:text-text-4"
            :data-testid="testid ? `${testid}-search` : undefined"
            @keydown="onKeydown"
          />
        </div>
        <ul
          v-if="rowCount"
          :id="listboxId"
          ref="list"
          class="hive-scroll flex min-h-0 flex-col gap-0.5 overflow-y-auto p-1.25"
          role="listbox"
          aria-label="Repository"
        >
          <li
            v-for="(preset, index) in shownPresets"
            :key="`preset:${preset.name}`"
            role="option"
            :aria-selected="false"
          >
            <button
              type="button"
              class="flex w-full items-center gap-2 rounded-md px-2.25 py-1.75 text-left text-body"
              :class="index === active ? 'bg-hover text-text' : 'text-text-2'"
              :data-testid="testid ? `${testid}-preset` : undefined"
              :data-preset="preset.name"
              :title="preset.remotes.join('\n')"
              @mousedown.prevent
              @click="choosePreset(preset)"
              @mousemove="active = index"
            >
              <IconLayers class="size-3.5 shrink-0 text-accent" />
              <span class="shrink-0">{{ preset.name }}</span>
              <span class="min-w-0 flex-1 truncate text-right text-caption text-text-4">{{ presetHint(preset) }}</span>
            </button>
          </li>
          <li v-if="presetCount && ranked.length" role="presentation" class="mx-1 my-1 border-t border-row" />
          <li v-for="(repo, index) in ranked" :key="repo.remote" role="option" :aria-selected="isSelected(repo.remote)">
            <button
              type="button"
              class="flex w-full items-center gap-2 rounded-md px-2.25 py-1.75 text-left text-body"
              :class="
                index + presetCount === active
                  ? 'bg-hover text-text'
                  : isSelected(repo.remote)
                    ? 'text-text'
                    : 'text-text-2'
              "
              :data-testid="testid ? `${testid}-option` : undefined"
              :title="repo.remote"
              @mousedown.prevent
              @click="choose(repo.remote)"
              @mousemove="active = index + presetCount"
            >
              <span class="min-w-0 flex-1 truncate">
                <span
                  v-for="(segment, part) in highlightSegments(repo.label, repo.indices)"
                  :key="part"
                  :class="segment.matched ? 'font-semibold text-accent' : ''"
                  >{{ segment.text }}</span
                >
              </span>
              <span v-if="repo.hint" class="shrink-0 truncate text-caption text-text-4">{{ repo.hint }}</span>
              <IconCheck v-if="isSelected(repo.remote)" class="size-3.5 shrink-0 text-accent" :stroke-width="3" />
            </button>
          </li>
          <li v-if="custom" role="option" :aria-selected="false">
            <button
              type="button"
              class="flex w-full items-center gap-2 rounded-md px-2.25 py-1.75 text-left text-body"
              :class="active === presetCount + ranked.length ? 'bg-hover text-text' : 'text-text-2'"
              :data-testid="testid ? `${testid}-custom` : undefined"
              @mousedown.prevent
              @click="choose(custom)"
              @mousemove="active = presetCount + ranked.length"
            >
              <span class="shrink-0 text-text-4">Use</span>
              <span class="min-w-0 flex-1 truncate">{{ custom }}</span>
            </button>
          </li>
        </ul>
        <div
          v-else
          class="px-3 py-4 text-center text-small text-text-4"
          :data-testid="testid ? `${testid}-empty` : undefined"
        >
          <template v-if="query.trim()">No matching repository</template>
          <template v-else>
            No repositories configured
            <!-- The list is built from the Hive config's workspaces, so an
                 empty one is a setting, not a missing feature. Someone who
                 skipped the first-run step lands here and would otherwise have
                 nothing to go on. -->
            <span v-if="server" class="mt-1 block text-caption">
              Type a URL that {{ server }} can clone, or add workspaces to the config.yaml on {{ server }}.
            </span>
            <span v-else class="mt-1 block text-caption">Add the folders holding them under Settings ▸ Hive CLI.</span>
          </template>
        </div>
      </div>
    </Teleport>
  </div>
</template>
