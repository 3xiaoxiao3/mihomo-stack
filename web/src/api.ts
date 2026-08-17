import type { Backup, Diagnostics, Status, UpdateRecord } from './types'

export class ApiError extends Error {
  readonly status: number
  readonly code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  if (init.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  let response: Response
  try {
    response = await fetch(path, {
      ...init,
      headers,
      credentials: 'same-origin',
    })
  } catch {
    throw new ApiError(0, 'network_error', '无法连接 Guardian，请检查服务状态')
  }
  const body = await response.json().catch(() => ({})) as Record<string, unknown>
  if (!response.ok) {
    const error = (body.error ?? {}) as Record<string, unknown>
    throw new ApiError(
      response.status,
      String(error.code ?? 'request_failed'),
      String(error.message ?? `请求失败（HTTP ${response.status}）`),
    )
  }
  return body as T
}

export const api = {
  login: (token: string) => request<{ authenticated: boolean }>('/api/v1/auth/login', {
    method: 'POST',
    body: JSON.stringify({ token }),
  }),
  logout: () => request<{ authenticated: boolean }>('/api/v1/auth/logout', { method: 'POST' }),
  status: () => request<Status>('/api/v1/status'),
  history: async () => (await request<{ items: UpdateRecord[] }>('/api/v1/history')).items,
  backups: async () => (await request<{ items: Backup[] }>('/api/v1/backups')).items,
  diagnostics: () => request<Diagnostics>('/api/v1/diagnostics'),
  update: () => request<UpdateRecord>('/api/v1/updates', { method: 'POST' }),
  restore: (id: string) => request<UpdateRecord>(`/api/v1/backups/${encodeURIComponent(id)}/restore`, {
    method: 'POST',
  }),
}
