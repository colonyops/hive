import { ref, type Ref } from 'vue'
import type { TerminalClient, WindowState } from '../lib/terminalClient'
import type { TerminalSessionRow } from './useTerminalSessions'

// The passive per-session window listings behind "Always show windows", keyed
// by slug. A module singleton for the same reason as useTerminalSessions, and
// it also stands in for a session whose attach is still in flight, so switching
// sessions does not collapse and rebuild its subtree and tab strip.
const listings = ref<Record<string, WindowState[]>>({})

// Whether the first sweep has landed. The tree waits on it before its first
// paint: rows and their window subtrees arriving separately means painting the
// panel twice, and the second one animates and relayouts every row.
const settled = ref(false)

// A sweep is one round trip answering every session. The triggers arrive in
// bursts — a switch moves the route, the pool and the session list inside a few
// ticks — so sweeps are coalesced rather than run per trigger: overlapping
// calls collapse into a single follow-up carrying the newest rows. Uncoalesced,
// one switch cost tens of sweeps, and the attach it raced queued behind every
// one of them.
let running: Promise<void> | null = null
let next: { client: TerminalClient; rows: TerminalSessionRow[] } | null = null

function refresh(client: TerminalClient, rows: TerminalSessionRow[]): Promise<void> {
  if (running) {
    next = { client, rows }
    return running
  }
  running = drain(client, rows)
  return running
}

async function drain(client: TerminalClient, rows: TerminalSessionRow[]): Promise<void> {
  let current: { client: TerminalClient; rows: TerminalSessionRow[] } | null = { client, rows }
  try {
    while (current) {
      await sweep(current.client, current.rows)
      current = next
      next = null
    }
  } finally {
    running = null
    next = null
  }
}

// Replaces the whole map so sessions that no longer exist fall out; a write
// landing after the option was toggled off is harmless because rendering is
// gated on the option, not on this cache.
async function sweep(client: TerminalClient, rows: TerminalSessionRow[]): Promise<void> {
  try {
    listings.value = await client.listWindows(rows.map((row) => row.slug))
  } catch {
    // Keep the last-known listings: a failed sweep must not collapse every
    // session's subtree, and the next trigger will try again.
  } finally {
    settled.value = true
  }
}

export function useTerminalWindowListings(): {
  listings: Ref<Record<string, WindowState[]>>
  settled: Ref<boolean>
  refresh: (client: TerminalClient, rows: TerminalSessionRow[]) => Promise<void>
} {
  return { listings, settled, refresh }
}

export function resetTerminalWindowListingsForTests(): void {
  listings.value = {}
  settled.value = false
  running = null
  next = null
}
