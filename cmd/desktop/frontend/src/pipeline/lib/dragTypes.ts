// The dataTransfer MIME type used to drag a node type from NodePalette.vue
// onto FlowsCanvas.vue's drop target. Shared here (rather than declared in
// each component) so the dragstart `setData` and the drop `getData` can
// never drift apart.
export const NODE_TYPE_MIME = 'application/x-hive-node-type'
