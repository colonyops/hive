// The UX half of an action's declared inputs. Go owns the contract — it
// validates every invocation against the catalog — so this exists to fill the
// form and to say what is wrong before a round trip, never as the gate.
import type { InputSpec } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/actions/models'

export type ActionInputValues = Record<string, string>

export function inputLabel(spec: InputSpec): string {
  return spec.label.trim() || spec.name
}

export function initialActionInputs(specs: InputSpec[]): ActionInputValues {
  const values: ActionInputValues = {}
  for (const spec of specs) values[spec.name] = spec.default
  return values
}

export function validateActionInputs(specs: InputSpec[], values: ActionInputValues): string | null {
  for (const spec of specs) {
    const value = values[spec.name] ?? ''
    if (spec.required && !value.trim()) return `${inputLabel(spec)} is required.`
    if (spec.type === 'select' && value && !(spec.options ?? []).includes(value)) return `${inputLabel(spec)} is not one of its options.`
  }
  return null
}
