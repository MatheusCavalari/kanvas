import { describe, expect, it, vi, beforeEach } from 'vitest'

describe('env', () => {
  beforeEach(() => {
    vi.unstubAllEnvs()
    vi.resetModules()
  })

  it('throws when VITE_API_URL is not set', async () => {
    vi.stubEnv('VITE_API_URL', '')
    await expect(import('./env')).rejects.toThrow(/VITE_API_URL/)
  })

  it('exposes API_URL when VITE_API_URL is set', async () => {
    vi.stubEnv('VITE_API_URL', 'http://localhost:8080')
    const { env } = await import('./env')
    expect(env.API_URL).toBe('http://localhost:8080')
  })
})
