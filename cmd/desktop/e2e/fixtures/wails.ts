import { readFileSync } from 'node:fs'

// The generated bindings call a service method by numeric ID. Reading the ID
// from the bindings keeps a mocked call in step with a regenerated surface.
export function bindingID(service: string, method: string): number {
  const source = readFileSync(new URL(`../../frontend/bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/${service}.ts`, import.meta.url), 'utf8')
  const match = source.match(new RegExp(`function ${method}\\([^]*?ByID\\((\\d+)\\)`))
  if (!match) throw new Error(`Missing binding ${service}.${method}`)
  return Number(match[1])
}
