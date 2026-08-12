import { describe, expect, it, vi, beforeEach } from 'vitest'
import { registerUser, loginUser, refreshSession, logoutUser } from './auth'
import * as client from './client'

vi.mock('./client', async () => {
  const actual = await vi.importActual<typeof import('./client')>('./client')
  return { ...actual, apiFetch: vi.fn() }
})

describe('auth API', () => {
  beforeEach(() => {
    vi.mocked(client.apiFetch).mockReset()
  })

  it('registerUser posts to /auth/register and returns the auth result', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue({
      access_token: 'tok',
      user: { id: '1', name: 'Ada', email: 'ada@example.com' },
    })

    const result = await registerUser('Ada', 'ada@example.com', 'password123')

    expect(client.apiFetch).toHaveBeenCalledWith('/auth/register', {
      method: 'POST',
      body: { name: 'Ada', email: 'ada@example.com', password: 'password123' },
      skipAuthRetry: true,
    })
    expect(result).toEqual({
      accessToken: 'tok',
      user: { id: '1', name: 'Ada', email: 'ada@example.com' },
    })
  })

  it('loginUser posts to /auth/login and returns the auth result', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue({
      access_token: 'tok',
      user: { id: '1', name: 'Ada', email: 'ada@example.com' },
    })

    const result = await loginUser('ada@example.com', 'password123')

    expect(client.apiFetch).toHaveBeenCalledWith('/auth/login', {
      method: 'POST',
      body: { email: 'ada@example.com', password: 'password123' },
      skipAuthRetry: true,
    })
    expect(result.user.email).toBe('ada@example.com')
  })

  it('refreshSession posts to /auth/refresh with skipAuthRetry', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue({
      access_token: 'tok',
      user: { id: '1', name: 'Ada', email: 'ada@example.com' },
    })

    await refreshSession()

    expect(client.apiFetch).toHaveBeenCalledWith('/auth/refresh', { method: 'POST', skipAuthRetry: true })
  })

  it('logoutUser posts to /auth/logout with skipAuthRetry', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue(undefined)

    await logoutUser()

    expect(client.apiFetch).toHaveBeenCalledWith('/auth/logout', { method: 'POST', skipAuthRetry: true })
  })
})
