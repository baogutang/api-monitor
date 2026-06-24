import type { APIError } from './types'

const TOKEN_KEY = 'api_monitor_token'

export function getApiBase(): string {
  const base = import.meta.env.VITE_API_BASE as string | undefined
  return base?.replace(/\/$/, '') ?? ''
}

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}

export function setToken(token: string | null) {
  if (token) localStorage.setItem(TOKEN_KEY, token)
  else localStorage.removeItem(TOKEN_KEY)
}

export class ApiClientError extends Error {
  code: string
  status: number
  details?: unknown

  constructor(status: number, body: APIError['error']) {
    super(body.message)
    this.code = body.code
    this.status = status
    this.details = body.details
  }
}

type RequestOptions = Omit<RequestInit, 'body'> & { body?: unknown }

let onUnauthorized: (() => void) | null = null

export function setUnauthorizedHandler(fn: () => void) {
  onUnauthorized = fn
}

export async function apiRequest<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const base = getApiBase()
  const url = `${base}${path.startsWith('/') ? path : `/${path}`}`
  const headers = new Headers(options.headers)
  if (!headers.has('Content-Type') && options.body !== undefined) {
    headers.set('Content-Type', 'application/json')
  }
  const token = getToken()
  if (token) headers.set('Authorization', `Bearer ${token}`)

  const res = await fetch(url, {
    ...options,
    headers,
    body: options.body !== undefined ? JSON.stringify(options.body) : undefined,
  })

  if (res.status === 204) return undefined as T

  const text = await res.text()
  const data = text ? JSON.parse(text) : null

  if (!res.ok) {
    if (res.status === 401 && onUnauthorized) onUnauthorized()
    const err = (data as APIError)?.error ?? {
      code: 'unknown',
      message: res.statusText || 'Request failed',
    }
    throw new ApiClientError(res.status, err)
  }

  return data as T
}

export const api = {
  get: <T>(path: string) => apiRequest<T>(path),
  post: <T>(path: string, body?: unknown) => apiRequest<T>(path, { method: 'POST', body }),
  patch: <T>(path: string, body?: unknown) => apiRequest<T>(path, { method: 'PATCH', body }),
  delete: <T>(path: string) => apiRequest<T>(path, { method: 'DELETE' }),
}
