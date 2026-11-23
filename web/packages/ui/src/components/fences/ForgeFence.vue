<script setup lang="ts">
import { computed } from 'vue'
import IconCircleDot from '~icons/lucide/circle-dot'
import IconGitPullRequest from '~icons/lucide/git-pull-request'
import IconMessageSquare from '~icons/lucide/message-square'
import type { ForgeData, ForgeItem } from '../../lib/fenceData'
import FenceCard from './FenceCard.vue'
import type { FenceProps } from './registry'

const props = defineProps<FenceProps<ForgeData>>()

const fenceHost = computed(() => props.data.host ?? 'github.com')

function defaultUrl(item: ForgeItem): string | undefined {
  const match = /^(.+)#(\d+)$/.exec(item.ref)
  if (!match) return undefined
  return `https://${fenceHost.value}/${match[1]}/${item.kind === 'pr' ? 'pull' : 'issues'}/${match[2]}`
}

function urlOf(item: ForgeItem): string | undefined {
  return item.url ?? defaultUrl(item)
}

function hostOf(item: ForgeItem): string {
  if (!item.url) return fenceHost.value
  try {
    return new URL(item.url).host
  } catch {
    return fenceHost.value
  }
}

const cards = computed(() =>
  props.data.items.map((item, index) => ({
    item,
    index,
    url: urlOf(item),
    host: hostOf(item) === fenceHost.value ? undefined : hostOf(item),
  })),
)

function checksTone(checks: string): string {
  const match = /^(\d+)\/(\d+)$/.exec(checks.trim())
  if (match && match[1] === match[2]) return 'is-ok'
  return /running|pending/i.test(checks) ? 'is-running' : ''
}

function diffParts(diff: string): { text: string; tone: string }[] {
  return diff
    .split(/\s+/)
    .filter(Boolean)
    .map((text) => ({ text, tone: text.startsWith('+') ? 'is-added' : text.startsWith('-') ? 'is-removed' : '' }))
}

// Label colors derive from the text, so one label looks the same on every card.
function labelColor(label: string): string {
  let hash = 0
  for (const char of label) hash = (hash * 31 + char.codePointAt(0)!) % 360
  return `hsl(${hash} 55% 62%)`
}
</script>

<template>
  <FenceCard
    kind="Forge"
    :title="`${data.items.length} ${data.items.length === 1 ? 'item' : 'items'}`"
    :summary="data.snapshot ? `snapshot ${data.snapshot}` : undefined"
    :source="source"
    lang="hive-forge"
    :testid="testid"
  >
    <ul class="forge-list">
      <li v-for="card in cards" :key="card.index" class="forge-row">
        <component
          :is="card.url ? 'button' : 'div'"
          :type="card.url ? 'button' : undefined"
          class="forge-item"
          :class="{ 'is-link': card.url }"
          :data-testid="`${testid}-item-${card.index}`"
          @click="card.url && ctx.follow(card.url)"
        >
          <span class="forge-icon" :class="card.item.state" aria-hidden="true">
            <component :is="card.item.kind === 'pr' ? IconGitPullRequest : IconCircleDot" />
          </span>
          <span class="forge-title">{{ card.item.title }}</span>
          <span class="forge-state" :class="card.item.state" :data-testid="`${testid}-state-${card.index}`">{{
            card.item.state
          }}</span>
          <span class="forge-sub">
            <span :data-testid="`${testid}-ref-${card.index}`">{{ card.item.ref }}</span>
            <span v-if="card.host" :data-testid="`${testid}-host-${card.index}`">{{ card.host }}</span>
            <span v-if="card.item.author">{{ card.item.author }}</span>
            <span v-if="card.item.checks" :class="checksTone(card.item.checks)">checks {{ card.item.checks }}</span>
            <span v-if="card.item.diff">
              <span v-for="part in diffParts(card.item.diff)" :key="part.text" :class="part.tone"
                >{{ part.text }}
              </span>
            </span>
            <span v-if="card.item.comments !== undefined" class="forge-comments">
              <IconMessageSquare aria-hidden="true" />{{ card.item.comments }}
            </span>
            <span
              v-for="label in card.item.labels"
              :key="label"
              class="forge-label"
              :style="{ '--label': labelColor(label) }"
              >{{ label }}</span
            >
            <span v-if="card.item.updated" class="forge-updated">{{ card.item.updated }}</span>
          </span>
        </component>
      </li>
    </ul>
  </FenceCard>
</template>

<style scoped>
.forge-list {
  margin: 0;
  padding: 0;
  list-style: none;
}
.forge-row + .forge-row {
  border-top: 1px solid var(--color-border);
}
.forge-item {
  display: grid;
  grid-template-columns: 28px minmax(0, 1fr) auto;
  align-items: center;
  gap: 3px 12px;
  width: 100%;
  padding: 10px 14px;
  border: 0;
  background: transparent;
  color: var(--color-text);
  font: inherit;
  text-align: left;
}
.forge-item.is-link {
  cursor: pointer;
}
.forge-item.is-link:hover {
  background: var(--color-row-hover);
}
.forge-item:focus-visible {
  outline: 2px solid var(--color-accent);
  outline-offset: -2px;
}
.forge-icon {
  display: grid;
  grid-row: span 2;
  place-items: center;
  width: 26px;
  height: 26px;
  border-radius: 50%;
}
.forge-icon svg {
  width: 14px;
  height: 14px;
}
.forge-icon.open,
.forge-state.open {
  background: var(--color-severity-success-tint);
  color: var(--color-severity-success);
}
.forge-icon.draft,
.forge-state.draft {
  background: var(--color-chip);
  color: var(--color-text-3);
}
.forge-icon.merged,
.forge-state.merged,
.forge-icon.closed,
.forge-state.closed {
  background: var(--color-node-purple-tint);
  color: var(--color-node-purple);
}
.forge-title {
  min-width: 0;
  overflow: hidden;
  font-size: 13.5px;
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.forge-state {
  justify-self: end;
  padding: 2px 8px;
  border-radius: 10px;
  font-size: 11.5px;
  text-transform: capitalize;
  white-space: nowrap;
}
.forge-sub {
  display: flex;
  flex-wrap: wrap;
  grid-column: 2 / 4;
  align-items: center;
  gap: 4px 14px;
  color: var(--color-text-3);
  font: 11.5px/1.5 var(--font-mono);
}
.forge-sub .is-ok,
.forge-sub .is-added {
  color: var(--color-severity-success);
}
.forge-sub .is-removed {
  color: var(--color-severity-error);
}
.forge-sub .is-running {
  color: var(--color-accent);
}
.forge-comments {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
.forge-comments svg {
  width: 12px;
  height: 12px;
}
.forge-label {
  padding: 0 7px;
  border: 1px solid var(--label);
  border-radius: 9px;
  color: var(--label);
  font: 11px/1.55 var(--font-sans);
}
.forge-updated {
  margin-left: auto;
  color: var(--color-text-4);
}
</style>
