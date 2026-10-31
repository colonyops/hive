(() => {
  const token = window.__hiveToken
  delete window.__hiveToken
  let nextID = 0
  const pending = new Map()
  const surface = new URLSearchParams(location.search).get('surface') ?? 'page'
  const listeners = { message: new Set(), session: new Set(), command: new Set() }
  // A command can be why this page opened, and arrive before its scripts have listened.
  const heldCommands = []

  const applyTheme = (theme) => {
    for (const [name, value] of Object.entries(theme)) document.documentElement.style.setProperty(name, value)
  }

  addEventListener('message', (event) => {
    if (event.source !== parent || event.data?.hiveApplet !== 1) return
    if (event.data.theme) {
      applyTheme(event.data.theme)
      return
    }
    if (event.data.storage) {
      applyStorage(event.data.storage)
      return
    }
    if (event.data.event) {
      if (event.data.event === 'command' && listeners.command.size === 0) {
        heldCommands.push(event.data.data)
        return
      }
      for (const listener of listeners[event.data.event] ?? []) listener(event.data.data)
      return
    }
    if (!pending.has(event.data.id)) return
    const { resolve, reject } = pending.get(event.data.id)
    pending.delete(event.data.id)
    if ('error' in event.data) reject(new Error(event.data.error))
    else resolve(event.data.result)
  })

  // Hive frames a page directly under its own window. Anything else above this page may be
  // a foreign page framing it to read the token off the first message, so nothing is sent.
  const framedByHive = parent !== window && parent === top

  function call(method, args = {}) {
    if (!framedByHive) return Promise.reject(new Error('window.hive works only in a page Hive opened itself.'))
    return new Promise((resolve, reject) => {
      const id = ++nextID
      pending.set(id, { resolve, reject })
      parent.postMessage({ hiveApplet: 1, token, id, method, args }, '*')
    })
  }

  async function pump(handle, options) {
    for (;;) {
      const out = await call('spawnRead', { handle })
      if (out.stdout) options.onStdout?.(out.stdout)
      if (out.stderr) options.onStderr?.(out.stderr)
      if (out.done) {
        options.onExit?.(out.code)
        return
      }
    }
  }

  window.hive = {
    surface,
    on(event, listener) {
      if (!listeners[event]) throw new Error(`hive has no event "${event}"`)
      listeners[event].add(listener)
      if (event === 'command') heldCommands.splice(0).forEach((id) => listener(id))
      return () => listeners[event].delete(listener)
    },
    broadcast: (data) => call('broadcast', { data }),
    open: () => call('open'),
    dropdown: (options = {}) => call('dropdown', options),
    resize: (size) => call('resize', size),
    command: (id) => call('command', { id }),
    commands: () => call('commands'),
    setCommands: (rows) => call('setCommands', { rows }),
    openPalette: (query = '') => call('openPalette', { query }),
    navigate: (path) => call('navigate', { path }),
    context: () => call('context'),
    exec: (command, options = {}) => call('exec', { command, ...options }),
    async spawn(command, options = {}) {
      const { onStdout, onStderr, onExit, ...request } = options
      const handle = await call('spawn', { command, ...request })
      pump(handle, { onStdout, onStderr, onExit }).catch(() => {})
      return {
        write: (data) => call('spawnWrite', { handle, data }),
        kill: () => call('spawnKill', { handle }),
      }
    },
    readFile: (path) => call('readFile', { path }),
    writeFile: (path, content) => call('writeFile', { path, content }),
    terminal: (command, options = {}) => call('terminal', { command, ...options }),
    toast: (message, options = {}) => call('toast', { message, ...options }),
    copy: (text) => call('copy', { text }),
    authoringPrompt: (brief) => call('authoringPrompt', { brief }),
    close: () => call('close'),
  }

  const notify = (method, args) => call(method, args).catch(() => {})

  call('theme').then(applyTheme, () => {})
  notify('ready')

  // The frame has an opaque origin, where the real localStorage throws. Generated
  // pages reach for it constantly, so this one works the same and persists per applet.
  const store = new Map(Object.entries(window.__hiveStorage ?? {}))
  delete window.__hiveStorage
  let saving = null
  const save = () => {
    clearTimeout(saving)
    saving = setTimeout(() => notify('saveStorage', { encoded: JSON.stringify(Object.fromEntries(store)) }), 200)
  }
  // Every frame of an applet keeps its own copy and saves all of it, so each change
  // crosses to the others at once. Otherwise a frame's next save would undo theirs.
  const changed = (key, value) => {
    save()
    notify('storageChanged', { key, value })
  }
  const applyStorage = ({ key, value }) => {
    if (key === null) store.clear()
    else if (value === null) store.delete(key)
    else store.set(key, value)
    dispatchEvent(new StorageEvent('storage', { key, newValue: value }))
  }
  const storage = {
    get length() {
      return store.size
    },
    key: (index) => [...store.keys()][index] ?? null,
    getItem: (key) => store.get(String(key)) ?? null,
    setItem(key, value) {
      store.set(String(key), String(value))
      changed(String(key), String(value))
    },
    removeItem(key) {
      store.delete(String(key))
      changed(String(key), null)
    },
    clear() {
      store.clear()
      changed(null, null)
    },
  }
  Object.defineProperty(window, 'localStorage', { value: storage, configurable: true })

  // Keys in a frame never reach the app, so this hands on what a chord would be: without
  // it one click into a page would leave every shortcut dead until the user clicked
  // elsewhere. Escape closes the applet from here. A page that wants either for itself
  // calls preventDefault, which is only settled once its own listeners have run.
  //
  // The rest of a chord sequence is a bare key, so it crosses only while the app says one
  // is waiting.
  let sequencePending = false
  addEventListener('keydown', (event) => {
    if (['Meta', 'Control', 'Alt', 'Shift'].includes(event.key)) return
    const chord = event.metaKey || event.ctrlKey || event.altKey
    const continuing = sequencePending
    if (event.key !== 'Escape' && !chord && !continuing) return
    sequencePending = false
    setTimeout(() => {
      if (event.defaultPrevented) return
      if (event.key === 'Escape' && !continuing) {
        notify('close')
        return
      }
      const { key, code, metaKey, ctrlKey, altKey, shiftKey } = event
      call('key', { key, code, metaKey, ctrlKey, altKey, shiftKey, continuing }).then(
        (pending) => {
          sequencePending = pending
        },
        () => {},
      )
    })
  })
})()
