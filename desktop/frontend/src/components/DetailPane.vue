<script setup lang="ts">
import { computed } from 'vue'
import ActionCard from './ActionCard.vue'
import PanelResizeHandle from './PanelResizeHandle.vue'
import SourceMark from './SourceMark.vue'
import { useResizablePanel } from '../composables/useResizablePanel'
import { feedSource } from '../lib/feedPresentation'
import { renderGithubMarkdown } from '../lib/githubMarkdown'
import IconCircleDot from '~icons/lucide/circle-dot'
import IconExternalLink from '~icons/lucide/external-link'
import IconGitPullRequest from '~icons/lucide/git-pull-request'
import IconInfo from '~icons/lucide/info'
import IconSettings from '~icons/lucide/settings'
import type { FeedItem } from '../types/feed'
import type { ActionView } from '../types/action'
import type { ActionRunView } from '../../bindings/github.com/colonyops/hive/internal/desktop/pipeline/models'

const props = defineProps<{ item: FeedItem | null; actions: ActionView[]; pendingAction?: string | null; actionRuns?: Record<string, ActionRunView> }>()
const emit = defineEmits<{ 'run-action': [actionId: string]; 'open-browser': []; 'open-url': [url: string]; edit: [] }>()

// The detail header leads with the item's source badge and type. The badge
// already identifies the provider, so the adjacent context stays focused on
// the repository and item number.
const source = computed(() => feedSource(props.item ?? undefined))

// Issue/PR bodies are GitHub-flavored markdown from untrusted authors;
// renderGithubMarkdown parses the GFM and escapes raw HTML / unsafe links, so
// the result is safe to inject with v-html.
const bodyHtml = computed(() => (props.item ? renderGithubMarkdown(props.item.body) : ''))

// Links inside the rendered body must open in the user's real browser rather
// than navigate the webview away from the app. Intercept anchor clicks and
// hand the href up to the parent, which routes it through Wails.
function onBodyClick(event: MouseEvent) {
  const anchor = (event.target as HTMLElement).closest('a')
  if (!anchor) return
  event.preventDefault()
  const href = anchor.getAttribute('href') ?? ''
  if (/^(https?:|mailto:)/i.test(href)) emit('open-url', href)
}

// DetailPane is docked on the right, so its handle sits on its left border —
// dragging left (toward the FeedList) grows the pane.
// max is generous so the preview can take over most of the window like an
// email client's reading pane; the FeedList (flex-1, min-w-0) yields the space.
const { size: paneWidth, startResize: startPaneResize, step: stepPane } = useResizablePanel({
  storageKey: 'hive.panel.detailpane',
  defaultSize: 466,
  min: 360,
  max: 1100,
  edge: 'left',
})

// The markdown body is a fixed-height, scrollable reading pane (email-client
// style): the user drags the divider below it to set the height — larger than
// the content (blank space) or smaller (the body scrolls) — and it's
// persisted, so a long issue/PR description never buries the ACTIONS section.
const { size: bodyHeight, startResize: startBodyResize, step: stepBody } = useResizablePanel({
  storageKey: 'hive.panel.detailbody',
  defaultSize: 240,
  min: 96,
  max: 640,
  edge: 'bottom',
})
</script>

<template>
  <aside class="hive-scroll relative flex shrink-0 flex-col overflow-y-auto bg-pane" :style="{ width: paneWidth + 'px' }" data-testid="detail-pane">
    <PanelResizeHandle edge="left" name="detailpane" :start="startPaneResize" :step="stepPane" />
    <template v-if="item">
      <div class="relative border-b border-border px-5 pb-4 pt-[18px]">
        <div class="mb-[11px] flex items-center gap-[9px]">
          <span class="source-badge" :data-source="source.key" data-testid="source-badge"><SourceMark :source="source" class="size-[15px]" /></span>
          <span class="kind-pill shrink-0 whitespace-nowrap" :class="item.kind === 'PR' ? 'kind-pill-pr' : 'kind-pill-issue'">
            <IconGitPullRequest v-if="item.kind === 'PR'" class="size-[13px]" />
            <IconCircleDot v-else class="size-[13px]" />
            {{ item.kind === 'PR' ? 'Pull Request' : 'Issue' }}
          </span>
          <span class="min-w-0 truncate font-mono text-xs text-text-3">{{ item.repo }} #{{ item.num }}</span>
          <span class="flex-1" />
          <button class="open-button shrink-0" @click="emit('open-browser')">open <IconExternalLink class="size-3" /></button>
        </div>
        <h1 class="text-[17px] font-semibold leading-[1.3] tracking-[-.01em]">{{ item.title }}</h1>
        <p class="mt-[9px] text-xs text-text-3"><span class="text-text-2">{{ item.author }}</span> · {{ item.age }} ago</p>
        <div v-if="bodyHtml" class="markdown-body hive-scroll mt-3 overflow-y-auto text-[14px] leading-[1.65] text-text-2" :style="{ height: bodyHeight + 'px' }" data-testid="detail-body" @click="onBodyClick" v-html="bodyHtml" />
        <!-- The border-b line below is draggable: it sets the description's
             reading-pane height (persisted), so long bodies never bury the actions. -->
        <PanelResizeHandle v-if="bodyHtml" edge="bottom" name="detailbody" :start="startBodyResize" :step="stepBody" />
      </div>

      <div class="px-5 pb-5 pt-4">
        <div class="mb-[13px] flex items-center gap-2">
          <span class="font-mono text-[10.5px] tracking-[.12em] text-accent">ACTIONS</span>
          <span class="font-mono text-[10.5px] text-text-4">· for {{ item.kind }}</span>
          <span class="flex-1" />
          <button class="edit-button" @click="emit('edit')"><IconSettings class="size-3" /> Edit</button>
        </div>
        <div class="flex flex-col gap-[9px]">
          <ActionCard v-for="action in actions" :key="action.id" :action="action" :pending="pendingAction === action.id" :run="actionRuns?.[action.id]" @run="emit('run-action', action.id)" />
        </div>
        <div class="action-footer-meta mt-3.5 font-mono text-[11px] text-text-3" data-testid="action-footer-meta"><IconInfo class="mt-0.5 size-3 shrink-0 text-accent" /><div class="min-w-0"><span class="block">Runs headless (batch) on</span><span class="block break-words text-text-2" data-testid="action-footer-branch">{{ item.branch }}</span></div></div>
        <div class="mt-1.5 pl-[19px] font-mono text-[11px] text-text-4">Actions defined in desktop actions.yml</div>
      </div>
    </template>
    <div v-else class="m-auto font-mono text-xs text-text-4">Select an item to inspect</div>
  </aside>
</template>

<style scoped>
.source-badge { display: inline-flex; flex: none; align-items: center; justify-content: center; width: 26px; height: 26px; border-radius: 7px; background: var(--color-chip); border: 1px solid var(--color-strong); color: var(--color-text); }
.kind-pill { display: inline-flex; align-items: center; gap: 6px; height: 22px; padding: 0 9px 0 7px; border-radius: 6px; font-size: 11px; font-weight: 600; }
.kind-pill-pr { background: var(--color-kind-pr-tint); color: var(--color-kind-pr); }
.kind-pill-issue { background: var(--color-kind-issue-tint); color: var(--color-kind-issue); }
.open-button, .edit-button { display: inline-flex; align-items: center; gap: 4px; cursor: pointer; border: 1px solid var(--color-card); border-radius: 4px; padding: 2px 7px; color: var(--color-text-2); font-family: var(--font-mono); font-size: 11px; }
.edit-button { border-radius: 5px; padding: 3px 8px; font-family: var(--font-sans); }
.open-button:hover, .edit-button:hover { border-color: var(--color-strong); color: var(--color-text); }
.action-footer-meta { display: grid; grid-template-columns: 12px minmax(0, 1fr); column-gap: 8px; align-items: start; }

/* Rendered issue/PR body (GitHub-flavored markdown). Its height is set inline
   from the user-adjustable bodyHeight, so a long description scrolls internally
   instead of burying the ACTIONS section. */
.markdown-body :deep(h1), .markdown-body :deep(h2), .markdown-body :deep(h3),
.markdown-body :deep(h4), .markdown-body :deep(h5), .markdown-body :deep(h6) {
  margin: 20px 0 8px; color: var(--color-text); font-weight: 650; line-height: 1.3;
}
.markdown-body :deep(h1) { font-size: 19px; }
.markdown-body :deep(h2) { font-size: 16.5px; }
.markdown-body :deep(h3) { font-size: 15px; }
.markdown-body :deep(h4), .markdown-body :deep(h5), .markdown-body :deep(h6) { font-size: 14px; }
.markdown-body :deep(*:first-child) { margin-top: 0; }
.markdown-body :deep(*:last-child) { margin-bottom: 0; }
.markdown-body :deep(p) { margin: 10px 0; }
.markdown-body :deep(a) { color: var(--color-accent); text-decoration: underline; text-underline-offset: 2px; cursor: pointer; }
.markdown-body :deep(a:hover) { text-decoration-thickness: 2px; }
.markdown-body :deep(ul), .markdown-body :deep(ol) { margin: 10px 0; padding-left: 22px; }
.markdown-body :deep(ul) { list-style: disc; }
.markdown-body :deep(ol) { list-style: decimal; }
.markdown-body :deep(li) { margin: 4px 0; }
.markdown-body :deep(li)::marker { color: var(--color-text-4); }
/* Task-list checkboxes: a small custom control instead of the bright, oversized
   native checkbox. Rendered read-only (the plugin emits `disabled`). */
.markdown-body :deep(ul.contains-task-list) { padding-left: 4px; }
.markdown-body :deep(li.task-list-item) { list-style: none; }
.markdown-body :deep(li.task-list-item input) {
  appearance: none; -webkit-appearance: none;
  position: relative; box-sizing: border-box;
  width: 14px; height: 14px; margin: 0 8px 0 0; vertical-align: -2px;
  border: 1.5px solid var(--color-strong); border-radius: 4px;
  background: var(--color-app);
}
.markdown-body :deep(li.task-list-item input:checked) { background: var(--color-accent); border-color: var(--color-accent); }
.markdown-body :deep(li.task-list-item input:checked)::after {
  content: ''; position: absolute; left: 4px; top: 1px;
  width: 3.5px; height: 7px; border: solid var(--color-accent-contrast);
  border-width: 0 2px 2px 0; transform: rotate(45deg);
}
.markdown-body :deep(strong) { color: var(--color-text); font-weight: 650; }
.markdown-body :deep(s) { color: var(--color-text-3); }
.markdown-body :deep(blockquote) {
  margin: 10px 0; padding: 2px 14px; border-left: 3px solid var(--color-border);
  color: var(--color-text-3);
}
.markdown-body :deep(hr) { margin: 16px 0; border: 0; border-top: 1px solid var(--color-border); }
.markdown-body :deep(code) {
  padding: 1.5px 6px; border-radius: 5px; background: var(--color-card);
  font-family: var(--font-mono); font-size: 0.86em;
}
.markdown-body :deep(pre) {
  margin: 10px 0; padding: 12px 14px; overflow-x: auto; border-radius: 7px;
  background: var(--color-card); line-height: 1.5;
}
.markdown-body :deep(pre code) { padding: 0; background: transparent; font-size: 0.86em; }
.markdown-body :deep(table) { margin: 10px 0; border-collapse: collapse; display: block; overflow-x: auto; font-size: 0.95em; }
.markdown-body :deep(th), .markdown-body :deep(td) { padding: 5px 11px; border: 1px solid var(--color-border); text-align: left; }
.markdown-body :deep(th) { background: var(--color-card); font-weight: 650; }
.markdown-body :deep(img) { max-width: 100%; }
</style>
