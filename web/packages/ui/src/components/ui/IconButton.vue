<script setup lang="ts">
import type { Component } from 'vue'
import { useTooltip } from '../../composables/useTooltip'
import Spinner from './Spinner.vue'
import TooltipBubble from './TooltipBubble.vue'

const props = withDefaults(
  defineProps<{
    /** The accessible name, and the tooltip unless `tooltip` is set. */
    label: string
    icon: Component
    /** Tooltip text when it says more than the accessible name should. */
    tooltip?: string
    size?: 'sm' | 'md' | 'lg' | 'xl'
    variant?: 'ghost' | 'outline'
    tone?: 'default' | 'danger'
    /** A pressed look: the open menu of a toggle, the current page of a nav button. */
    active?: boolean
    busy?: boolean
    disabled?: boolean
  }>(),
  { tooltip: undefined, size: 'md', variant: 'ghost', tone: 'default' },
)

const { anchor, hide, triggers } = useTooltip(() => props.tooltip ?? props.label)

const SIZES = {
  sm: 'size-4.5 rounded-md',
  md: 'size-6 rounded-lg',
  lg: 'size-7 rounded-lg',
  xl: 'size-8.5 rounded-lg',
}
const ICON_SIZES = { sm: 'size-3', md: 'size-3.5', lg: 'size-3.5', xl: 'size-4' }
</script>

<template>
  <button
    type="button"
    :aria-label="label"
    :disabled="disabled || busy"
    :aria-busy="busy || undefined"
    class="inline-flex shrink-0 cursor-pointer items-center justify-center focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-accent disabled:cursor-default disabled:not-aria-busy:opacity-40"
    :class="[
      SIZES[size],
      variant === 'outline' && 'border border-card enabled:hover:border-strong',
      active
        ? 'bg-accent-tint text-accent'
        : [
            'text-text-3',
            variant === 'ghost' && 'enabled:hover:bg-chip',
            tone === 'danger' ? 'enabled:hover:text-severity-error' : 'enabled:hover:text-text',
          ],
    ]"
    v-on="triggers"
  >
    <Spinner v-if="busy" />
    <component :is="icon" v-else :class="ICON_SIZES[size]" />
    <slot />
    <TooltipBubble v-if="anchor" :anchor="anchor" :text="tooltip ?? label" @dismiss="hide" />
  </button>
</template>
