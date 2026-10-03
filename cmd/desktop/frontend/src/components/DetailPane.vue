<script setup lang="ts">
import { computed, ref } from 'vue'
import ActionCard from './ActionCard.vue'
import ItemActionMenu from './ItemActionMenu.vue'
import IconButton from './ui/IconButton.vue'
import PanelResizeHandle from './ui/PanelResizeHandle.vue'
import PullRequestMetadata from './PullRequestMetadata.vue'
import { useResizablePanel } from '../composables/useResizablePanel'
import {
  body,
  byline,
  container,
  containerLine,
  kind,
  kindIcon,
  kindLabel,
  kindStyle,
  presentationFor,
} from '../lib/itemPresentation'
import { relativeAge, relativeAgo } from '../lib/age'
import { renderGithubMarkdown } from '../lib/githubMarkdown'
import { externalMarkdownHref } from '../lib/markdownLinks'
import IconEllipsisVertical from '~icons/lucide/ellipsis-vertical'
import IconSettings from '~icons/lucide/settings'
import type { InboxEvent, InboxItem } from '../types/feed'
import type { ActionView } from '../types/action'
import type {
  ActionRunView,
  ItemChatView,
  ItemSessionView,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'
import FeedKindPill from './FeedKindPill.vue'
import FeedSourceBadge from './FeedSourceBadge.vue'

const props = defineProps<{
  item: InboxItem | null
  actions: ActionView[]
  events?: InboxEvent[]
  sessions?: ItemSessionView[]
  chats?: ItemChatView[]
  pendingAction?: string | null
  actionRuns?: Record<string, ActionRunView>
  sourceIcons?: Record<string, string>
  sourceImages?: Record<string, string>
}>()
const emit = defineEmits<{
  'run-action': [actionId: string]
  'open-browser': []
  'open-url': [url: string]
  'set-unread': [unread: boolean]
  'toggle-archive': []
  'toggle-ignored': []
  'copy-link': []
  'copy-contents': []
  'create-session': [target: 'repository' | 'workspace']
  'open-session': [slug: string]
  'open-chat': [workspace: string, id: number]
  edit: []
}>()

// A session is reachable only while it still has a checkout: a recycled or
// corrupted one has no tmux session to attach to, and its row stays a record
// of what ran rather than a link.
function attachable(session: ItemSessionView): boolean {
  return session.state === 'active'
}

const itemMenuWrap = ref<HTMLElement | null>(null)
const itemMenuOpen = ref(false)

// The detail header leads with the item's source badge and type. The badge
// already identifies the provider (via the sourceKind-keyed adapter
// registry), so the adjacent context stays focused on the repository and
// item number.
const presentation = computed(() => presentationFor(props.item?.sourceKind))
// The github/default adapters never dereference `item`; the source badge only
// renders inside the `v-if="item"` branch below, so a null item here is
// never actually resolved to the webhook adapter (which does).
const markContext = computed(() => ({ sourceIcons: props.sourceIcons, sourceImages: props.sourceImages }))
const sourceMark = computed(() => presentation.value.mark(props.item!, markContext.value))
const sourceMarkImage = computed(() => presentation.value.markImage?.(props.item!, markContext.value))
// The kind pill mirrors the feed row's type pill: PR and Issue keep their
// GitHub styling, anything else (webhook items, custom kinds) is neutral.
const itemKind = computed(() => (props.item ? kind(props.item) : ''))
const itemKindLabel = computed(() => (props.item ? kindLabel(props.item) : ''))
const itemKindStyle = computed(() => (props.item ? kindStyle(props.item) : 'neutral'))
const itemKindIcon = computed(() => (props.item ? kindIcon(props.item) : undefined))
const itemContainer = computed(() => (props.item ? container(props.item) : ''))
const itemContainerLine = computed(() => (props.item ? containerLine(props.item) : ''))
const itemByline = computed(() => (props.item ? byline(props.item) : ''))

// Issue/PR bodies are GitHub-flavored markdown from untrusted authors;
// renderGithubMarkdown parses the GFM and escapes raw HTML / unsafe links, so
// the result is safe to inject with v-html.
const bodyHtml = computed(() => (props.item ? renderGithubMarkdown(body(props.item)) : ''))

// Links inside the rendered body must open in the user's real browser rather
// than navigate the webview away from the app. Intercept anchor clicks and
// hand the href up to the parent, which routes it through Wails.
function onBodyClick(event: MouseEvent) {
  const href = externalMarkdownHref(event)
  if (href) emit('open-url', href)
}

// DetailPane is docked on the right, so its handle sits on its left border —
// dragging left (toward the FeedList) grows the pane.
// max is generous so the preview can take over most of the window like an
// email client's reading pane; the FeedList (flex-1, min-w-0) yields the space.
const {
  size: paneWidth,
  startResize: startPaneResize,
  step: stepPane,
} = useResizablePanel({
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
const {
  size: bodyHeight,
  startResize: startBodyResize,
  step: stepBody,
} = useResizablePanel({
  storageKey: 'hive.panel.detailbody',
  defaultSize: 240,
  min: 96,
  max: 640,
  edge: 'bottom',
})
</script>

<template>
  <aside
    class="hive-scroll relative flex shrink-0 flex-col overflow-y-auto bg-pane"
    :style="{ width: paneWidth + 'px' }"
    data-testid="detail-pane"
  >
    <PanelResizeHandle edge="left" name="detailpane" :start="startPaneResize" :step="stepPane" />
    <template v-if="item">
      <div class="relative border-b border-border px-5 pb-4 pt-[18px]">
        <div class="mb-[11px] flex items-center gap-[9px]">
          <FeedSourceBadge
            size="sm"
            :icon="sourceMark"
            :image="sourceMarkImage"
            :source="item.sourceKind"
            data-testid="source-badge"
          />
          <FeedKindPill size="md" :tone="itemKindStyle" data-testid="kind-pill">
            <component :is="itemKindIcon" v-if="itemKindIcon" class="size-[13px]" />
            {{ itemKindLabel }}
          </FeedKindPill>
          <span v-if="itemContainer" class="min-w-0 truncate font-mono text-xs text-text-3">{{
            itemContainerLine
          }}</span>
          <span class="flex-1" />
          <div ref="itemMenuWrap" class="relative shrink-0">
            <IconButton
              label="Item actions"
              :icon="IconEllipsisVertical"
              variant="outline"
              aria-haspopup="menu"
              data-testid="item-actions-toggle"
              :active="itemMenuOpen"
              :aria-expanded="itemMenuOpen"
              @click="itemMenuOpen = !itemMenuOpen"
            />
            <ItemActionMenu
              v-if="itemMenuOpen"
              :item="item"
              :actions="actions"
              :ignore="[itemMenuWrap]"
              testid="item-actions-menu"
              @close="itemMenuOpen = false"
              @set-unread="(value) => emit('set-unread', value)"
              @toggle-archive="emit('toggle-archive')"
              @toggle-ignored="emit('toggle-ignored')"
              @open-browser="emit('open-browser')"
              @copy-link="emit('copy-link')"
              @copy-contents="emit('copy-contents')"
              @create-session="(target) => emit('create-session', target)"
              @run-action="(actionId) => emit('run-action', actionId)"
            />
          </div>
        </div>
        <h1 class="text-heading font-semibold leading-[1.3] tracking-[-.01em]">{{ item.title }}</h1>
        <p class="mt-[9px] text-xs text-text-3">
          <template v-if="itemByline"
            ><span class="text-text-2">{{ itemByline }}</span> · </template
          >{{ relativeAgo(item.lastEventAt) }}
        </p>
        <PullRequestMetadata :item="item" class="mt-3" />
        <!-- eslint-disable vue/no-v-html -- renderGithubMarkdown escapes raw HTML (githubMarkdown.spec.ts) -->
        <div
          v-if="bodyHtml"
          class="markdown-body hive-scroll mt-3 overflow-y-auto text-reading leading-[1.65] text-text-2"
          :style="{ height: bodyHeight + 'px' }"
          data-testid="detail-body"
          @click="onBodyClick"
          v-html="bodyHtml"
        />
        <!-- eslint-enable vue/no-v-html -->
        <!-- The border-b line below is draggable: it sets the description's
             reading-pane height (persisted), so long bodies never bury the actions. -->
        <PanelResizeHandle v-if="bodyHtml" edge="bottom" name="detailbody" :start="startBodyResize" :step="stepBody" />
      </div>

      <div class="px-5 pb-5 pt-4">
        <!-- Actions are capability-gated, not provider-gated: the block shows
             whenever the backend offered at least one applicable action
             (ActionApplicability — applies_to + payload capability), so it
             never renders an empty header for an item with zero matches. -->
        <template v-if="actions.length">
          <div class="mb-[13px] flex items-center gap-2">
            <span class="font-mono text-micro tracking-[.12em] text-accent">ACTIONS</span>
            <span class="font-mono text-micro text-text-4">· for {{ itemKind }}</span>
            <span class="flex-1" />
            <button class="edit-button" @click="emit('edit')"><IconSettings class="size-3" /> Edit</button>
          </div>
          <div class="action-list">
            <ActionCard
              v-for="action in actions"
              :key="action.id"
              :action="action"
              :pending="pendingAction === action.id"
              :run="actionRuns?.[action.id]"
              @run="emit('run-action', action.id)"
            />
          </div>
        </template>
        <!-- Sessions this item spawned. Absent rather than empty when it has
             none: an item that never started work should not carry a heading
             announcing that. -->
        <section v-if="(sessions ?? []).length" class="mt-6 border-t border-border pt-4" data-testid="item-sessions">
          <h2 class="mb-3 font-mono text-micro tracking-[.12em] text-accent">SESSIONS</h2>
          <ul class="session-list">
            <li v-for="session in sessions ?? []" :key="session.id">
              <component
                :is="attachable(session) ? 'button' : 'div'"
                class="session-row"
                :class="{ 'session-row-linked': attachable(session) }"
                :data-testid="'item-session-' + session.id"
                v-bind="attachable(session) ? { type: 'button' } : {}"
                @click="attachable(session) && emit('open-session', session.slug)"
              >
                <span class="session-dot" :class="session.running ? 'session-dot-live' : 'session-dot-idle'" />
                <span class="min-w-0 flex-1 truncate text-body text-text">{{ session.name }}</span>
                <span class="shrink-0 font-mono text-micro text-text-4">{{
                  session.running ? 'running' : session.state === 'active' ? 'idle' : session.state
                }}</span>
              </component>
              <p class="session-meta">
                <span v-if="session.repo" class="truncate">{{ session.repo }}</span>
                <span v-if="session.repo">·</span>
                <span>{{ relativeAge(new Date(session.createdAt).getTime()) }}</span>
              </p>
            </li>
          </ul>
        </section>
        <section v-if="(chats ?? []).length" class="mt-6 border-t border-border pt-4" data-testid="item-chats">
          <h2 class="mb-3 font-mono text-micro tracking-[.12em] text-accent">CHATS</h2>
          <ul class="session-list">
            <li v-for="chat in chats ?? []" :key="chat.id">
              <button
                type="button"
                class="session-row session-row-linked"
                :data-testid="'item-chat-' + chat.id"
                @click="emit('open-chat', chat.workspace, chat.id)"
              >
                <span class="session-dot session-dot-idle" />
                <span class="min-w-0 flex-1 truncate text-body text-text">{{ chat.name }}</span>
              </button>
              <p class="session-meta">
                <span class="truncate">{{ chat.workspace }}</span>
                <span>·</span>
                <span>{{ relativeAge(new Date(chat.createdAt).getTime()) }}</span>
              </p>
            </li>
          </ul>
        </section>
        <section v-if="(events ?? []).length" class="mt-6 border-t border-border pt-4" data-testid="observed-activity">
          <h2 class="mb-3 font-mono text-micro tracking-[.12em] text-accent">ACTIVITY</h2>
          <ol class="space-y-2">
            <li v-for="event in events ?? []" :key="event.id" class="text-xs text-text-3">
              <span class="text-text-2">{{ event.summary || event.kind }}</span
              ><span v-if="event.summary && event.kind !== 'observed'" class="ml-1 font-mono text-micro text-text-4">{{
                event.kind.replaceAll('_', ' ')
              }}</span
              ><span class="ml-2 font-mono text-micro">{{ relativeAge(event.createdAt) }}</span>
            </li>
          </ol>
        </section>
      </div>
    </template>
    <div v-else class="m-auto font-mono text-xs text-text-4">Select an item to inspect</div>
  </aside>
</template>

<style scoped>
.edit-button {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  cursor: pointer;
  border: 1px solid var(--color-card);
  border-radius: var(--radius-md);
  padding: 3px 8px;
  color: var(--color-text-2);
  font-size: var(--text-caption);
}
.edit-button:hover {
  border-color: var(--color-strong);
  color: var(--color-text);
}
.action-list {
  overflow: hidden;
  border: 1px solid var(--color-card);
  border-radius: var(--radius-lg);
  background: var(--color-raised);
}
.session-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.session-row {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 8px;
  text-align: left;
}
.session-row-linked {
  cursor: pointer;
}
.session-row-linked:hover .session-dot {
  box-shadow: 0 0 0 3px var(--color-chip);
}
.session-row-linked:hover span:not(.session-dot) {
  color: var(--color-accent);
}
.session-dot {
  flex: none;
  width: 7px;
  height: 7px;
  border-radius: 50%;
}
.session-dot-live {
  background: var(--color-accent);
}
.session-dot-idle {
  background: var(--color-strong);
}
.session-meta {
  display: flex;
  gap: 5px;
  padding-left: 15px;
  color: var(--color-text-4);
  font-family: var(--font-mono);
  font-size: var(--text-micro);
}
</style>
