<script setup lang="ts">
import { computed } from 'vue'
import { useIntegrations } from '../../composables/useIntegrations'
import SelectField from './SelectField.vue'
import TextField from './TextField.vue'

type Provider = 'github' | 'gitea' | 'grafana' | 'posthog'

const PROVIDERS: Record<Provider, { label: string; name: string; placeholder: string }> = {
  github: { label: 'Account', name: 'GitHub account', placeholder: 'github/octocat' },
  gitea: { label: 'Account', name: 'Gitea instance', placeholder: 'gitea/git.example.com-octocat' },
  grafana: { label: 'Stack', name: 'Grafana stack', placeholder: 'grafana/grafana.example.com-1' },
  posthog: { label: 'Project', name: 'PostHog project', placeholder: 'posthog/us.posthog.com-1' },
}

const props = defineProps<{
  provider: Provider
  modelValue?: string
  hint?: string
  testid: string
}>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

const meta = computed(() => PROVIDERS[props.provider])

const { credentialRefsFor, loaded } = useIntegrations()
const connectedRefs = computed(() => credentialRefsFor(props.provider))

const options = computed(() => {
  const options = connectedRefs.value.map((ref) => ({ value: ref, label: ref }))
  // Dropping a since-disconnected credential would silently rewrite the
  // node's config on the next edit, so it stays selectable and says why.
  const current = props.modelValue
  if (current && !connectedRefs.value.includes(current)) {
    options.unshift({ value: current, label: `${current} — not connected` })
  }
  return options
})

const hintText = computed(() => {
  if (!loaded.value) return `Loading connected ${meta.value.label.toLowerCase()}s…`
  if (connectedRefs.value.length === 0)
    return `No ${meta.value.name} is connected. Connect one in Settings ▸ Integrations.`
  return props.hint ?? `The connected ${meta.value.name} to fetch as.`
})
</script>

<template>
  <!-- With nothing connected there is no valid choice to offer, so the field
       stays a text input rather than an empty dropdown. -->
  <SelectField
    v-if="options.length > 0"
    :label="meta.label"
    :model-value="modelValue ?? ''"
    :options="options"
    :hint="hintText"
    :testid="testid"
    @update:model-value="emit('update:modelValue', $event)"
  />
  <TextField
    v-else
    :label="meta.label"
    :model-value="modelValue ?? ''"
    :placeholder="meta.placeholder"
    :hint="hintText"
    monospace
    :testid="testid"
    @update:model-value="emit('update:modelValue', $event)"
  />
</template>
