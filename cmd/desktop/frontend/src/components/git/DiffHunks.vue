<script setup lang="ts">
// One file's hunks. Each hunk's old and new sides highlight as one block each,
// because the highlighter takes whole code blocks and a line alone loses the
// context around it. A line shows as plain text until its block lands.
import { computed } from 'vue'
import { useCodeHighlight } from '@hive/ui/lib/highlight'
import {
  diffLanguage,
  highlightedLines,
  parseUnifiedDiff,
  splitRows,
  type DiffHunk,
  type DiffLine,
} from '../../lib/unifiedDiff'

const props = defineProps<{ path: string; text: string; mode: 'unified' | 'split' }>()

const hunks = computed(() => parseUnifiedDiff(props.text))
const { highlight } = useCodeHighlight()

function sideHtml(hunk: DiffHunk, side: 'old' | 'new'): Map<DiffLine, string> {
  const lines = hunk.lines.filter((line) => line.kind === 'context' || line.kind === (side === 'old' ? 'del' : 'add'))
  const html = highlight(lines.map((line) => line.text).join('\n'), diffLanguage(props.path))
  const out = new Map<DiffLine, string>()
  if (!html) return out
  highlightedLines(html).forEach((lineHtml, index) => {
    if (lines[index]) out.set(lines[index], lineHtml)
  })
  return out
}

const rendered = computed(() =>
  hunks.value.map((hunk) => {
    const before = sideHtml(hunk, 'old')
    const after = sideHtml(hunk, 'new')
    // A context line is the same on both sides, so either block can color it.
    const html = (line: DiffLine) => (line.kind === 'del' ? before.get(line) : after.get(line)) ?? null
    return { hunk, html, split: props.mode === 'split' ? splitRows(hunk.lines) : [] }
  }),
)

const MARK = { context: ' ', add: '+', del: '-' }
const TINT = { context: '', add: 'diff-add', del: 'diff-del' }
</script>

<template>
  <div class="diff" :class="mode === 'split' ? 'diff-split' : ''">
    <template v-for="{ hunk, html, split } in rendered" :key="hunk.header">
      <div class="diff-hunk" data-hunk>{{ hunk.header }}</div>
      <template v-if="mode === 'unified'">
        <div
          v-for="(line, index) in hunk.lines"
          :key="index"
          class="diff-row"
          :class="TINT[line.kind]"
          :data-line-kind="line.kind"
        >
          <span class="diff-num">{{ line.oldNumber }}</span>
          <span class="diff-num">{{ line.newNumber }}</span>
          <span class="diff-mark">{{ MARK[line.kind] }}</span>
          <!-- eslint-disable-next-line vue/no-v-html -- shiki escapes the code itself -->
          <span v-if="html(line)" class="diff-code" v-html="html(line)" />
          <span v-else class="diff-code">{{ line.text }}</span>
        </div>
      </template>
      <template v-else>
        <div v-for="(row, index) in split" :key="index" class="diff-pair" data-testid="changes-diff-pair">
          <div
            v-for="(line, side) in [row.left, row.right]"
            :key="side"
            class="diff-row"
            :class="line ? TINT[line.kind] : 'diff-empty'"
          >
            <template v-if="line">
              <span class="diff-num">{{ side === 0 ? line.oldNumber : line.newNumber }}</span>
              <span class="diff-mark">{{ MARK[line.kind] }}</span>
              <!-- eslint-disable-next-line vue/no-v-html -- shiki escapes the code itself -->
              <span v-if="html(line)" class="diff-code" v-html="html(line)" />
              <span v-else class="diff-code">{{ line.text }}</span>
            </template>
          </div>
        </div>
      </template>
    </template>
  </div>
</template>

<style scoped>
.diff {
  min-width: max-content;
  font-family: var(--font-mono);
  font-size: 11.5px;
  line-height: 18px;
  font-variant-ligatures: none;
}
.diff-split {
  min-width: 0;
}
.diff-hunk {
  padding: 1px 10px;
  background: var(--color-raised);
  color: var(--color-text-3);
  white-space: pre;
}
.diff-row {
  display: flex;
  min-height: 18px;
  white-space: pre;
  color: var(--color-text-2);
}
.diff-add {
  background: var(--color-severity-success-tint);
  color: var(--color-text);
}
.diff-del {
  background: var(--color-severity-error-tint);
  color: var(--color-text);
}
.diff-num {
  flex: none;
  width: 42px;
  padding-right: 6px;
  text-align: right;
  color: var(--color-text-4);
  user-select: none;
}
.diff-mark {
  flex: none;
  width: 13px;
  color: var(--color-text-4);
  user-select: none;
}
.diff-add .diff-mark {
  color: var(--color-severity-success);
}
.diff-del .diff-mark {
  color: var(--color-severity-error);
}
.diff-code :deep(span) {
  color: var(--shiki-app);
}
.diff-pair {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
}
.diff-pair .diff-row {
  min-width: 0;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.diff-pair .diff-row + .diff-row {
  border-left: 1px solid var(--color-border);
}
.diff-pair .diff-code {
  min-width: 0;
}
.diff-empty {
  background: var(--color-raised);
}
</style>
