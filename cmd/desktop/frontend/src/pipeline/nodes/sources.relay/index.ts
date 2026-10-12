import editor from './editor.vue'
import help from '@nodedocs/sources.relay.md?raw'
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
  // eslint-disable-next-line @typescript-eslint/no-unsafe-assignment -- type-aware lint does not resolve .vue module types
  editor,
  help,
})
