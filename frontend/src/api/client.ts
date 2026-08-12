import { env } from '../lib/env'

export class ApiError extends Error {
  status: number
  code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

let accessToken: string | null = null
let unauthorizedHandler: () => void = () => {}

export function setAccessToken(token: string | null): void {
  accessToken = token
}

export function getAccessToken(): string | null {
  return accessToken
}

export function setUnauthorizedHandler(handler: () => void): void {
  unauthorizedHandler = handler
}

interface ApiFetchOptions extends Omit<RequestInit, 'body'> {
  body?: unknown
  /** Skip the 401-refresh-retry cycle — used by the refresh call itself to avoid recursion. */
  skipAuthRetry?: boolean
}

interface BackendErrorBody {
  error?: { code?: string; message?: string }
}

async function parseErrorBody(response: Response): Promise<BackendErrorBody> {
  try {
    return await response.json()
  } catch {
    return {}
  }
}

async function performFetch(path: string, options: ApiFetchOptions): Promise<Response> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options.headers as Record<string, string> | undefined),
  }
  if (accessToken) {
    headers.Authorization = `Bearer ${accessToken}`
  }

  return fetch(`${env.API_URL}${path}`, {
    ...options,
    headers,
    credentials: 'include',
    body: options.body !== undefined ? JSON.stringify(options.body) : undefined,
  })
}

export async function apiFetch<T>(path: string, options: ApiFetchOptions = {}): Promise<T> {
  const response = await performFetch(path, options)

  if (response.status === 401 && !options.skipAuthRetry) {
    const refreshed = await tryRefresh()
    if (refreshed) {
      const retryResponse = await performFetch(path, options)
      return handleResponse<T>(retryResponse)
    }
    unauthorizedHandler()
  }

  return handleResponse<T>(response)
}

async function handleResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const body = await parseErrorBody(response)
    throw new ApiError(
      response.status,
      body.error?.code ?? 'unknown_error',
      body.error?.message ?? response.statusText,
    )
  }
  if (response.status === 204) {
    return undefined as T
  }
  return response.json() as Promise<T>
}

async function tryRefresh(): Promise<boolean> {
  const response = await performFetch('/auth/refresh', { method: 'POST', skipAuthRetry: true })
  if (!response.ok) {
    setAccessToken(null)
    return false
  }
  const body = (await response.json()) as { access_token: string }
  setAccessToken(body.access_token)
  return true
}
