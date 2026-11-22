import { ref, type Ref } from 'vue'

// The same words the server draws from, so a name reads alike whoever made it.
const adjectives = [
  'Amber',
  'Brave',
  'Calm',
  'Clever',
  'Curious',
  'Daring',
  'Eager',
  'Gentle',
  'Happy',
  'Jolly',
  'Keen',
  'Lively',
  'Lucky',
  'Merry',
  'Nimble',
  'Quiet',
  'Quick',
  'Sunny',
  'Swift',
  'Witty',
]
const animals = [
  'Badger',
  'Beaver',
  'Crane',
  'Falcon',
  'Ferret',
  'Fox',
  'Gecko',
  'Heron',
  'Koala',
  'Lemur',
  'Lynx',
  'Marten',
  'Otter',
  'Owl',
  'Panda',
  'Puffin',
  'Raven',
  'Seal',
  'Stoat',
  'Wren',
]

export const presenceColors = 12

function hash(seed: string): number {
  // FNV-1a, 32-bit.
  let h = 0x811c9dc5
  for (let i = 0; i < seed.length; i++) {
    h ^= seed.charCodeAt(i)
    h = Math.imul(h, 0x01000193) >>> 0
  }
  return h
}

/** A readable name like "Curious Otter", the same for one seed. */
export function generatedName(seed: string): string {
  const n = hash(seed)
  return `${adjectives[n % adjectives.length]} ${animals[Math.floor(n / adjectives.length) % animals.length]}`
}

const storageKey = 'hive.drive.viewer'
const maxName = 40

interface Stored {
  seed: string
  name: string
}

// Read once and validated, so a hand-edited or foreign value cannot reach the
// page; useStorage would hand it over as is.
function read(): Stored | null {
  try {
    // eslint-disable-next-line no-restricted-globals -- read once with validation
    const value = JSON.parse(localStorage.getItem(storageKey) ?? 'null') as Partial<Stored> | null
    return value && typeof value.seed === 'string' && value.seed && typeof value.name === 'string'
      ? { seed: value.seed, name: value.name }
      : null
  } catch {
    return null
  }
}

function write(value: Stored): void {
  try {
    // eslint-disable-next-line no-restricted-globals -- paired with read above
    localStorage.setItem(storageKey, JSON.stringify(value))
  } catch {
    // Private windows can refuse storage; the name then lasts for this page only.
  }
}

/** The anonymous viewer's name and color, kept across visits when storage allows. */
export function useViewerIdentity(): { name: Readonly<Ref<string>>; color: number; rename(name: string): void } {
  const seed = read()?.seed ?? crypto.randomUUID()
  const initial = read()?.name.trim().slice(0, maxName) || generatedName(seed)
  const name = ref(initial)
  write({ seed, name: initial })
  return {
    name,
    color: hash(seed) % presenceColors,
    rename(next: string): void {
      name.value = next.trim().slice(0, maxName) || generatedName(seed)
      write({ seed, name: name.value })
    },
  }
}
