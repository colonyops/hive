// One question whose answer becomes an authoring prompt. The prompt's words belong to
// Hive (prompts/templates/applets.tmpl), and this page only sends the answers.

const STYLES = [
  { id: 'hive', label: 'Hive', detail: 'Part of the app' },
  { id: 'website', label: 'Website', detail: 'A designed page' },
  { id: 'free', label: 'You choose', detail: 'The agent decides' },
  { id: 'terminal', label: 'Terminal', detail: 'Monospace, phosphor' },
  { id: 'editorial', label: 'Editorial', detail: 'Like a magazine' },
  { id: 'swiss', label: 'Swiss', detail: 'Strict grid, one accent' },
  { id: 'brutalist', label: 'Brutalist', detail: 'Thick borders, loud' },
  { id: 'arcade', label: 'Arcade', detail: 'Neon and pixels' },
  { id: 'reference', label: 'Feels like...', detail: 'Name an app or a mood' },
]
const NAMED = new Set(['terminal', 'editorial', 'swiss', 'brutalist', 'arcade'])

const SAVED = 'builder.answers'

function load() {
  try {
    return JSON.parse(localStorage.getItem(SAVED)) ?? {}
  } catch {
    return {}
  }
}

const answers = { style: 'hive', reference: '', ...load() }
const reference = document.getElementById('reference')
const copy = document.getElementById('copy')
const status = document.getElementById('status')

function renderChoices() {
  document.getElementById('styles').replaceChildren(
    ...STYLES.map((option) => {
      const button = document.createElement('button')
      button.type = 'button'
      button.className = 'choice'
      button.dataset.testid = `builder-style-${option.id}`
      button.setAttribute('aria-pressed', String(answers.style === option.id))
      const label = document.createElement('strong')
      label.textContent = option.label
      const detail = document.createElement('span')
      detail.textContent = option.detail
      button.append(label, detail)
      button.addEventListener('click', () => {
        answers.style = option.id
        localStorage.setItem(SAVED, JSON.stringify(answers))
        render()
        if (option.id === 'reference') reference.focus()
      })
      return button
    }),
  )
}

function render() {
  renderChoices()
  reference.hidden = answers.style !== 'reference'
  reference.value = answers.reference
  copy.disabled = answers.style === 'reference' && !answers.reference.trim()
}

function brief() {
  if (NAMED.has(answers.style)) return { style: 'named', reference: answers.style }
  return { style: answers.style, reference: answers.style === 'reference' ? answers.reference : '' }
}

async function copyPrompt() {
  if (copy.disabled) return
  try {
    await hive.copy(await hive.authoringPrompt(brief()))
    void hive.toast('Prompt copied', { severity: 'success', body: 'Paste it to an agent. It will ask what the applet should do.' })
    void hive.close()
  } catch (error) {
    status.className = 'error'
    status.textContent = error.message
  }
}

reference.addEventListener('input', () => {
  answers.reference = reference.value
  localStorage.setItem(SAVED, JSON.stringify(answers))
  copy.disabled = !answers.reference.trim()
})
copy.addEventListener('click', () => void copyPrompt())
document.addEventListener('keydown', (event) => {
  if (event.key !== 'Enter' || event.target.tagName === 'BUTTON') return
  event.preventDefault()
  void copyPrompt()
})

render()
