<script setup lang="ts">
import { computed } from 'vue'
import { buildFileTree, type FileNode, type TreeData, type TreeStatus } from '../../lib/fenceData'
import { linkTarget } from '../../lib/document'
import FenceCard from './FenceCard.vue'
import FenceTree, { type FenceTreeItem } from './FenceTree.vue'
import type { FenceProps } from './registry'

const props = defineProps<FenceProps<TreeData>>()

interface TreeItem extends FenceTreeItem {
  node: FileNode
  children: TreeItem[]
}

const letters: Record<TreeStatus, string> = { added: 'A', modified: 'M', deleted: 'D', renamed: 'R' }

function toItem(node: FileNode): TreeItem {
  return { id: node.path, node, children: node.children.map(toItem) }
}

const items = computed(() => buildFileTree(props.data).map(toItem))

const counts = computed(() => {
  const count = { added: 0, modified: 0, deleted: 0 }
  for (const file of props.data.files) {
    if (file.status === 'added') count.added++
    else if (file.status === 'deleted') count.deleted++
    else if (file.status === 'modified' || file.status === 'renamed') count.modified++
  }
  return count
})

function docLink(node: FileNode): boolean {
  return node.kind === 'file' && linkTarget(node.path, props.ctx.docPath, props.ctx.paths)?.kind === 'document'
}
</script>

<template>
  <FenceCard
    kind="Files"
    :title="`${data.files.length} ${data.files.length === 1 ? 'file' : 'files'}`"
    :source="source"
    lang="hive-tree"
    :testid="testid"
  >
    <template #summary>
      <span v-if="counts.added" class="tree-fence-added" :data-testid="`${testid}-added`">+{{ counts.added }}</span>
      <span v-if="counts.modified" class="tree-fence-modified" :data-testid="`${testid}-modified`"
        >~{{ counts.modified }}</span
      >
      <span v-if="counts.deleted" class="tree-fence-deleted" :data-testid="`${testid}-deleted`"
        >&minus;{{ counts.deleted }}</span
      >
    </template>
    <FenceTree :items="items" label="Files" :testid="`${testid}-tree`">
      <template #row="{ item }">
        <span v-if="item.node.file?.status" class="tree-fence-status" :class="item.node.file.status">{{
          letters[item.node.file.status]
        }}</span>
        <button
          v-if="docLink(item.node)"
          type="button"
          class="tree-fence-name is-link"
          :data-testid="`${testid}-link-${item.node.path}`"
          @click.stop="ctx.follow(item.node.path)"
        >
          {{ item.node.name }}
        </button>
        <span
          v-else
          class="tree-fence-name"
          :class="{ 'is-deleted': item.node.file?.status === 'deleted', 'is-dir': item.node.kind === 'dir' }"
          >{{ item.node.name }}{{ item.node.kind === 'dir' ? '/' : '' }}</span
        >
        <span v-if="item.node.file?.from" class="tree-fence-note">from {{ item.node.file.from }}</span>
        <span v-if="item.node.file?.note" class="tree-fence-note">{{ item.node.file.note }}</span>
      </template>
    </FenceTree>
  </FenceCard>
</template>

<style scoped>
.tree-fence-added {
  color: var(--color-severity-success);
}
.tree-fence-modified {
  color: var(--color-severity-warning);
}
.tree-fence-deleted {
  color: var(--color-severity-error);
}
.tree-fence-status {
  flex: none;
  width: 12px;
  font: 600 11px/1 var(--font-mono);
  text-align: center;
}
.tree-fence-status.added {
  color: var(--color-severity-success);
}
.tree-fence-status.modified,
.tree-fence-status.renamed {
  color: var(--color-severity-warning);
}
.tree-fence-status.deleted {
  color: var(--color-severity-error);
}
.tree-fence-name {
  min-width: 0;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--color-text);
  font: 12.5px/1.6 var(--font-mono);
  white-space: nowrap;
}
.tree-fence-name.is-dir {
  color: var(--color-text-3);
}
.tree-fence-name.is-deleted {
  color: var(--color-text-3);
  text-decoration: line-through;
}
.tree-fence-name.is-link {
  cursor: pointer;
  text-decoration: underline;
  text-decoration-color: var(--color-accent);
  text-underline-offset: 3px;
}
.tree-fence-note {
  min-width: 0;
  overflow: hidden;
  color: var(--color-text-3);
  font-size: 12.5px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
