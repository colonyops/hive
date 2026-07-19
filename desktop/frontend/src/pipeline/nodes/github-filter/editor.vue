<script setup lang="ts">
// Ports the FeedEditorSheet filter UI (8 glob/checkbox groups) onto the
// field kit, as a controlled component over Config (see config.ts for the
// shape and the matches()/glob semantics this only edits, never evaluates).
import { GlobListField, ToggleField } from '../../fields'
import type { Config } from './config'

const props = defineProps<{ config: Config; errors?: string[] }>()
const emit = defineEmits<{ 'update:config': [config: Config] }>()

type GlobKey = 'repos' | 'exclude_repos' | 'authors' | 'exclude_authors' | 'labels' | 'exclude_labels'

const globGroups: Array<{ key: GlobKey; label: string; placeholder: string; testid: string }> = [
  { key: 'repos', label: 'Repos', placeholder: 'colonyops/*', testid: 'github-filter-editor-repos' },
  { key: 'exclude_repos', label: 'Exclude repos', placeholder: 'colonyops/sandbox', testid: 'github-filter-editor-exclude-repos' },
  { key: 'authors', label: 'Authors', placeholder: 'hay-kot', testid: 'github-filter-editor-authors' },
  { key: 'exclude_authors', label: 'Exclude authors', placeholder: '*[bot]', testid: 'github-filter-editor-exclude-authors' },
  { key: 'labels', label: 'Labels', placeholder: 'area/*', testid: 'github-filter-editor-labels' },
  { key: 'exclude_labels', label: 'Exclude labels', placeholder: 'wontfix', testid: 'github-filter-editor-exclude-labels' },
]

const allReasons = [
  'approval_requested', 'assign', 'author', 'ci_activity', 'comment', 'invitation', 'manual',
  'member_feature_requested', 'mention', 'review_requested', 'security_advisory_credit',
  'security_alert', 'state_change', 'subscribed', 'team_mention',
]

function setGlobGroup(key: GlobKey, value: string[]) {
  emit('update:config', { ...props.config, [key]: value.length > 0 ? value : undefined })
}

function typeChecked(value: string): boolean {
  return (props.config.types ?? []).includes(value)
}

function toggleType(value: string, checked: boolean) {
  const current = props.config.types ?? []
  const next = checked ? [...current, value] : current.filter((v) => v !== value)
  emit('update:config', { ...props.config, types: next.length > 0 ? next : undefined })
}

function reasonChecked(value: string): boolean {
  return (props.config.reasons ?? []).includes(value)
}

function toggleReason(value: string, checked: boolean) {
  const current = props.config.reasons ?? []
  const next = checked ? [...current, value] : current.filter((v) => v !== value)
  emit('update:config', { ...props.config, reasons: next.length > 0 ? next : undefined })
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <div class="grid grid-cols-2 gap-x-3.5 gap-y-3">
      <GlobListField
        v-for="group in globGroups"
        :key="group.key"
        :label="group.label"
        :model-value="config[group.key] ?? []"
        :placeholder="group.placeholder"
        :testid="group.testid"
        @update:model-value="(v) => setGlobGroup(group.key, v)"
      />
    </div>

    <div class="flex items-center gap-6 border-t border-row pt-4">
      <div class="text-[12.5px] text-text-2">Types</div>
      <ToggleField
        label="Pull requests"
        :model-value="typeChecked('pr')"
        testid="github-filter-editor-type-pr"
        @update:model-value="(v) => toggleType('pr', v)"
      />
      <ToggleField
        label="Issues"
        :model-value="typeChecked('issue')"
        testid="github-filter-editor-type-issue"
        @update:model-value="(v) => toggleType('issue', v)"
      />
    </div>

    <div>
      <div class="mb-2 text-[12.5px] text-text-2">Notification reasons</div>
      <div class="grid grid-cols-3 gap-x-3 gap-y-1.5" data-testid="github-filter-editor-reasons">
        <label v-for="reason in allReasons" :key="reason" class="flex cursor-pointer items-center gap-2 font-mono text-[11.5px] text-text-2">
          <input
            type="checkbox"
            :checked="reasonChecked(reason)"
            class="accent-accent"
            :data-testid="`github-filter-editor-reason-${reason}`"
            @change="(e) => toggleReason(reason, (e.target as HTMLInputElement).checked)"
          >{{ reason }}
        </label>
      </div>
    </div>
  </div>
</template>
