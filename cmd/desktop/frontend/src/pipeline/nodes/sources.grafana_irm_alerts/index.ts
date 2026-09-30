import editor from './editor.vue'
import help from '@nodedocs/sources.grafana_irm_alerts.md?raw'
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
