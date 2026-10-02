import editor from './editor.vue'
import help from '@nodedocs/launch-chat.md?raw'
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
  // eslint-disable-next-line @typescript-eslint/no-unsafe-assignment -- type-aware lint does not resolve .vue module types
  editor,
  help,
})
