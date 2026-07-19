// App-registry entry (D2) — a terminal node, so there is no runtime.ts to
// keep out of the app chunk (the engine tags arriving msgs with sink()
// itself; see engine/runGraph.ts's TERMINALS map).
import editor from './editor.vue'
import help from './help.md?raw'
import { category, defaults, glyph, label, outputs, role, type, validate } from './config'
import { defineNodeType } from '../../nodeType'

export default defineNodeType({
  type,
  label,
  category,
  role,
  glyph,
  defaults,
  outputs,
  validate,
  editor,
  help,
})
