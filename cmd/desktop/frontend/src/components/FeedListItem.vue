<script setup lang="ts">
import { computed, ref } from 'vue'
import ItemActionMenu from './ItemActionMenu.vue'
import PullRequestMetadata from './PullRequestMetadata.vue'
import SourceMark from './SourceMark.vue'
import { relativeAge } from '../lib/age'
import { byline, container, containerLine, kind, kindLabel, kindStyle, presentationFor, pullRequestMetadata } from '../lib/itemPresentation'
import IconArchive from '~icons/lucide/archive'
import IconCheck from '~icons/lucide/check'
import IconDot from '~icons/lucide/dot'
import IconEllipsisVertical from '~icons/lucide/ellipsis-vertical'
import IconExternalLink from '~icons/lucide/external-link'
import IconEye from '~icons/lucide/eye'
import type { InboxItem } from '../types/feed'

const props = defineProps<{ item: InboxItem; archived?: boolean; trash?: boolean; selected: boolean; selectionMode?: boolean; checked?: boolean; sourceIcons?: Record<string, string>; sourceImages?: Record<string, string> }>()
const emit = defineEmits<{
  select: []
  activate: []
  'toggle-selection': []
  'set-unread': [unread: boolean]
  'toggle-archive': []
  'toggle-ignored': []
  'open-browser': []
  'copy-link': []
  'copy-contents': []
  'create-session': [target: 'repository' | 'workspace']
  'run-action': [actionId: string]
}>()
// The source label, badge mark, and (for webhook items) icon resolution are
// all provider-variant — delegated to the sourceKind-keyed adapter registry.
const presentation = computed(() => presentationFor(props.item.sourceKind))
const markContext = computed(() => ({ sourceIcons: props.sourceIcons, sourceImages: props.sourceImages }))
const sourceMark = computed(() => presentation.value.mark(props.item, markContext.value))
const sourceMarkImage = computed(() => presentation.value.markImage?.(props.item, markContext.value))
const itemKind = computed(() => kind(props.item))
const itemKindLabel = computed(() => kindLabel(props.item))
const itemKindStyle = computed(() => kindStyle(props.item))
const itemContainer = computed(() => container(props.item))
const itemContainerLine = computed(() => containerLine(props.item))
const itemByline = computed(() => byline(props.item))
const itemPullRequestMetadata = computed(() => pullRequestMetadata(props.item))
const hasPullRequestStatus = computed(() => {
  const ci = itemPullRequestMetadata.value?.ci
  return ci != null && ci !== 'none'
})
const hasPullRequestReview = computed(() => {
  const review = itemPullRequestMetadata.value?.review
  return review === 'approved' || review === 'draft'
})
const hasPullRequestSummary = computed(() => hasPullRequestReview.value || hasPullRequestStatus.value)

// The row's "…" menu (also opened by right-click). Anchored under the kebab;
// flipped upward when the row sits too close to the bottom of the window for
// the menu to fit below.
const root = ref<HTMLElement | null>(null)
const menuToggle = ref<HTMLElement | null>(null)
const menuOpen = ref(false)
const menuFlip = ref(false)

function openMenu(): void {
  const rect = root.value?.getBoundingClientRect()
  menuFlip.value = rect != null && window.innerHeight - rect.bottom < 320 && rect.top > 320
  menuOpen.value = true
}
function toggleMenu(): void {
  if (menuOpen.value) menuOpen.value = false
  else openMenu()
}

function selectRow(): void {
  if (props.selectionMode) emit('toggle-selection')
  else emit('select')
}

function activateRow(): void {
  if (props.selectionMode) emit('toggle-selection')
  else emit('activate')
}
</script>

<template>
  <!-- Not a <button>: the hover-action pill nests real buttons, which is
       invalid inside one. The div keeps the row focusable, and Enter/Space are
       the keyboard's double-click (`.self` so pill keystrokes don't reach it). -->
  <div ref="root" class="feed-item" :class="{ selected: !selectionMode && selected, 'multi-selected': selectionMode && checked, 'menu-open': menuOpen }" :role="selectionMode ? 'checkbox' : 'button'" tabindex="0" :aria-checked="selectionMode ? checked : undefined" :aria-label="selectionMode ? `Select ${item.title}` : undefined" :data-id="item.externalId" :data-inbox-id="item.id" data-testid="feed-item" @click="selectRow" @dblclick="activateRow" @keydown.enter.self.prevent="activateRow" @keydown.space.self.prevent="activateRow" @contextmenu.prevent="selectionMode ? undefined : openMenu()">
    <div class="relative flex items-start gap-3">
      <span v-if="selectionMode" class="selection-check" aria-hidden="true" data-testid="feed-item-checkbox">
        <span class="selection-box" :class="{ checked }"><IconCheck v-if="checked" class="size-3" :stroke-width="3" /></span>
      </span>
      <span v-else class="source-badge" :data-source="item.sourceKind" data-testid="source-badge"><SourceMark :icon="sourceMark" :image="sourceMarkImage" class="size-4" /></span>
      <div class="min-w-0 flex-1">
        <div class="flex items-baseline gap-2.5"><div class="min-w-0 flex-1 truncate text-left text-[13.5px] leading-[1.35]" :class="item.unread ? 'font-semibold text-text' : 'font-normal text-text-2'" data-testid="item-title">{{ item.title }}</div><div class="meta-right flex shrink-0 items-center"><span class="font-mono text-[11px] text-text-4">{{ relativeAge(item.lastEventAt) }}</span></div></div>
        <div class="mt-[5px] flex min-w-0 items-center gap-2"><span v-if="archived && item.archivedReason" class="type-pill type-pill-neutral" data-testid="archive-reason">{{ item.archivedReason }}</span><span v-if="trash && item.ignoredAt != null" class="type-pill type-pill-neutral" data-testid="ignored-pill">ignored</span><span class="type-pill" :class="'type-pill-' + itemKindStyle" data-testid="type-pill" :data-kind="itemKind">{{ itemKindLabel }}</span><span class="min-w-0 flex-1 truncate font-mono text-[11px] text-text-3">{{ presentation.sourceLabel }}<template v-if="itemContainer"> · {{ itemContainerLine }}</template></span><PullRequestMetadata class="ml-auto" :item="item" mode="changes" /></div>
        <div v-if="itemByline || hasPullRequestSummary" class="mt-[5px] flex items-center gap-1.5 truncate text-left text-[12px] leading-[1.4] text-text-3" data-testid="item-byline"><span v-if="itemByline" class="truncate text-text-2">{{ itemByline }}</span><IconDot v-if="itemByline && hasPullRequestReview" class="-mx-1.5 size-4 text-text-2" aria-hidden="true" data-testid="metadata-separator" /><PullRequestMetadata v-if="hasPullRequestReview" :item="item" mode="review" /><PullRequestMetadata v-if="hasPullRequestStatus" :item="item" mode="status" class="ml-1" /></div>
      </div>
    </div>
    <!-- Hover actions replace the row metadata so they never cover the title. -->
    <div v-if="!selectionMode" class="hover-actions" data-testid="row-hover-actions" @click.stop @dblclick.stop>
      <button v-if="trash" class="hover-action" type="button" title="Stop ignoring" aria-label="Stop ignoring" data-testid="row-restore" @click="emit('toggle-ignored')"><IconEye class="size-[15px]" /></button>
      <button v-else class="hover-action" type="button" :title="item.archivedAt ? 'Move to inbox' : 'Archive'" :aria-label="item.archivedAt ? 'Move to inbox' : 'Archive'" data-testid="row-archive" @click="emit('toggle-archive')"><IconArchive class="size-[15px]" /></button>
      <button v-if="item.url" class="hover-action" type="button" title="Open in browser" aria-label="Open in browser" data-testid="row-open" @click="emit('open-browser')"><IconExternalLink class="size-[15px]" /></button>
      <div class="relative">
        <button ref="menuToggle" class="hover-action" type="button" title="More actions" aria-label="More actions" aria-haspopup="menu" :aria-expanded="menuOpen" data-testid="row-menu-toggle" @click="toggleMenu()"><IconEllipsisVertical class="size-[15px]" /></button>
        <ItemActionMenu
          v-if="menuOpen"
          :item="item"
          :flip="menuFlip"
          :ignore="[menuToggle]"
          testid="row-menu"
          @close="menuOpen = false"
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
  </div>
</template>

<style scoped>
/* user-select: none so the double-click that opens a row does not also leave a
   word of its title selected. */
.feed-item { position: relative; width: 100%; padding: 13px 16px 13px 18px; border-bottom: 1px solid var(--color-row); cursor: pointer; text-align: left; user-select: none; }
.feed-item:hover, .feed-item.menu-open { background: var(--color-row-hover); }.feed-item:focus-visible { outline: 2px solid var(--color-accent); outline-offset: -2px; }.feed-item.selected::after, .feed-item.multi-selected::after { content: ''; position: absolute; inset: 0; background: var(--color-selection); pointer-events: none; }.source-badge { display: inline-flex; flex: none; align-items: center; justify-content: center; width: 30px; height: 30px; margin-top: 1px; border: 1px solid var(--color-strong); border-radius: 8px; background: var(--color-chip); color: var(--color-text); }.selection-check { display: inline-flex; flex: none; align-items: center; justify-content: center; width: 30px; height: 30px; margin-top: 1px; }.selection-box { display: inline-flex; width: 18px; height: 18px; align-items: center; justify-content: center; border: 1px solid var(--color-strong); border-radius: 5px; background: var(--color-app); }.selection-box.checked { border-color: var(--color-accent); background: var(--color-accent); color: var(--color-accent-contrast); }.type-pill { display: inline-flex; flex: none; align-items: center; border-radius: 4px; padding: 2px 7px; font-family: var(--font-mono); font-size: 10px; font-weight: 600; letter-spacing: .02em; }.type-pill-pr { background: var(--color-kind-pr-tint); color: var(--color-kind-pr); }.type-pill-issue { background: var(--color-kind-issue-tint); color: var(--color-kind-issue); }.type-pill-neutral { background: var(--color-chip); color: var(--color-text-2); }
.meta-right { transition: opacity .1s ease; }
.hover-actions { position: absolute; top: 8px; right: 12px; z-index: 10; display: flex; gap: 2px; padding: 3px; border: 1px solid var(--color-strong); border-radius: 8px; background: var(--color-pane); box-shadow: 0 6px 18px -8px rgb(0 0 0 / .4); opacity: 0; pointer-events: none; transition: opacity .1s ease; }
.feed-item:hover .hover-actions, .feed-item:focus-within .hover-actions, .feed-item.menu-open .hover-actions { opacity: 1; pointer-events: auto; }
.feed-item:hover .meta-right, .feed-item:focus-within .meta-right, .feed-item.menu-open .meta-right { opacity: 0; pointer-events: none; }
.hover-action { display: inline-flex; align-items: center; justify-content: center; width: 26px; height: 26px; border-radius: 6px; color: var(--color-text-2); cursor: pointer; }
.hover-action:hover, .hover-action[aria-expanded="true"] { background: var(--color-hover); color: var(--color-text); }
.hover-action:focus-visible { outline: 2px solid var(--color-accent); outline-offset: -2px; }
@media (prefers-reduced-motion: reduce) { .hover-actions, .meta-right { transition: none; } }
</style>
