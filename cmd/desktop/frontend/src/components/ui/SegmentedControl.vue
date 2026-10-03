<script setup lang="ts" generic="T extends string">
import { shallowRef } from 'vue'
import { useTooltip } from '../../composables/useTooltip'
import FormField from './FormField.vue'
import TooltipBubble from './TooltipBubble.vue'

export interface SegmentedControlOption<V extends string = string> {
  value: V
  label: string
  /** Spelled-out name when the label is abbreviated or hidden; it also names the segment for screen readers. */
  title?: string
}

const props = withDefaults(
  defineProps<{
    modelValue?: T
    options: SegmentedControlOption<T>[]
    /**
     * `field` is a settings control with an optional label and hint above
     * and below. `compact` is the bare strip a toolbar, a title bar, or a
     * dialog header carries.
     */
    variant?: 'field' | 'compact'
    /** `compact` only: `sm` for the title bar and dialog headers, `md` for view toolbars. */
    size?: 'sm' | 'md'
    label?: string
    hint?: string
    testid?: string
    /** Names the strip when a SettingsRow supplies the visible label instead. */
    ariaLabel?: string
    /** Lay the options out as a grid with this many columns instead of a single strip. */
    columns?: number
  }>(),
  {
    variant: 'field',
    size: 'md',
    label: undefined,
    hint: undefined,
    testid: undefined,
    ariaLabel: undefined,
    columns: undefined,
  },
)

const emit = defineEmits<{ 'update:modelValue': [value: T] }>()

defineSlots<{
  option?: (props: { option: SegmentedControlOption<T>; selected: boolean }) => unknown
}>()

// One tooltip for the strip, carrying the title of the option under the pointer.
const tipOption = shallowRef<SegmentedControlOption<T> | null>(null)
const tip = useTooltip(() => tipOption.value?.title ?? '')

function tipTriggers(option: SegmentedControlOption<T>) {
  return {
    ...tip.triggers,
    pointerenter: (event: PointerEvent) => {
      tipOption.value = option
      tip.triggers.pointerenter(event)
    },
    focusin: (event: FocusEvent) => {
      tipOption.value = option
      tip.triggers.focusin(event)
    },
  }
}

function optionClass(selected: boolean): string[] {
  if (props.variant === 'field') {
    return [
      'flex-1 whitespace-nowrap rounded-md px-2.5 py-1.5 text-small',
      selected ? 'bg-raised text-text' : 'text-text-3 hover:text-text-2',
    ]
  }
  return [
    'flex items-center gap-1.5 rounded-md px-2.5 font-medium',
    props.size === 'sm' ? 'h-[22px] text-caption' : 'h-[26px] text-small',
    selected ? 'bg-chip text-text' : 'text-text-2 hover:text-text',
  ]
}
</script>

<template>
  <component :is="variant === 'field' ? FormField : 'div'" v-bind="variant === 'field' ? { label, hint } : {}">
    <div
      :class="[
        props.columns ? 'grid' : 'flex',
        variant === 'field'
          ? 'gap-1 rounded-lg border border-row bg-app p-1'
          : 'gap-0.5 rounded-lg border border-strong bg-app p-0.5',
      ]"
      :style="props.columns ? { gridTemplateColumns: `repeat(${props.columns}, minmax(0, 1fr))` } : undefined"
      role="group"
      :aria-label="props.ariaLabel ?? props.label"
      :data-testid="variant === 'compact' ? testid : undefined"
    >
      <button
        v-for="opt in options"
        :key="opt.value"
        type="button"
        class="cursor-pointer transition-colors"
        :class="optionClass(modelValue === opt.value)"
        :aria-pressed="modelValue === opt.value"
        :aria-label="opt.title"
        :data-testid="testid ? `${testid}-${opt.value}` : undefined"
        v-on="tipTriggers(opt)"
        @click="emit('update:modelValue', opt.value)"
      >
        <slot name="option" :option="opt" :selected="modelValue === opt.value">{{ opt.label }}</slot>
      </button>
      <TooltipBubble
        v-if="tip.anchor.value"
        :anchor="tip.anchor.value"
        :text="tipOption?.title ?? ''"
        @dismiss="tip.hide"
      />
    </div>
  </component>
</template>
