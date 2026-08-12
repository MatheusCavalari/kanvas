import { describe, expect, it, vi, beforeEach } from 'vitest'
import { useAuthStore } from './useAuthStore'
import * as authApi from '../../api/auth'
import { ApiError } from '../../api/client'

vi.mock('../../api/auth')

const sampleUser = { id: '1', name: 'Ada', email: 'ada@example.com' }

describe('useAuthStore', () => {
  beforeEach(() => {
    useAuthStore.setState({ user: null, status: 'idle' })
    vi.resetAllMocks()
  })

  it('login: on success, sets user and status to authenticated', async () => {
    vi.mocked(authApi.loginUser).mockResolvedValue({ user: sampleUser, accessToken: 'tok' })

    await useAuthStore.getState().login('ada@example.com', 'password123')

    expect(useAuthStore.getState().user).toEqual(sampleUser)
    expect(useAuthStore.getState().status).toBe('authenticated')
  })

  it('login: on failure, leaves status unauthenticated and rethrows', async () => {
    vi.mocked(authApi.loginUser).mockRejectedValue(new ApiError(401, 'invalid_credentials', 'invalid credentials'))

    await expect(useAuthStore.getState().login('ada@example.com', 'wrong')).rejects.toThrow('invalid credentials')

    expect(useAuthStore.getState().user).toBeNull()
    expect(useAuthStore.getState().status).toBe('unauthenticated')
  })

  it('register: on success, sets user and status to authenticated', async () => {
    vi.mocked(authApi.registerUser).mockResolvedValue({ user: sampleUser, accessToken: 'tok' })

    await useAuthStore.getState().register('Ada', 'ada@example.com', 'password123')

    expect(useAuthStore.getState().user).toEqual(sampleUser)
    expect(useAuthStore.getState().status).toBe('authenticated')
  })

  it('restoreSession: on success, sets user and status to authenticated', async () => {
    vi.mocked(authApi.refreshSession).mockResolvedValue({ user: sampleUser, accessToken: 'tok' })

    await useAuthStore.getState().restoreSession()

    expect(useAuthStore.getState().user).toEqual(sampleUser)
    expect(useAuthStore.getState().status).toBe('authenticated')
  })

  it('restoreSession: on failure, sets status to unauthenticated without throwing', async () => {
    vi.mocked(authApi.refreshSession).mockRejectedValue(new ApiError(401, 'invalid_refresh_token', 'no session'))

    await expect(useAuthStore.getState().restoreSession()).resolves.toBeUndefined()

    expect(useAuthStore.getState().user).toBeNull()
    expect(useAuthStore.getState().status).toBe('unauthenticated')
  })

  it('logout: clears user and sets status to unauthenticated', async () => {
    useAuthStore.setState({ user: sampleUser, status: 'authenticated' })
    vi.mocked(authApi.logoutUser).mockResolvedValue(undefined)

    await useAuthStore.getState().logout()

    expect(useAuthStore.getState().user).toBeNull()
    expect(useAuthStore.getState().status).toBe('unauthenticated')
  })

  it('logout: still clears local state even if the backend call fails', async () => {
    useAuthStore.setState({ user: sampleUser, status: 'authenticated' })
    vi.mocked(authApi.logoutUser).mockRejectedValue(new Error('network error'))

    await useAuthStore.getState().logout()

    expect(useAuthStore.getState().user).toBeNull()
    expect(useAuthStore.getState().status).toBe('unauthenticated')
  })
})
