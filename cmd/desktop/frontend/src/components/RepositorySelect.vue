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
import IconSearch from '~icons/lucide/search'
import type { SessionLaunchRepository } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'
import { useAnchoredPopover } from '../composables/useAnchoredPopover'
import { highlightSegments, rankRepositories, repoDisplayName, toChoices } from '../lib/repositories'

const props = defineProps<{
  modelValue: string
  repositories: SessionLaunchRepository[] | null | undefined
  testid?: string
}>()

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

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
const ranked = computed(() => rankRepositories(choices.value, query.value, props.modelValue))

// A remote that is not on disk yet is still a valid answer, so the typed query
// stays selectable — but only once it looks like one, or every half-typed name
// would offer to become a repository.
const custom = computed(() => {
  const value = query.value.trim()
  if (!value || !/[/:.]/.test(value)) return ''
  return choices.value.some((choice) => choice.remote === value) ? '' : value
})
const rowCount = computed(() => ranked.value.length + (custom.value ? 1 : 0))

const selectedLabel = computed(() => repoDisplayName(props.modelValue) || props.modelValue)

watch(rowCount, (count) => {
  if (active.value >= count) active.value = Math.max(0, count - 1)
})

function revealActive(): void {
  if (!open.value) return
  void nextTick(() =>
    (list.value?.children[active.value]?.firstElementChild as HTMLElement | undefined)?.scrollIntoView?.({
      block: 'nearest',
    }),
  )
}
watch(active, revealActive)

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
  if (remote !== props.modelValue) emit('update:modelValue', remote)
  close()
}

function commitActive(): void {
  const row = ranked.value[active.value]
  if (row) {
    choose(row.remote)
    return
  }
  if (custom.value) choose(custom.value)
}

function step(delta: number): void {
  const count = rowCount.value
  if (!count) return
  active.value = (active.value + delta + count) % count
}

function onKeydown(event: KeyboardEvent): void {
  if (!open.value) {
    if (event.key === 'ArrowDown' || event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      openList()
    }
    return
  }
  if (event.key === 'Escape') {
    event.preventDefault()
    event.stopPropagation()
    close()
  } else if (event.key === 'ArrowDown') {
    event.preventDefault()
    step(1)
  } else if (event.key === 'ArrowUp') {
    event.preventDefault()
    step(-1)
  } else if (event.key === 'Home') {
    event.preventDefault()
    active.value = 0
  } else if (event.key === 'End') {
    event.preventDefault()
    active.value = Math.max(0, rowCount.value - 1)
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
    <button
      ref="trigger"
      type="button"
      class="flex w-full items-center gap-2 rounded-lg border bg-app px-3 py-2.5 text-left text-[13.5px] text-text outline-none"
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
        class="fixed z-50 flex flex-col overflow-hidden rounded-lg border border-card bg-raised shadow-[0_16px_34px_-12px_rgba(0,0,0,.6)]"
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
            placeholder="Search repositories or paste a remote…"
            class="w-0 min-w-0 flex-1 bg-transparent text-[13px] text-text outline-none placeholder:text-text-4"
            :data-testid="testid ? `${testid}-search` : undefined"
            @keydown="onKeydown"
          />
        </div>
        <ul
          v-if="rowCount"
          :id="listboxId"
          ref="list"
          class="hive-scroll flex min-h-0 flex-col gap-0.5 overflow-y-auto p-[5px]"
          role="listbox"
          aria-label="Repository"
        >
          <li
            v-for="(repo, index) in ranked"
            :key="repo.remote"
            role="option"
            :aria-selected="repo.remote === modelValue"
          >
            <button
              type="button"
              class="flex w-full items-center gap-2 rounded-md px-[9px] py-[7px] text-left text-[13px]"
              :class="
                index === active ? 'bg-hover text-text' : repo.remote === modelValue ? 'text-text' : 'text-text-2'
              "
              :data-testid="testid ? `${testid}-option` : undefined"
              :title="repo.remote"
              @mousedown.prevent
              @click="choose(repo.remote)"
              @mousemove="active = index"
            >
              <span class="min-w-0 flex-1 truncate">
                <span
                  v-for="(segment, part) in highlightSegments(repo.label, repo.indices)"
                  :key="part"
                  :class="segment.matched ? 'font-semibold text-accent' : ''"
                  >{{ segment.text }}</span
                >
              </span>
              <span v-if="repo.hint" class="shrink-0 truncate text-[11.5px] text-text-4">{{ repo.hint }}</span>
              <IconCheck v-if="repo.remote === modelValue" class="size-3.5 shrink-0 text-accent" :stroke-width="3" />
            </button>
          </li>
          <li v-if="custom" role="option" :aria-selected="false">
            <button
              type="button"
              class="flex w-full items-center gap-2 rounded-md px-[9px] py-[7px] text-left text-[13px]"
              :class="active === ranked.length ? 'bg-hover text-text' : 'text-text-2'"
              :data-testid="testid ? `${testid}-custom` : undefined"
              @mousedown.prevent
              @click="choose(custom)"
              @mousemove="active = ranked.length"
            >
              <span class="shrink-0 text-text-4">Use</span>
              <span class="min-w-0 flex-1 truncate">{{ custom }}</span>
            </button>
          </li>
        </ul>
        <div
          v-else
          class="px-3 py-4 text-center text-[12.5px] text-text-4"
          :data-testid="testid ? `${testid}-empty` : undefined"
        >
          <template v-if="query.trim()">No matching repository</template>
          <template v-else>
            No repositories configured
            <!-- The list is built from the Hive config's workspaces, so an
                 empty one is a setting, not a missing feature. Someone who
                 skipped the first-run step lands here and would otherwise have
                 nothing to go on. -->
            <span class="mt-1 block text-[11.5px]">Add the folders holding them under Settings ▸ Hive CLI.</span>
          </template>
        </div>
      </div>
    </Teleport>
  </div>
</template>
