// App-registry entry (D2) — never imports runtime.ts (see runtime.ts for the
// worker-side ProcessorRuntime this pairs with, discovered separately by
// registry.ts's worker glob).
import editor from './editor.vue'
import help from './help.md?raw'
import { accentToken, category, defaults, glyph, label, outputs, role, tint, type, validate } from './config'
import { defineNodeType } from '../../nodeType'

export default defineNodeType({
  type,
  label,
  category,
  role,
  glyph,
  accentToken,
  tint,
  defaults,
  outputs,
  validate,
  editor,
  help,
})
