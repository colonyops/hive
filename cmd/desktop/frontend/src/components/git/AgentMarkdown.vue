<script setup lang="ts">
import { computed } from 'vue'
import { Browser } from '@wailsio/runtime'
import { renderGithubMarkdown } from '@hive/ui/lib/githubMarkdown'
import { useCodeHighlight } from '@hive/ui/lib/highlight'
import { externalMarkdownHref } from '@hive/ui/lib/markdownLinks'

// An agent wrote this, so it goes through the renderer that is strict enough for GitHub bodies.
const props = defineProps<{ source: string }>()

const { highlight } = useCodeHighlight()
const html = computed(() => renderGithubMarkdown(props.source, highlight))

function onClick(event: MouseEvent): void {
  const href = externalMarkdownHref(event)
  if (href) void Browser.OpenURL(href)
}
</script>

<template>
  <!-- eslint-disable vue/no-v-html -- renderGithubMarkdown escapes raw HTML (githubMarkdown.spec.ts) -->
  <div class="markdown-body break-words" @click="onClick" v-html="html" />
  <!-- eslint-enable vue/no-v-html -->
</template>
