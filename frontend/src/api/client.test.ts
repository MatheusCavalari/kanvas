import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { apiFetch, ApiError, setAccessToken, getAccessToken, setUnauthorizedHandler } from './client'

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

describe('apiFetch', () => {
  beforeEach(() => {
    setAccessToken(null)
    setUnauthorizedHandler(() => {})
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('sends the Authorization header when an access token is set', async () => {
    setAccessToken('token-123')
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ ok: true }))
    vi.stubGlobal('fetch', fetchMock)

    await apiFetch('/boards')

    const [, init] = fetchMock.mock.calls[0]
    expect(init.headers.Authorization).toBe('Bearer token-123')
    expect(init.credentials).toBe('include')
  })

  it('does not send an Authorization header when no token is set', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ ok: true }))
    vi.stubGlobal('fetch', fetchMock)

    await apiFetch('/auth/login', { method: 'POST' })

    const [, init] = fetchMock.mock.calls[0]
    expect(init.headers.Authorization).toBeUndefined()
  })

  it('returns the parsed JSON body on success', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ hello: 'world' }))
    vi.stubGlobal('fetch', fetchMock)

    const result = await apiFetch<{ hello: string }>('/boards')

    expect(result).toEqual({ hello: 'world' })
  })

  it('throws ApiError with code/message from the backend envelope on a non-401 error', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({ error: { code: 'invalid_request', message: 'title is required' } }, 400),
    )
    vi.stubGlobal('fetch', fetchMock)

    await expect(apiFetch('/boards', { method: 'POST' })).rejects.toMatchObject({
      status: 400,
      code: 'invalid_request',
      message: 'title is required',
    })
    await expect(apiFetch('/boards', { method: 'POST' })).rejects.toBeInstanceOf(ApiError)
  })

  it('on a 401, retries once after a successful refresh, using the new token', async () => {
    setAccessToken('stale-token')
    const fetchMock = vi
      .fn()
      // 1st call: the original request, comes back 401
      .mockResolvedValueOnce(jsonResponse({ error: { code: 'unauthorized', message: 'unauthorized' } }, 401))
      // 2nd call: POST /auth/refresh, succeeds with a new token
      .mockResolvedValueOnce(
        jsonResponse({ access_token: 'fresh-token', user: { id: '1', name: 'Ada', email: 'ada@example.com' } }),
      )
      // 3rd call: retry of the original request, succeeds
      .mockResolvedValueOnce(jsonResponse({ ok: true }))
    vi.stubGlobal('fetch', fetchMock)

    const result = await apiFetch<{ ok: boolean }>('/boards')

    expect(result).toEqual({ ok: true })
    expect(fetchMock).toHaveBeenCalledTimes(3)
    expect(fetchMock.mock.calls[1][0]).toBe('http://localhost:8080/auth/refresh')
    expect(fetchMock.mock.calls[2][1].headers.Authorization).toBe('Bearer fresh-token')
    expect(getAccessToken()).toBe('fresh-token')
  })

  it('on a 401 where refresh also fails, calls the unauthorized handler and throws', async () => {
    setAccessToken('stale-token')
    const unauthorizedHandler = vi.fn()
    setUnauthorizedHandler(unauthorizedHandler)
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ error: { code: 'unauthorized', message: 'unauthorized' } }, 401))
      .mockResolvedValueOnce(
        jsonResponse({ error: { code: 'invalid_refresh_token', message: 'invalid refresh token' } }, 401),
      )
    vi.stubGlobal('fetch', fetchMock)

    await expect(apiFetch('/boards')).rejects.toMatchObject({ status: 401 })
    expect(unauthorizedHandler).toHaveBeenCalledTimes(1)
    expect(getAccessToken()).toBeNull()
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('does not attempt a refresh loop when the failing request IS the refresh call', async () => {
    setAccessToken('stale-token')
    const unauthorizedHandler = vi.fn()
    setUnauthorizedHandler(unauthorizedHandler)
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ error: { code: 'invalid_refresh_token', message: 'bad' } }, 401))
    vi.stubGlobal('fetch', fetchMock)

    await expect(apiFetch('/auth/refresh', { method: 'POST', skipAuthRetry: true })).rejects.toMatchObject({
      status: 401,
    })
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(unauthorizedHandler).not.toHaveBeenCalled()
  })
})
