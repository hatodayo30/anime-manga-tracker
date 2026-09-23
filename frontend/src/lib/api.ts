const API_BASE = '/api'

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const res = await fetch(API_BASE + path, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || `request failed: ${res.status}`)
  }
  if (res.status === 204) return null as T
  return res.json()
}

export interface Me {
  id: string
  email: string
}

export const api = {
  getMe: () => request<Me>('/auth/me'),
}
