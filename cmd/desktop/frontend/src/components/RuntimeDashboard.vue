<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import IconChevronRight from '~icons/lucide/chevron-right'
import { startFrameStats, stopFrameStats, useFrameStats } from '../composables/useFrameStats'
import { useRuntimeStats } from '../composables/useRuntimeStats'
import { formatBytes, formatBytesParts } from '../lib/bytes'
import SettingsSection from './settings/SettingsSection.vue'
import SparkLine from './SparkLine.vue'

const { stats, rssHistory, cpuHistory, error, start, stop } = useRuntimeStats()
const { stats: frames } = useFrameStats()

const processRows = computed(() => {
  const sample = stats.value
  if (!sample) return []
  return [{ ...sample.process, self: true }, ...(sample.children ?? []).map((child) => ({ ...child, self: false }))]
})

function plural(count: number, noun: string): string {
  return `${count} ${noun}${count === 1 ? '' : 's'}`
}

const uptime = computed(() => {
  const ms = stats.value?.uptimeMs ?? 0
  const minutes = Math.floor(ms / 60_000)
  if (minutes < 60) return `${minutes}m`
  const hours = Math.floor(minutes / 60)
  return hours < 24 ? `${hours}h ${minutes % 60}m` : `${Math.floor(hours / 24)}d ${hours % 24}h`
})

const peakRSS = computed(() => (rssHistory.value.length ? Math.max(...rssHistory.value) : 0))
const peakCPU = computed(() => (cpuHistory.value.length ? Math.max(...cpuHistory.value) : 0))

const heapPressure = computed(() => {
  const go = stats.value?.go
  if (!go?.nextGcBytes) return 0
  return Math.min(100, Math.round((go.heapAllocBytes / go.nextGcBytes) * 100))
})

const vitals = computed(() => {
  const sample = stats.value
  if (!sample) return []
  return [
    { label: 'Goroutines', value: `${sample.go.goroutines}` },
    { label: 'OS threads', value: `${sample.process.threads}` },
    { label: 'Processes', value: `${processRows.value.length}` },
    { label: 'Cores', value: `${sample.go.gomaxprocs} of ${sample.go.numCpu}` },
    { label: 'Uptime', value: uptime.value },
    { label: 'UI lag', value: `${frames.value.lagMs.toFixed(0)} / ${frames.value.worstLagMs.toFixed(0)}ms` },
  ]
})

const goRows = computed(() => {
  const go = stats.value?.go
  if (!go) return []
  return [
    { label: 'Heap reserved', value: formatBytes(go.heapSysBytes), hint: `${go.heapObjects.toLocaleString()} objects` },
    { label: 'Stacks', value: formatBytes(go.stackSysBytes), hint: plural(go.goroutines, 'goroutine') },
    { label: 'Mapped by Go', value: formatBytes(go.totalSysBytes), hint: 'heap, stacks, runtime' },
    { label: 'Collections', value: `${go.gcCount}`, hint: `${go.lastPauseMs.toFixed(2)}ms last · ${go.totalPauseMs.toFixed(0)}ms total` },
  ]
})

onMounted(() => {
  startFrameStats()
  start()
})

onUnmounted(() => {
  stop()
  stopFrameStats()
})
</script>

<template>
  <SettingsSection
    title="Runtime"
    description="What this install is costing the machine right now. Process data is sampled every two seconds; UI frames are sampled while this page is open."
    testid="observability-runtime"
  >
    <p v-if="error" class="text-xs text-severity-error" data-testid="observability-runtime-error">{{ error }}</p>

    <div v-if="stats" class="flex flex-col gap-3">
      <div class="grid grid-cols-1 gap-3 @[440px]/pane:grid-cols-2 @[720px]/pane:grid-cols-3">
        <div class="flex flex-col overflow-hidden rounded-[11px] border border-card bg-raised" data-testid="observability-runtime-memory">
          <div class="flex flex-col gap-1.5 px-4 pb-3 pt-4">
            <div class="flex items-center justify-between gap-3">
              <span class="font-mono text-[10px] font-semibold uppercase tracking-[.14em] text-text-3">Memory</span>
              <span class="font-mono text-[11px] tabular-nums text-text-4">peak {{ formatBytes(peakRSS) }}</span>
            </div>
            <div class="font-mono text-[28px] font-semibold leading-none tabular-nums text-text">
              {{ formatBytesParts(stats.totalRssBytes).value }}
              <span class="text-[16px] font-medium text-text-3">{{ formatBytesParts(stats.totalRssBytes).unit }}</span>
            </div>
            <div class="text-[12px] text-text-3">resident</div>
          </div>
          <SparkLine :values="rssHistory" class="h-14 text-accent" />
        </div>
        <div class="flex flex-col overflow-hidden rounded-[11px] border border-card bg-raised" data-testid="observability-runtime-cpu">
          <div class="flex flex-col gap-1.5 px-4 pb-3 pt-4">
            <div class="flex items-center justify-between gap-3">
              <span class="font-mono text-[10px] font-semibold uppercase tracking-[.14em] text-text-3">CPU</span>
              <span class="font-mono text-[11px] tabular-nums text-text-4">peak {{ peakCPU.toFixed(1) }}%</span>
            </div>
            <div class="font-mono text-[28px] font-semibold leading-none tabular-nums text-text">
              {{ stats.totalCpuPercent.toFixed(1) }}<span class="text-[16px] font-medium text-text-3">%</span>
            </div>
            <div class="text-[12px] text-text-3">of one core</div>
          </div>
          <SparkLine :values="cpuHistory" class="h-14 text-severity-info" />
        </div>
        <div class="flex flex-col overflow-hidden rounded-[11px] border border-card bg-raised" data-testid="observability-runtime-frames">
          <div class="flex flex-col gap-1.5 px-4 pb-3 pt-4">
            <div class="flex items-center justify-between gap-3">
              <span class="font-mono text-[10px] font-semibold uppercase tracking-[.14em] text-text-3">Frames</span>
              <span class="font-mono text-[11px] tabular-nums text-text-4">worst {{ frames.worstFrameMs.toFixed(0) }}ms</span>
            </div>
            <div class="font-mono text-[28px] font-semibold leading-none tabular-nums text-text">
              {{ frames.fps.toFixed(0) }}<span class="text-[16px] font-medium text-text-3"> fps</span>
            </div>
            <div class="text-[12px] tabular-nums text-text-3">{{ frames.dropped }} dropped in 10s</div>
          </div>
          <SparkLine :values="frames.buckets" class="h-14 text-severity-warning" />
        </div>
      </div>

      <div class="grid grid-cols-2 overflow-hidden rounded-[11px] border border-card bg-raised @[440px]/pane:grid-cols-3 @[720px]/pane:grid-cols-6" data-testid="observability-runtime-vitals">
        <div v-for="vital in vitals" :key="vital.label" class="-ml-px -mt-px flex min-w-0 flex-col gap-1.5 border-l border-t border-border px-4 py-3.5">
          <div class="font-mono text-[10px] font-semibold uppercase tracking-[.12em] text-text-3">{{ vital.label }}</div>
          <div class="truncate font-mono text-[16px] tabular-nums text-text">{{ vital.value }}</div>
        </div>
      </div>

      <details class="group overflow-hidden rounded-[11px] border border-card bg-raised" data-testid="observability-runtime-details">
        <summary class="flex cursor-pointer list-none items-center gap-2 px-4 py-3 text-[12.5px] font-medium text-text-2 hover:text-text [&::-webkit-details-marker]:hidden">
          <IconChevronRight class="size-3.5 shrink-0 transition-transform group-open:rotate-90" />
          Go heap and process details
        </summary>
        <div class="border-t border-border" data-testid="observability-runtime-go">
          <div class="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1 px-4 py-3.5">
            <div class="flex items-baseline gap-2.5">
              <span class="font-mono text-[10px] font-semibold uppercase tracking-[.14em] text-text-3">Go heap</span>
              <span class="font-mono text-[16px] tabular-nums text-text">
                {{ formatBytes(stats.go.heapAllocBytes) }}
                <span class="text-text-4">of {{ formatBytes(stats.go.nextGcBytes) }}</span>
              </span>
            </div>
            <span class="text-[12px] tabular-nums text-text-3" data-testid="observability-runtime-heap-pressure">{{ heapPressure }}% of next GC target</span>
          </div>
          <div class="grid grid-cols-2 @[560px]/pane:grid-cols-4">
            <div v-for="row in goRows" :key="row.label" class="-ml-px -mt-px flex min-w-0 flex-col gap-1.5 border-l border-t border-border px-4 py-3.5">
              <div class="font-mono text-[10px] font-semibold uppercase tracking-[.12em] text-text-3">{{ row.label }}</div>
              <div class="truncate font-mono text-[15px] tabular-nums text-text">{{ row.value }}</div>
              <div class="truncate text-[11px] tabular-nums text-text-4">{{ row.hint }}</div>
            </div>
          </div>
        </div>

        <div class="border-t border-border" data-testid="observability-runtime-processes">
          <div class="flex items-center gap-3 border-b border-border px-4 py-2.5 font-mono text-[10px] font-semibold uppercase tracking-[.12em] text-text-3">
            <span class="min-w-0 flex-1">Process</span>
            <span class="w-20 text-right">Memory</span>
            <span class="w-14 text-right">CPU</span>
            <span class="hidden w-16 text-right @[560px]/pane:block">Threads</span>
          </div>
          <div v-for="row in processRows" :key="row.pid" class="flex items-center gap-3 border-b border-border px-4 py-2 text-[12.5px] text-text-2 last:border-b-0">
            <span class="min-w-0 flex-1 truncate" :class="row.self ? 'font-semibold text-text' : ''">
              {{ row.name || 'unknown' }}
              <span class="font-mono text-[11px] text-text-4">{{ row.pid }}</span>
            </span>
            <span class="w-20 text-right font-mono tabular-nums">{{ formatBytes(row.rssBytes) }}</span>
            <span class="w-14 text-right font-mono tabular-nums">{{ row.cpuPercent.toFixed(1) }}%</span>
            <span class="hidden w-16 text-right font-mono tabular-nums @[560px]/pane:block">{{ row.threads }}</span>
          </div>
          <p v-if="stats.childrenTruncated" class="border-t border-border px-4 py-2 text-[11px] text-text-4">
            Only the first processes in the tree are listed; the totals above are a floor.
          </p>
          <p v-else-if="processRows.length === 1" class="border-t border-border px-4 py-2 text-[11px] text-text-4">
            Nothing else is parented to Hive right now. Only processes Hive is the parent of are counted. A terminal's shell or an agent is included; the webview's rendering helpers are not because the OS starts and owns them.
          </p>
        </div>
      </details>
    </div>
    <p v-else-if="!error" class="text-xs text-text-4">Sampling…</p>
  </SettingsSection>
</template>
