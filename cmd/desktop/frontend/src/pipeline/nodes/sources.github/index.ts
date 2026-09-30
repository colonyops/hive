// App-registry entry (D2) — never imports runtime.ts. sources.github has no
// runtime.ts at all: role 'source' means "backend-run", so there is no
// worker-side code for this type to keep out of the app chunk (unlike the
// processor types, where this matters).
import editor from './editor.vue'
import help from '@nodedocs/sources.github.md?raw'
import { accentToken, category, defaults, glyph, logoMark, label, role, tint, type, validate } from './config'
import { defineNodeType } from '../../nodeType'

export default defineNodeType({
  type,
  label,
  category,
  role,
  glyph,
  logoMark,
  accentToken,
  tint,
  defaults,
  outputs: 1,
  validate,
  editor,
  help,
})
