<script setup lang="ts">
// One release's notes, rendered. The markdown is first-party — it ships inside
// the binary — but it goes through the same escaping renderer as issue bodies
// rather than a second, laxer one.
import { computed } from 'vue'
import { renderGithubMarkdown } from '../lib/githubMarkdown'

const props = defineProps<{ body: string }>()

const html = computed(() => renderGithubMarkdown(props.body))
</script>

<template>
  <div v-if="html" class="release-notes text-[13.5px] leading-[1.65] text-text-2" v-html="html" />
</template>

<style scoped>
.release-notes :deep(h1), .release-notes :deep(h2), .release-notes :deep(h3) {
  margin: 18px 0 6px; color: var(--color-text); font-weight: 650; line-height: 1.3; font-size: 13px;
  text-transform: uppercase; letter-spacing: .06em;
}
.release-notes :deep(*:first-child) { margin-top: 0; }
.release-notes :deep(*:last-child) { margin-bottom: 0; }
.release-notes :deep(p) { margin: 8px 0; }
.release-notes :deep(ul), .release-notes :deep(ol) { margin: 8px 0; padding-left: 20px; }
.release-notes :deep(ul) { list-style: disc; }
.release-notes :deep(ol) { list-style: decimal; }
.release-notes :deep(li) { margin: 5px 0; }
.release-notes :deep(li)::marker { color: var(--color-text-4); }
.release-notes :deep(strong) { color: var(--color-text); font-weight: 650; }
.release-notes :deep(code) {
  padding: 1px 5px; border-radius: 5px; background: var(--color-card);
  font-family: var(--font-mono); font-size: 12px;
}
.release-notes :deep(a) { color: var(--color-accent); text-decoration: underline; text-underline-offset: 2px; }
</style>
