/** A stand-in EventSource: specs read the opened streams and emit events by hand. */
export class FakeEventSource extends EventTarget {
  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSED = 2
  static instances: FakeEventSource[] = []

  readonly url: string
  readyState = FakeEventSource.OPEN

  constructor(url: string | URL) {
    super()
    this.url = String(url)
    FakeEventSource.instances.push(this)
  }

  emit(name: string, data: unknown = {}): void {
    this.dispatchEvent(new MessageEvent(name, { data: JSON.stringify(data) }))
  }

  fail(closed = false): void {
    if (closed) this.readyState = FakeEventSource.CLOSED
    this.dispatchEvent(new Event('error'))
  }

  close(): void {
    this.readyState = FakeEventSource.CLOSED
  }

  static latest(): FakeEventSource {
    const last = FakeEventSource.instances.at(-1)
    if (!last) throw new Error('no EventSource opened')
    return last
  }
}
