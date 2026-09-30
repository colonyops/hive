import { getTerminalEndpoint } from './terminalClient'

async function request(path: string, body: FormData | object, signal: AbortSignal): Promise<Response> {
  const endpoint = await getTerminalEndpoint()
  const form = body instanceof FormData
  const response = await fetch(`${endpoint.httpBaseURL}/api/terminal/${path}`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${endpoint.token}`, ...(form ? {} : { 'Content-Type': 'application/json' }) },
    body: form ? body : JSON.stringify(body),
    signal,
  })
  if (!response.ok) {
    const error = await response.json().catch(() => null)
    throw new Error(error?.message || 'Could not prepare the image.')
  }
  return response
}

export async function prepareTerminalImages(input: File[] | string[], signal: AbortSignal): Promise<string[]> {
  let response: Response
  if (typeof input[0] === 'string') {
    response = await request('images/paths', { paths: input }, signal)
  } else {
    const form = new FormData()
    for (const file of input as File[]) form.append('images', file)
    response = await request('images/upload', form, signal)
  }
  return (await response.json()).pastes
}

export async function pasteTerminalImage(slug: string, paneId: string, text: string, signal: AbortSignal): Promise<void> {
  await request('panes/paste', { slug, paneId, text }, signal)
}
