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

  const receive = ({ data }) => {
    if (data.theme) {
      applyTheme(data.theme)
      return
    }
    if (data.storage) {
      applyStorage(data.storage)
      return
    }
    if (data.event) {
      if (data.event === 'command' && listeners.command.size === 0) {
        heldCommands.push(data.data)
        return
      }
      for (const listener of listeners[data.event] ?? []) listener(data.data)
      return
    }
    if (!pending.has(data.id)) return
    const { resolve, reject } = pending.get(data.id)
    pending.delete(data.id)
    if ('error' in data) reject(new Error(data.error))
    else resolve(data.result)
  }

  // Hive frames a page directly under its own window. Anything else above this page may be
  // a foreign page framing it to read the token off the first message, so nothing is sent.
  const framedByHive = parent !== window && parent === top

  // Everything after the handshake crosses this port, which a page the frame navigates to
  // does not hold. The frame's window goes with the frame, so posting to it would hand
  // whatever loads next the replies and events meant for this page.
  const channel = new MessageChannel()
  channel.port1.onmessage = receive
  if (framedByHive) parent.postMessage({ hiveApplet: 1, token, method: 'connect' }, '*', [channel.port2])

  function call(method, args = {}) {
    if (!framedByHive) return Promise.reject(new Error('window.hive works only in a page Hive opened itself.'))
    return new Promise((resolve, reject) => {
      const id = ++nextID
      pending.set(id, { resolve, reject })
      channel.port1.postMessage({ id, method, args })
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
      // A page written for a later Hive must still load here.
      if (!listeners[event]) return () => {}
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
    close() {
      saveNow()
      return call('close')
    },
  }

  const notify = (method, args) => call(method, args).catch(() => {})

  call('theme').then(applyTheme, () => {})
  notify('ready')

  // The frame has an opaque origin, where the real localStorage throws. Generated
  // pages reach for it constantly, so this one works the same and persists per applet.
  const store = new Map(Object.entries(window.__hiveStorage ?? {}))
  delete window.__hiveStorage
  let saving = null
  const writeStorage = () => {
    saving = null
    notify('saveStorage', { encoded: JSON.stringify(Object.fromEntries(store)) })
  }
  const save = () => {
    clearTimeout(saving)
    saving = setTimeout(writeStorage, 200)
  }
  // The timer dies with the page, and a write just before closing must still land.
  function saveNow() {
    if (saving === null) return
    clearTimeout(saving)
    writeStorage()
  }
  addEventListener('pagehide', saveNow)
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
        saveNow()
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
