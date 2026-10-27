import { ref, toRaw, type Ref, type UnwrapRef } from 'vue'

/**
 * A ref that starts as a copy of a prop and then belongs to the component: a
 * form field seeded from what it edits. Later changes to the prop do not reach
 * it, so the component must mount again (v-if or :key) or watch the prop when
 * it needs a fresh copy.
 *
 * The value is deep-cloned, so editing a seeded object or array never writes
 * through to the parent's data. It must be structured-cloneable: no functions,
 * DOM nodes, or class instances that need their prototype.
 */
export function seedRef<T>(initial: () => T): Ref<UnwrapRef<T>> {
  return ref(structuredClone(unwrapDeep(initial()))) as Ref<UnwrapRef<T>>
}

// structuredClone rejects a Proxy, and a value built in the getter can hold
// reactive pieces below its top level (a mapped array of objects that keep a
// prop's nested array), which toRaw alone does not reach.
function unwrapDeep(value: unknown): unknown {
  const raw: unknown = toRaw(value)
  if (Array.isArray(raw)) return raw.map(unwrapDeep)
  if (raw !== null && typeof raw === 'object' && Object.getPrototypeOf(raw) === Object.prototype) {
    return Object.fromEntries(Object.entries(raw).map(([key, item]) => [key, unwrapDeep(item)]))
  }
  return raw
}
