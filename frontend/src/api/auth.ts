import { apiFetch } from './client'

export interface User {
  id: string
  name: string
  email: string
}

export interface AuthResult {
  user: User
  accessToken: string
}

interface AuthResponseBody {
  access_token: string
  user: User
}

function toAuthResult(body: AuthResponseBody): AuthResult {
  return { user: body.user, accessToken: body.access_token }
}

export async function registerUser(name: string, email: string, password: string): Promise<AuthResult> {
  const body = await apiFetch<AuthResponseBody>('/auth/register', {
    method: 'POST',
    body: { name, email, password },
    skipAuthRetry: true,
  })
  return toAuthResult(body)
}

export async function loginUser(email: string, password: string): Promise<AuthResult> {
  const body = await apiFetch<AuthResponseBody>('/auth/login', {
    method: 'POST',
    body: { email, password },
    skipAuthRetry: true,
  })
  return toAuthResult(body)
}

export async function refreshSession(): Promise<AuthResult> {
  const body = await apiFetch<AuthResponseBody>('/auth/refresh', { method: 'POST', skipAuthRetry: true })
  return toAuthResult(body)
}

export async function logoutUser(): Promise<void> {
  await apiFetch<void>('/auth/logout', { method: 'POST', skipAuthRetry: true })
}
