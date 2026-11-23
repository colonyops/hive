<script setup lang="ts">
// The comments column beside the document. Open threads on the text line up
// with their highlights; general, outdated, and resolved threads stack below.
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import { useAlignedCards } from '@hive/ui/composables/useAlignedCards'
import { computed, ref } from 'vue'
import type { RouteLocationRaw } from 'vue-router'
import IconX from '~icons/lucide/x'
import type { DocumentCollab } from '../../lib/collab'
import type { Thread, Viewer } from '../../types'
import CommentComposer from './CommentComposer.vue'
import ThreadCard from './ThreadCard.vue'

const props = defineProps<{
  threads: readonly Thread[]
  viewer: Viewer | null
  collab: Pick<DocumentCollab, 'createThread' | 'reply' | 'setResolved' | 'editComment' | 'deleteComment'>
  activeId: string | null
  hoveredId: string | null
  /** The element holding the rendered document; null stacks every thread. */
  alignTo: HTMLElement | null
  /** Where "Sign in to comment" goes; null hides it. */
  signIn: RouteLocationRaw | null
  error: string | null
}>()
const emit = defineEmits<{ select: [id: string]; hover: [id: string | null]; close: [] }>()

const groups = computed(() => {
  const open: Thread[] = []
  const general: Thread[] = []
  const outdated: Thread[] = []
  const resolved: Thread[] = []
  for (const t of props.threads) {
    if (t.resolved_at) resolved.push(t)
    else if (!t.anchor) general.push(t)
    else if (t.outdated) outdated.push(t)
    else open.push(t)
  }
  return { open, general, outdated, resolved }
})
const stacked = computed(() =>
  [
    { key: 'general', title: 'General', threads: groups.value.general },
    { key: 'outdated', title: 'Outdated', threads: groups.value.outdated },
    { key: 'resolved', title: 'Resolved', threads: groups.value.resolved },
  ].filter((g) => g.threads.length),
)

const canComment = computed(() => !!props.viewer?.can_comment)
const signedIn = computed(() => !!props.viewer?.user_id)

function anchor(id: string): HTMLElement | null {
  const root = props.alignTo
  if (!root) return null
  const escaped = CSS.escape(id)
  return (
    root.querySelector<HTMLElement>(`[data-review-anchor="${escaped}"]`) ??
    root.querySelector<HTMLElement>(`[data-review-ids~="${escaped}"]`)
  )
}

const cards = ref<HTMLElement | null>(null)
const aligned = useAlignedCards({
  cards,
  ids: () => groups.value.open.map((t) => t.id),
  anchor,
  alignTo: () => props.alignTo,
  deps: [() => props.threads, () => props.activeId],
})

function select(id: string): void {
  emit('select', id)
  anchor(id)?.scrollIntoView({ block: 'center', behavior: 'smooth' })
}
</script>

<template>
  <aside class="flex min-h-full flex-col bg-pane" data-testid="comment-rail">
    <header class="sticky top-0 z-10 flex h-11 shrink-0 items-center gap-2 border-b border-row bg-pane px-4">
      <span class="text-small font-semibold text-text">Comments</span>
      <span class="rounded bg-chip px-1.5 py-0.5 font-mono text-micro leading-none text-text-2">{{
        threads.length
      }}</span>
      <div class="flex-1" />
      <IconButton :icon="IconX" label="Close comments" size="sm" data-testid="rail-close" @click="emit('close')" />
    </header>
    <div class="flex flex-1 flex-col gap-2 px-4 py-2">
      <InlineError v-if="error" variant="line" :message="error" testid="rail-error" />
      <p v-if="!threads.length" class="py-2 text-small text-text-4" data-testid="rail-empty">
        No comments yet. Select text to comment on it.
      </p>
      <ol
        v-if="groups.open.length"
        ref="cards"
        class="flex flex-col"
        :class="alignTo ? 'relative' : 'gap-2'"
        :style="alignTo ? { minHeight: `${aligned.height.value}px` } : undefined"
        data-testid="group-open"
      >
        <ThreadCard
          v-for="t in groups.open"
          :key="t.id"
          :ref="(card) => aligned.cardRef(t.id, card)"
          :thread="t"
          :viewer="viewer"
          :collab="collab"
          :active="activeId === t.id"
          :hovered="hoveredId === t.id"
          :aligned="!!alignTo"
          :top="aligned.tops.value.get(t.id)"
          :sliding="aligned.sliding.value.has(t.id)"
          @select="select"
          @hover="emit('hover', $event)"
        />
      </ol>
      <section
        v-for="group in stacked"
        :key="group.key"
        class="flex flex-col gap-2 pt-2"
        :data-testid="`group-${group.key}`"
      >
        <h3 class="text-caption font-medium tracking-wide text-text-4 uppercase">
          {{ group.title }} · {{ group.threads.length }}
        </h3>
        <ol class="flex flex-col gap-2">
          <ThreadCard
            v-for="t in group.threads"
            :key="t.id"
            :thread="t"
            :viewer="viewer"
            :collab="collab"
            :active="activeId === t.id"
            :hovered="hoveredId === t.id"
            @select="emit('select', $event)"
            @hover="emit('hover', $event)"
          />
        </ol>
      </section>
    </div>
    <footer class="sticky bottom-0 shrink-0 border-t border-row bg-pane px-4 py-3">
      <div v-if="canComment" data-testid="rail-composer">
        <CommentComposer
          :submit="(text) => collab.createThread(null, text)"
          placeholder="Add a general comment"
          testid="new-thread"
        />
      </div>
      <p v-else-if="signedIn" class="text-small text-text-3" data-testid="rail-readonly">
        This sign-in cannot comment.
      </p>
      <RouterLink
        v-else-if="signIn"
        :to="signIn"
        class="text-small font-medium text-accent hover:underline"
        data-testid="rail-sign-in"
      >
        Sign in to comment
      </RouterLink>
    </footer>
  </aside>
</template>
