// App-registry entry (D2) — never imports runtime.ts. github-source has no
// runtime.ts at all: role 'source' means "backend-run", so there is no
// worker-side code for this type to keep out of the app chunk (unlike the
// processor types, where this matters).
import editor from './editor.vue'
import help from './help.md?raw'
import { accentToken, category, defaults, glyph, label, role, tint, type, validate } from './config'
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
  outputs: 1,
  validate,
  editor,
  help,
})
