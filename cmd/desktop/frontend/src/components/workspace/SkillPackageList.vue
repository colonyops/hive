<script setup lang="ts">
// A workspace enables packages, not skills (ADR
// skill-packages-are-the-unit-a-workspace-enables): skills.yml defines each
// package as glob patterns, and the rows show what those patterns select now.
import { computed, onMounted, ref } from 'vue'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconFolderOpen from '~icons/lucide/folder-open'
import IconPencil from '~icons/lucide/pencil'
import AppSwitch from '../ui/AppSwitch.vue'
import BaseBadge from '../ui/BaseBadge.vue'
import EmptyState from '../ui/EmptyState.vue'
import InlineError from '../ui/InlineError.vue'
import SettingsSection from '../settings/SettingsSection.vue'
import ListFooterButton from './ListFooterButton.vue'
import { useAgentWorkspaces } from '../../stores/useAgentWorkspaces'
import type { SkillPackageMember } from '../../lib/agentWorkspacesClient'

defineProps<{ busy?: boolean }>()
const selected = defineModel<string[]>({ required: true })

const {
  skillPackages,
  skillNames,
  skillPackagesProblem,
  reloadSkillPackages,
  revealSkillPackages,
  revealSharedSkills,
} = useAgentWorkspaces()

onMounted(() => void reloadSkillPackages())

interface PackageRow {
  name: string
  title: string
  description: string
  members: SkillPackageMember[]
  /** Why an enabled name does not resolve, when it does not. */
  warning: string
}

// An enabled name skills.yml no longer defines still rows, so it can be
// switched off rather than silently selecting nothing.
const rows = computed<PackageRow[]>(() => {
  const known = new Set(skillPackages.value.map((pkg) => pkg.name))
  const bySlug = new Map(skillNames.value.map((skill) => [skill.slug, skill]))
  return [
    ...skillPackages.value.map((pkg) => ({ ...pkg, title: pkg.title || pkg.name, warning: '' })),
    ...selected.value
      .filter((name) => !known.has(name))
      .map((name) => ({
        name,
        title: name,
        description: '',
        members: [],
        warning: missingSkillWarning(bySlug.get(name)?.selectedBy),
      })),
  ]
})

// A name skills.yml does not define is a package that never existed *or* a
// skill slug from a manifest written before packages were the enablement unit
// (hay-kot/hive-desktop#307). Only the second has a fix on screen, and saying
// "not defined in skills.yml" for both hides it.
function missingSkillWarning(selectedBy: string[] | undefined): string {
  if (!selectedBy) return 'not defined in skills.yml — an enabled package without a definition brings nothing'
  if (!selectedBy.length) return 'a skill, not a package — no package selects it yet, so define one in skills.yml'
  const packages = selectedBy.map((pkg) => `"${pkg}"`).join(' or ')
  return `a skill, not a package — the ${packages} package selects it, so enable that and switch this off`
}

function enabled(name: string): boolean {
  return selected.value.includes(name)
}

function toggle(name: string): void {
  selected.value = enabled(name) ? selected.value.filter((x) => x !== name) : [...selected.value, name]
}

// A package's members are the whole authority it grants, so they are worth
// seeing before enabling it, but not worth the height by default.
const expanded = ref(new Set<string>())

function toggleExpanded(name: string): void {
  if (!expanded.value.delete(name)) expanded.value.add(name)
}

const error = ref('')

async function reveal(open: () => Promise<void>, fallback: string): Promise<void> {
  error.value = ''
  try {
    await open()
  } catch (failure) {
    error.value = failure instanceof Error ? failure.message : fallback
  }
}
</script>

<template>
  <SettingsSection
    title="Skill packages"
    description="A package is glob patterns over skill names in skills.yml. Names come from the skills Hive ships and the SKILL.md files under .shared/skills."
    testid="agent-workspace-editor-skills"
  >
    <div class="divide-y divide-row overflow-hidden rounded-[11px] border border-card bg-raised">
      <div v-for="row in rows" :key="row.name">
        <div class="flex items-start gap-3 px-4 py-3.5">
          <AppSwitch
            class="mt-0.5"
            :model-value="enabled(row.name)"
            :aria-label="`Enable ${row.title}`"
            :disabled="busy"
            :testid="`agent-workspace-editor-skill-${row.name}`"
            @update:model-value="toggle(row.name)"
          />
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-2">
              <span
                class="truncate text-[13.5px] font-semibold"
                :class="enabled(row.name) ? 'text-text' : 'text-text-2'"
                >{{ row.title }}</span
              >
              <button
                v-if="!row.warning"
                type="button"
                class="flex shrink-0 cursor-pointer items-center gap-1 font-mono text-[11px] text-text-4 hover:text-text-2"
                :aria-expanded="expanded.has(row.name)"
                :data-testid="`agent-workspace-editor-skill-members-${row.name}`"
                @click="toggleExpanded(row.name)"
              >
                {{ row.members.length }} {{ row.members.length === 1 ? 'skill' : 'skills' }}
                <IconChevronDown
                  class="size-3 transition-transform"
                  :class="{ '-rotate-90': !expanded.has(row.name) }"
                />
              </button>
            </div>
            <div v-if="row.description" class="mt-1 text-[12px] leading-relaxed text-text-3">
              {{ row.description }}
            </div>
            <div v-if="!row.warning && !row.members.length" class="mt-1 text-[11.5px] text-severity-warning">
              matches no skill — check its patterns in skills.yml
            </div>
            <div v-if="row.warning" class="mt-1 text-[11.5px] text-severity-warning">{{ row.warning }}</div>
          </div>
        </div>
        <ul
          v-if="expanded.has(row.name) && row.members.length"
          class="flex flex-col gap-1 border-t border-row pb-3 pl-[58px] pr-4 pt-2.5"
        >
          <li v-for="member in row.members" :key="member.slug" class="flex items-center gap-2">
            <span class="truncate font-mono text-[11.5px] text-text-3">{{ member.slug }}</span>
            <BaseBadge tone="muted" variant="pill" class="shrink-0 px-2 py-0.5 text-[10.5px] font-medium">{{
              member.shipped ? 'shipped' : 'custom'
            }}</BaseBadge>
          </li>
        </ul>
      </div>
      <EmptyState v-if="!rows.length" variant="inline" class="px-4 py-3.5" message="No packages are defined yet." />
      <div class="flex divide-x divide-row">
        <ListFooterButton
          :disabled="busy"
          data-testid="agent-workspace-editor-skills-packages"
          @click="reveal(revealSkillPackages, 'skills.yml could not be opened.')"
        >
          <IconPencil class="size-3.5" />Edit skills.yml…
        </ListFooterButton>
        <ListFooterButton
          :disabled="busy"
          data-testid="agent-workspace-editor-skills-shared"
          @click="reveal(revealSharedSkills, 'The skills folder could not be opened.')"
        >
          <IconFolderOpen class="size-3.5" />Open the skills folder…
        </ListFooterButton>
      </div>
    </div>
    <InlineError
      v-if="skillPackagesProblem"
      testid="agent-workspace-editor-skills-problem"
      variant="line"
      :message="skillPackagesProblem"
    />
    <InlineError v-if="error" testid="agent-workspace-editor-skill-error" variant="line" :message="error" />
  </SettingsSection>
</template>
