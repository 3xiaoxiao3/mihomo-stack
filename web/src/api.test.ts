import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError } from './api'

describe('api client', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('sends the login token only in the request body', async () => {
    const fetchMock = vi.fn(async (_path: string, init: RequestInit) => {
      expect(init.credentials).toBe('same-origin')
      expect(init.body).toBe(JSON.stringify({ token: 'secret-token' }))
      return new Response(JSON.stringify({ authenticated: true }), { status: 200 })
    })
    vi.stubGlobal('fetch', fetchMock)

    await expect(api.login('secret-token')).resolves.toEqual({ authenticated: true })
  })

  it('converts API errors into a stable typed error', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({
      error: { code: 'unauthorized', message: '需要登录' },
    }), { status: 401 })))

    await expect(api.status()).rejects.toMatchObject({
      status: 401,
      code: 'unauthorized',
      message: '需要登录',
    })
  })
})
