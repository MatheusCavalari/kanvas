import { create } from 'zustand'
import { loginUser, registerUser, refreshSession, logoutUser, type User } from '../../api/auth'
import { setAccessToken } from '../../api/client'

export type AuthStatus = 'idle' | 'authenticated' | 'unauthenticated'

interface AuthState {
  user: User | null
  status: AuthStatus
  login: (email: string, password: string) => Promise<void>
  register: (name: string, email: string, password: string) => Promise<void>
  restoreSession: () => Promise<void>
  logout: () => Promise<void>
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  status: 'idle',

  login: async (email, password) => {
    try {
      const result = await loginUser(email, password)
      setAccessToken(result.accessToken)
      set({ user: result.user, status: 'authenticated' })
    } catch (error) {
      set({ user: null, status: 'unauthenticated' })
      throw error
    }
  },

  register: async (name, email, password) => {
    try {
      const result = await registerUser(name, email, password)
      setAccessToken(result.accessToken)
      set({ user: result.user, status: 'authenticated' })
    } catch (error) {
      set({ user: null, status: 'unauthenticated' })
      throw error
    }
  },

  restoreSession: async () => {
    try {
      const result = await refreshSession()
      setAccessToken(result.accessToken)
      set({ user: result.user, status: 'authenticated' })
    } catch {
      setAccessToken(null)
      set({ user: null, status: 'unauthenticated' })
    }
  },

  logout: async () => {
    try {
      await logoutUser()
    } catch {
      // Suppress error during logout
    } finally {
      setAccessToken(null)
      set({ user: null, status: 'unauthenticated' })
    }
  },
}))
