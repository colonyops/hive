<script setup lang="ts">
import BaseCard from './BaseCard.vue'

defineProps<{
  label: string
  value: string | number
  unit?: string
  hint?: string
  testid?: string
  valueTestid?: string
}>()
</script>

<template>
  <BaseCard :padded="false" class="overflow-hidden rounded-xl border border-card bg-raised" :data-testid="testid">
    <div class="flex min-w-0 flex-1 flex-col">
      <div class="flex flex-col gap-1.5 px-4 pb-3 pt-4">
        <div class="flex items-center justify-between gap-3">
          <span class="font-mono text-micro font-semibold uppercase tracking-[.14em] text-text-3">{{ label }}</span>
          <span v-if="$slots.detail" class="font-mono text-caption tabular-nums text-text-4">
            <slot name="detail" />
          </span>
        </div>
        <div
          class="font-mono text-display font-semibold leading-none tabular-nums text-text"
          :data-testid="valueTestid ?? (testid ? `${testid}-value` : undefined)"
        >
          {{ value }}<span v-if="unit" class="text-lead font-medium text-text-3">{{ unit }}</span>
        </div>
        <div v-if="hint" class="text-small tabular-nums text-text-3">{{ hint }}</div>
      </div>
      <slot />
    </div>
  </BaseCard>
</template>
