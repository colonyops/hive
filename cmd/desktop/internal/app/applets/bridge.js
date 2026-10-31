(() => {
  const token = window.__hiveToken
  delete window.__hiveToken
  let nextID = 0
  const pending = new Map()

  addEventListener('message', (event) => {
    if (event.source !== parent || event.data?.hiveApplet !== 1 || !pending.has(event.data.id)) return
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
    close: () => call('close'),
  }

  // The frame has an opaque origin, where the real localStorage throws. Generated
  // pages reach for it constantly, so this one works the same and persists per applet.
  const store = new Map(Object.entries(window.__hiveStorage ?? {}))
  delete window.__hiveStorage
  let saving = null
  const save = () => {
    clearTimeout(saving)
    saving = setTimeout(() => call('saveStorage', { encoded: JSON.stringify(Object.fromEntries(store)) }), 200)
  }
  const storage = {
    get length() {
      return store.size
    },
    key: (index) => [...store.keys()][index] ?? null,
    getItem: (key) => store.get(String(key)) ?? null,
    setItem(key, value) {
      store.set(String(key), String(value))
      save()
    },
    removeItem(key) {
      store.delete(String(key))
      save()
    },
    clear() {
      store.clear()
      save()
    },
  }
  Object.defineProperty(window, 'localStorage', { value: storage, configurable: true })

  // Keys in a frame never reach the app, so Escape closes the applet from here. A page
  // that wants Escape for itself calls preventDefault, which is only settled once its
  // own listeners have run.
  addEventListener('keydown', (event) => {
    if (event.key !== 'Escape') return
    setTimeout(() => {
      if (!event.defaultPrevented) void call('close')
    })
  })
})()
