<script setup lang="ts">
import { errorText } from '../lib/appError'
import { computed, onMounted, ref } from 'vue'
import IconArrowDown from '~icons/lucide/arrow-down'
import IconArrowUp from '~icons/lucide/arrow-up'
import IconX from '~icons/lucide/x'
import AppSelect from './ui/AppSelect.vue'
import InlineError from './ui/InlineError.vue'
import SettingsPage from './settings/SettingsPage.vue'
import SettingsRow from './settings/SettingsRow.vue'
import SettingsSection from './settings/SettingsSection.vue'
import {
  FeedChoices,
  Limits,
  Pins,
  SetPins,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/menubarservice'
import type {
  MenuBarFeedChoice,
  MenuBarPin,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'

const pins = ref<MenuBarPin[]>([])
const choices = ref<MenuBarFeedChoice[]>([])
const maxFeeds = ref(3)
const maxItemLimit = ref(10)
const defaultItemLimit = ref(3)
const loaded = ref(false)
const error = ref('')

async function load(): Promise<void> {
  error.value = ''
  try {
    const [limits, current, available] = await Promise.all([Limits(), Pins(), FeedChoices()])
    maxFeeds.value = limits.maxFeeds
    maxItemLimit.value = limits.maxItemLimit
    defaultItemLimit.value = limits.defaultItemLimit
    pins.value = current ?? []
    choices.value = available ?? []
  } catch (err) {
    error.value = errorText(err, 'Could not load the menu bar settings.')
  } finally {
    loaded.value = true
  }
}

// A failed save rolls back only if no newer edit has landed since.
let saveVersion = 0

async function save(next: MenuBarPin[]): Promise<void> {
  const previous = pins.value
  const version = ++saveVersion
  pins.value = next
  error.value = ''
  try {
    await SetPins(next)
  } catch (err) {
    if (saveVersion === version) pins.value = previous
    error.value = errorText(err, 'Could not save the menu bar pins.')
  }
}

function feedPath(choice: MenuBarFeedChoice, withName = true): string {
  return [choice.profileName, choice.folder, withName ? choice.name : ''].filter((part) => part !== '').join(' › ')
}

const choiceByFeed = computed(() => new Map(choices.value.map((choice) => [choice.feed, choice])))

const addOptions = computed(() => {
  const pinned = new Set(pins.value.map((pin) => pin.feed))
  return choices.value
    .filter((choice) => !pinned.has(choice.feed))
    .map((choice) => ({ value: choice.feed, label: feedPath(choice) }))
})

const limitOptions = computed(() =>
  Array.from({ length: maxItemLimit.value }, (_, index) => {
    const count = index + 1
    return { value: String(count), label: count === 1 ? '1 item' : `${count} items` }
  }),
)

function addPin(feed: string): void {
  if (!feed || pins.value.length >= maxFeeds.value) return
  void save([...pins.value, { feed, limit: defaultItemLimit.value }])
}

function removePin(index: number): void {
  void save(pins.value.filter((_, i) => i !== index))
}

function movePin(index: number, offset: -1 | 1): void {
  const target = index + offset
  if (target < 0 || target >= pins.value.length) return
  const next = [...pins.value]
  ;[next[index], next[target]] = [next[target], next[index]]
  void save(next)
}

function setLimit(index: number, value: string): void {
  void save(pins.value.map((pin, i) => (i === index ? { ...pin, limit: Number(value) } : pin)))
}

onMounted(() => {
  void load()
})
</script>

<template>
  <SettingsPage testid="menubar-settings">
    <InlineError v-if="error" :message="error" testid="menubar-settings-error" />

    <SettingsSection
      title="Pinned feeds"
      :description="`Up to ${maxFeeds} feeds listed in the menu bar dropdown, top first.`"
      boxed
    >
      <div v-if="loaded && pins.length === 0" class="px-4 py-3.5 text-xs text-text-3" data-testid="menubar-empty">
        No feeds are pinned. The menu bar links here until you pin one.
      </div>
      <SettingsRow
        v-for="(pin, index) in pins"
        :key="pin.feed"
        :label="choiceByFeed.get(pin.feed)?.name ?? pin.feed"
        :hint="
          choiceByFeed.has(pin.feed)
            ? feedPath(choiceByFeed.get(pin.feed)!, false)
            : 'This feed no longer exists, so the menu bar skips it.'
        "
        :testid="`menubar-pin-${index}`"
      >
        <div class="flex items-center gap-1.5">
          <AppSelect
            class="w-[110px]"
            size="sm"
            :model-value="String(pin.limit)"
            :options="limitOptions"
            aria-label="Items shown"
            :testid="`menubar-pin-${index}-limit`"
            @update:model-value="(value) => setLimit(index, value)"
          />
          <button
            type="button"
            class="cursor-pointer rounded-md p-1.5 text-text-3 hover:bg-chip hover:text-text disabled:cursor-not-allowed disabled:opacity-40"
            :disabled="index === 0"
            aria-label="Move up"
            :data-testid="`menubar-pin-${index}-up`"
            @click="movePin(index, -1)"
          >
            <IconArrowUp class="size-3.5" />
          </button>
          <button
            type="button"
            class="cursor-pointer rounded-md p-1.5 text-text-3 hover:bg-chip hover:text-text disabled:cursor-not-allowed disabled:opacity-40"
            :disabled="index === pins.length - 1"
            aria-label="Move down"
            :data-testid="`menubar-pin-${index}-down`"
            @click="movePin(index, 1)"
          >
            <IconArrowDown class="size-3.5" />
          </button>
          <button
            type="button"
            class="cursor-pointer rounded-md p-1.5 text-text-3 hover:bg-chip hover:text-text"
            aria-label="Unpin"
            :data-testid="`menubar-pin-${index}-remove`"
            @click="removePin(index)"
          >
            <IconX class="size-3.5" />
          </button>
        </div>
      </SettingsRow>
      <SettingsRow
        v-if="loaded && pins.length < maxFeeds"
        label="Pin a feed"
        hint="Items open in the browser or in Hive, and run any action that needs no further input."
      >
        <AppSelect
          class="w-[260px]"
          model-value=""
          :options="addOptions"
          placeholder="Choose a feed…"
          searchable
          :disabled="addOptions.length === 0"
          aria-label="Pin a feed"
          testid="menubar-add"
          @update:model-value="addPin"
        />
      </SettingsRow>
    </SettingsSection>
  </SettingsPage>
</template>
