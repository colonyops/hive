<script setup lang="ts">
import { computed, ref, watch, type Component } from 'vue'
import IconBox from '~icons/lucide/box'
import IconBraces from '~icons/lucide/braces'
import IconCode from '~icons/lucide/code'
import IconPackage from '~icons/lucide/package'
import IconSigma from '~icons/lucide/sigma'
import { useClipboard } from '../../composables/useClipboard'
import { callKind, type CallData, type CallKind } from '../../lib/fenceData'
import FenceCard from './FenceCard.vue'
import FenceTree from './FenceTree.vue'
import type { FenceProps } from './registry'

// eslint-disable-next-line vue/no-unused-properties -- ctx is part of the FenceProps contract
const props = defineProps<FenceProps<CallData>>()

interface CallItem {
  id: string
  call?: CallData
  parent: string | null
  /** A "… N more" row standing in for calls the agent left out. */
  elided?: number
  children: CallItem[]
}

const kindIcons: Record<CallKind, Component> = {
  function: IconBraces,
  method: IconCode,
  component: IconBox,
  computed: IconSigma,
  external: IconPackage,
}

const nodes = computed(() => {
  const byId = new Map<string, CallItem>()
  const focus: { id: string | null } = { id: null }
  const build = (call: CallData, id: string, parent: string | null): CallItem => {
    const item: CallItem = { id, call, parent, children: [] }
    byId.set(id, item)
    if (call.focus && focus.id === null) focus.id = id
    call.calls?.forEach((child, index) => item.children.push(build(child, `${id}.${index}`, id)))
    if (call.more) {
      const elidedId = `${id}.more`
      const row: CallItem = { id: elidedId, parent: id, elided: call.more, children: [] }
      byId.set(elidedId, row)
      item.children.push(row)
    }
    return item
  }
  const root = build(props.data, '0', null)
  return { root, byId, focus: focus.id ?? root.id }
})

const selected = ref<string | null>(null)
watch(nodes, (next) => (selected.value = next.focus), { immediate: true })
const current = computed(() => (selected.value ? nodes.value.byId.get(selected.value) : undefined))

const total = computed(() => [...nodes.value.byId.values()].filter((item) => item.call).length)

function pathFromRoot(id: string): CallItem[] {
  const path: CallItem[] = []
  for (let item = nodes.value.byId.get(id); item; item = item.parent ? nodes.value.byId.get(item.parent) : undefined) {
    path.unshift(item)
  }
  return path
}

const trail = computed(() => (selected.value ? pathFromRoot(selected.value) : []))
const calledBy = computed(() => (current.value?.parent ? nodes.value.byId.get(current.value.parent) : undefined))
const callees = computed(() => current.value?.children.filter((child) => child.call) ?? [])

const explanation = computed(() => {
  const call = current.value?.call
  if (!call) return ''
  return call.explain ?? call.note ?? ''
})

function atPath(at: string): string {
  return at.replace(/:\d+(?::\d+)?$/, '')
}

function atLabel(at: string): string {
  const path = atPath(at)
  const line = at.slice(path.length)
  return `${path.slice(path.lastIndexOf('/') + 1)}${line}`
}

// Every call in one file gets the same hue, so the chips group by file at a glance.
function fileColor(at: string): string {
  const path = atPath(at)
  let hash = 0
  for (const char of path) hash = (hash * 31 + char.codePointAt(0)!) % 360
  return `hsl(${hash} 55% 62%)`
}

const { copy } = useClipboard()
</script>

<template>
  <FenceCard
    kind="Calls"
    :title="data.name"
    :summary="`${total} ${total === 1 ? 'call' : 'calls'}`"
    :source="source"
    lang="hive-calls"
    :testid="testid"
  >
    <FenceTree
      v-model:selected="selected"
      :items="[nodes.root]"
      selectable
      label="Call tree"
      :testid="`${testid}-tree`"
    >
      <template #row="{ item, selected: isSelected }">
        <template v-if="item.call">
          <span class="calls-kind" :class="callKind(item.call)" aria-hidden="true">
            <component :is="kindIcons[callKind(item.call)]" />
          </span>
          <span class="calls-sym" :class="{ 'is-selected': isSelected }">{{ item.call.name }}</span>
          <span v-if="item.call.note" class="calls-note">{{ item.call.note }}</span>
          <button
            v-if="item.call.at"
            type="button"
            class="calls-chip is-pushed"
            :style="{ '--file': fileColor(item.call.at) }"
            :title="item.call.at"
            :data-testid="`${testid}-loc-${item.id}`"
            @click.stop="copy(item.call.at)"
          >
            {{ atLabel(item.call.at) }}
          </button>
        </template>
        <span v-else class="calls-sym is-elided">&hellip; {{ item.elided }} more</span>
      </template>
    </FenceTree>

    <div class="calls-strip" :data-testid="`${testid}-strip`">
      <template v-if="current">
        <div class="calls-strip-label">Selected <span>&uarr; &darr; to step through</span></div>
        <template v-if="current.call">
          <div class="calls-strip-head">
            <span class="calls-kind" :class="callKind(current.call)" aria-hidden="true">
              <component :is="kindIcons[callKind(current.call)]" />
            </span>
            <span class="calls-sym calls-strip-name" :data-testid="`${testid}-strip-name`">{{
              current.call.name
            }}</span>
            <span
              v-if="current.call.at"
              class="calls-chip"
              :style="{ '--file': fileColor(current.call.at) }"
              :data-testid="`${testid}-strip-loc`"
              >{{ atLabel(current.call.at) }}</span
            >
          </div>
          <div class="calls-path" :data-testid="`${testid}-strip-path`">
            <template v-for="(step, index) in trail" :key="step.id">
              <span v-if="index" aria-hidden="true">&rsaquo;</span>
              <b v-if="step.id === current.id">{{ step.call?.name }}</b>
              <button v-else type="button" :data-testid="`${testid}-path-${step.id}`" @click="selected = step.id">
                {{ step.call?.name }}
              </button>
            </template>
          </div>
          <p v-if="explanation" class="calls-explain" :data-testid="`${testid}-strip-explain`">{{ explanation }}</p>
          <div class="calls-rel">
            <span>called by</span>
            <button
              v-if="calledBy?.call"
              type="button"
              class="calls-rel-chip"
              :data-testid="`${testid}-caller-${calledBy.id}`"
              @click="selected = calledBy.id"
            >
              <span class="calls-kind" :class="callKind(calledBy.call)" aria-hidden="true">
                <component :is="kindIcons[callKind(calledBy.call)]" />
              </span>
              {{ calledBy.call.name }}
            </button>
            <i v-else>none</i>
            <span class="calls-rel-sep" aria-hidden="true" />
            <span>calls</span>
            <button
              v-for="callee in callees"
              :key="callee.id"
              type="button"
              class="calls-rel-chip"
              :data-testid="`${testid}-callee-${callee.id}`"
              @click="selected = callee.id"
            >
              <span class="calls-kind" :class="callKind(callee.call!)" aria-hidden="true">
                <component :is="kindIcons[callKind(callee.call!)]" />
              </span>
              {{ callee.call!.name }}
            </button>
            <i v-if="!callees.length">none</i>
          </div>
        </template>
        <p v-else class="calls-explain" :data-testid="`${testid}-strip-explain`">
          {{ current.elided }} more {{ current.elided === 1 ? 'call' : 'calls' }} not shown.
        </p>
      </template>
    </div>
  </FenceCard>
</template>

<style scoped>
.calls-kind {
  display: grid;
  flex: none;
  place-items: center;
  width: 18px;
  height: 18px;
  border-radius: 5px;
}
.calls-kind svg {
  width: 12px;
  height: 12px;
}
.calls-kind.function {
  background: var(--color-node-purple-tint);
  color: var(--color-node-purple);
}
.calls-kind.method {
  background: var(--color-node-orange-tint);
  color: var(--color-node-orange);
}
.calls-kind.component {
  background: var(--color-node-teal-tint);
  color: var(--color-node-teal);
}
.calls-kind.computed {
  background: var(--color-node-blue-tint);
  color: var(--color-node-blue);
}
.calls-kind.external {
  background: var(--color-chip);
  color: var(--color-text-3);
}
.calls-sym {
  color: var(--color-text);
  font: 13px/1.5 var(--font-mono);
  white-space: nowrap;
}
.calls-sym.is-selected {
  color: var(--color-accent);
}
.calls-sym.is-elided {
  color: var(--color-text-4);
}
.calls-note {
  min-width: 0;
  overflow: hidden;
  color: var(--color-text-3);
  font-size: 12.5px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.calls-chip {
  display: inline-flex;
  flex: none;
  align-items: center;
  gap: 6px;
  padding: 1px 7px;
  border: 1px solid var(--color-border);
  border-radius: 4px;
  background: var(--color-pane);
  color: var(--color-text-3);
  font: 11px/1.6 var(--font-mono);
}
.calls-chip::before {
  width: 7px;
  height: 7px;
  border-radius: 2px;
  background: var(--file);
  content: '';
}
button.calls-chip {
  cursor: pointer;
}
button.calls-chip:hover {
  border-color: var(--color-strong);
  color: var(--color-text);
}
.is-pushed {
  margin-left: auto;
}
.calls-strip {
  min-height: 132px;
  padding: 12px 14px 10px;
  border-top: 1px solid var(--color-border);
}
.calls-strip-label {
  display: flex;
  gap: 10px;
  margin-bottom: 6px;
  color: var(--color-text-4);
  font: 600 10px/1 var(--font-mono);
  letter-spacing: 0.1em;
  text-transform: uppercase;
}
.calls-strip-label span {
  font-weight: 400;
  letter-spacing: 0;
  text-transform: none;
}
.calls-strip-head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
}
.calls-strip-name {
  font-size: 14px;
}
.calls-path {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 2px 4px;
  margin-bottom: 8px;
  color: var(--color-text-4);
  font: 11px/1.5 var(--font-mono);
}
.calls-path button {
  padding: 0 3px;
  border: 0;
  border-radius: 3px;
  background: transparent;
  color: var(--color-text-3);
  cursor: pointer;
  font: inherit;
}
.calls-path button:hover {
  background: var(--color-chip);
  color: var(--color-text);
}
.calls-path b {
  padding: 0 3px;
  color: var(--color-text);
  font-weight: 500;
}
.calls-explain {
  margin: 0 0 9px;
  color: var(--color-text-2);
  font-size: 13px;
  line-height: 1.55;
}
.calls-rel {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px 8px;
  color: var(--color-text-4);
  font-size: 12px;
}
.calls-rel i {
  font-style: italic;
}
.calls-rel-sep {
  width: 1px;
  height: 14px;
  margin: 0 4px;
  background: var(--color-border);
}
.calls-rel-chip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 1px 7px 1px 3px;
  border: 1px solid var(--color-border);
  border-radius: 5px;
  background: var(--color-pane);
  color: var(--color-text-2);
  cursor: pointer;
  font: 11.5px/1.6 var(--font-mono);
}
.calls-rel-chip:hover {
  border-color: var(--color-strong);
  color: var(--color-text);
}
.calls-rel-chip .calls-kind {
  width: 15px;
  height: 15px;
  border-radius: 4px;
}
.calls-rel-chip .calls-kind svg {
  width: 10px;
  height: 10px;
}
</style>
