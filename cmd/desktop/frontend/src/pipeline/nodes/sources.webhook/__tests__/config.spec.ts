import { describe, expect, it } from 'vitest'
import { defaults, freshConfig, randomPath, randomSecret, role, type, validate, type Config } from '../config'

describe('sources.webhook config', () => {
  it('is a backend-run source node', () => {
    expect(type).toBe('sources.webhook')
    expect(role).toBe('source')
    expect(defaults).toEqual({ path: '' })
  })
})

describe('sources.webhook generators', () => {
  it('generates paths that pass validation and differ between calls', () => {
    const paths = new Set(Array.from({ length: 25 }, () => randomPath()))
    expect(paths.size).toBe(25)
    for (const path of paths) {
      expect(validate({ path })).toEqual([])
    }
  })

  it('generates 32-character secrets that pass validation and differ between calls', () => {
    const secrets = new Set(Array.from({ length: 25 }, () => randomSecret()))
    expect(secrets.size).toBe(25)
    for (const secret of secrets) {
      expect(secret).toHaveLength(32)
      expect(validate({ path: 'ci', secret })).toEqual([])
    }
  })

  it('seeds a fresh node with a generated path and no secret', () => {
    const seed = freshConfig()
    expect(validate({ ...defaults, ...seed })).toEqual([])
    expect(seed).not.toHaveProperty('secret')
  })
})

describe('sources.webhook validate', () => {
  const ok = (config: Config) => expect(validate(config)).toEqual([])
  const bad = (config: Config, fragment: string) =>
    expect(validate(config).join('\n')).toContain(fragment)

  it('accepts slug paths, nested paths, and printable secrets', () => {
    ok({ path: 'ci-alerts' })
    ok({ path: 'ci/deploys/prod_1' })
    ok({ path: 'ci', secret: 's3cret-token_9' })
  })

  it('requires a path', () => {
    bad({ path: '' }, 'requires a path')
    bad({ path: '   ' }, 'requires a path')
  })

  it('rejects malformed paths', () => {
    bad({ path: 'CI' }, 'lowercase slug')
    bad({ path: '/ci' }, 'lowercase slug')
    bad({ path: 'ci/' }, 'lowercase slug')
    bad({ path: 'ci//x' }, 'lowercase slug')
    bad({ path: '-ci' }, 'lowercase slug')
    bad({ path: 'ci alerts' }, 'lowercase slug')
    bad({ path: 'a'.repeat(129) }, 'caps at 128')
  })

  it('rejects malformed secrets', () => {
    bad({ path: 'ci', secret: 's'.repeat(129) }, 'secret caps at 128')
    bad({ path: 'ci', secret: 'no spaces' }, 'printable ASCII')
  })

  it('accepts feed icons and rejects unknown ones', () => {
    ok({ path: 'ci', icon: 'bell' })
    ok({ path: 'ci', icon: 'webhook' })
    bad({ path: 'ci', icon: 'no-such-icon' }, 'not a supported feed icon')
  })

  it('accepts a well-formed image hash and rejects a malformed one', () => {
    ok({ path: 'ci', image: '0123456789abcdef0123456789abcdef' })
    bad({ path: 'ci', image: '0123456789ABCDEF0123456789abcdef' }, 'not a valid mark reference')
    bad({ path: 'ci', image: 'not-a-hash' }, 'not a valid mark reference')
  })
})
