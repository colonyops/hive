(() => {
  const token = window.__hiveToken
  delete window.__hiveToken
  let nextID = 0
  const pending = new Map()
  const surface = new URLSearchParams(location.search).get('surface') ?? 'page'
  const listeners = { message: new Set(), session: new Set() }

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
      for (const listener of listeners[event.data.event] ?? []) listener(event.data.data)
      return
    }
    if (!pending.has(event.data.id)) return
    const { resolve, reject } = pending.get(event.data.id)
    pending.delete(event.data.id)
    if ('error' in event.data) reject(new Error(event.data.error))
    else resolve(event.data.result)
  })

  function call(method, args = {}) {
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
      return () => listeners[event].delete(listener)
    },
    broadcast: (data) => call('broadcast', { data }),
    open: () => call('open'),
    dropdown: (options = {}) => call('dropdown', options),
    resize: (size) => call('resize', size),
    command: (id) => call('command', { id }),
    commands: () => call('commands'),
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

  call('theme').then(applyTheme, () => {})

  // The frame has an opaque origin, where the real localStorage throws. Generated
  // pages reach for it constantly, so this one works the same and persists per applet.
  const store = new Map(Object.entries(window.__hiveStorage ?? {}))
  delete window.__hiveStorage
  let saving = null
  const save = () => {
    clearTimeout(saving)
    saving = setTimeout(() => call('saveStorage', { encoded: JSON.stringify(Object.fromEntries(store)) }), 200)
  }
  // Every frame of an applet keeps its own copy and saves all of it, so each change
  // crosses to the others at once. Otherwise a frame's next save would undo theirs.
  const changed = (key, value) => {
    save()
    void call('storageChanged', { key, value })
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

  // Keys in a frame never reach the app, so Escape closes the applet from here. A page
  // that wants Escape for itself calls preventDefault, which is only settled once its
  // own listeners have run.
  //
  // A panel is modal, so its keys stay its own. A title bar page sits beside the app,
  // and one click into it would otherwise leave every shortcut dead until the user
  // clicked elsewhere, so it hands on what a chord would be.
  addEventListener('keydown', (event) => {
    const chord = surface !== 'page' && (event.metaKey || event.ctrlKey)
    if (event.key !== 'Escape' && !chord) return
    setTimeout(() => {
      if (event.defaultPrevented) return
      if (event.key === 'Escape') void call('close')
      else {
        const { key, code, metaKey, ctrlKey, altKey, shiftKey } = event
        void call('key', { key, code, metaKey, ctrlKey, altKey, shiftKey })
      }
    })
  })
})()
