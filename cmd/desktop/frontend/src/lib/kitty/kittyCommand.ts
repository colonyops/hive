/** One Kitty graphics command, with the protocol's defaults for absent keys. */
export interface KittyCommand {
  /** The keys the command actually carried. */
  keys: ReadonlySet<string>
  action: string
  format: number
  medium: string
  compression: string
  id: number
  number: number
  placement: number
  more: boolean
  quiet: number
  /** Source width and height for raw pixel formats. */
  sourceWidth: number
  sourceHeight: number
  columns: number
  rows: number
  /** 1 leaves the cursor where it is. Recorded only: this renderer never moves the cursor. */
  cursorMovement: number
  cropX: number
  cropY: number
  cropWidth: number
  cropHeight: number
  offsetX: number
  offsetY: number
  z: number
  unicode: number
  target: string
  payload: string
}

const LETTER_KEYS: Record<string, 'action' | 'medium' | 'compression' | 'target'> = {
  a: 'action',
  t: 'medium',
  o: 'compression',
  d: 'target',
}

type NumericField = {
  [K in keyof KittyCommand]: KittyCommand[K] extends number ? K : never
}[keyof KittyCommand]

const NUMBER_KEYS: Record<string, NumericField> = {
  f: 'format',
  i: 'id',
  I: 'number',
  p: 'placement',
  q: 'quiet',
  s: 'sourceWidth',
  v: 'sourceHeight',
  c: 'columns',
  r: 'rows',
  C: 'cursorMovement',
  x: 'cropX',
  y: 'cropY',
  w: 'cropWidth',
  h: 'cropHeight',
  X: 'offsetX',
  Y: 'offsetY',
  z: 'z',
  U: 'unicode',
}

const INTEGER = /^-?\d+$/

/** Parses `key=value,...;payload`. Unknown keys and malformed values are skipped. */
export function parseKittyCommand(data: string): KittyCommand {
  const separator = data.indexOf(';')
  const control = separator === -1 ? data : data.slice(0, separator)
  const keys = new Set<string>()
  const command: KittyCommand = {
    keys,
    action: 't',
    format: 32,
    medium: 'd',
    compression: '',
    id: 0,
    number: 0,
    placement: 0,
    more: false,
    quiet: 0,
    sourceWidth: 0,
    sourceHeight: 0,
    columns: 0,
    rows: 0,
    cursorMovement: 0,
    cropX: 0,
    cropY: 0,
    cropWidth: 0,
    cropHeight: 0,
    offsetX: 0,
    offsetY: 0,
    z: 0,
    unicode: 0,
    target: 'a',
    payload: separator === -1 ? '' : data.slice(separator + 1),
  }
  for (const pair of control.split(',')) {
    const equals = pair.indexOf('=')
    if (equals !== 1) continue
    const key = pair[0]
    const value = pair.slice(2)
    const letter = LETTER_KEYS[key]
    if (letter) {
      if (value.length !== 1) continue
      command[letter] = value
    } else if (key === 'm') {
      if (!INTEGER.test(value)) continue
      command.more = Number(value) === 1
    } else {
      const field = NUMBER_KEYS[key]
      if (!field || !INTEGER.test(value)) continue
      command[field] = Number(value)
    }
    keys.add(key)
  }
  return command
}
