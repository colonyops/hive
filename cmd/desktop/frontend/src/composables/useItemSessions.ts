import { ref, type Ref } from 'vue'
import { ItemSessions } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice'
import type { ItemSessionView } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'

// The hive sessions the selected item created. Module state because one detail
// pane is on screen: the pane renders this and App.vue drives it from the
// selection, so there is nothing to keep per-instance.
//
// Read on selection and on jobs:updated rather than polled. Session creation,
// deletion and recycling all run as jobs, so that event is the moment the
// answer can have changed; a timer would ask hive about a pane nobody is
// looking at.
const sessions = ref<ItemSessionView[]>([])

// The item the list on screen belongs to, so a slow answer for a
// previously-selected item cannot paint over a faster one.
let currentItemID: number | null = null
let loadSeq = 0

async function load(itemID: number | null): Promise<void> {
  const seq = ++loadSeq
  currentItemID = itemID
  if (itemID === null) {
    sessions.value = []
    return
  }
  try {
    const found = (await ItemSessions(itemID)) ?? []
    if (seq !== loadSeq) return
    sessions.value = found
  } catch {
    // An install with no hive behind it, or a momentarily unreadable hive.db,
    // means "no sessions to show" — not an error worth a pane full of red.
    if (seq !== loadSeq) return
    sessions.value = []
  }
}

function refresh(): Promise<void> {
  return load(currentItemID)
}

export function useItemSessions(): {
  sessions: Ref<ItemSessionView[]>
  load: (itemID: number | null) => Promise<void>
  refresh: () => Promise<void>
} {
  return { sessions, load, refresh }
}

export function resetItemSessionsForTests(): void {
  ++loadSeq
  currentItemID = null
  sessions.value = []
}
