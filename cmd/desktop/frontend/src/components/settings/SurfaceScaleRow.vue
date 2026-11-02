<script setup lang="ts">
import SettingsRow from './SettingsRow.vue'
import SettingsStepper from './SettingsStepper.vue'
import { nextScale, surfaceScaleRanges, useSurfaceScale, type Surface } from '../../stores/useSurfaceScale'

defineProps<{
  surface: Surface
  label: string
  hint: string
  testid: string
}>()

const { scales, setScale } = useSurfaceScale()
</script>

<template>
  <SettingsRow :label="label" :hint="hint" :testid="testid">
    <SettingsStepper
      :model-value="scales[surface].value"
      :display="`${scales[surface].value}%`"
      :min="surfaceScaleRanges[surface].min"
      :max="surfaceScaleRanges[surface].max"
      :next="(value, direction) => nextScale(surface, value, direction)"
      :aria-label="`${label} text size`"
      :testid="`${testid}-stepper`"
      @update:model-value="setScale(surface, $event)"
    />
  </SettingsRow>
</template>
