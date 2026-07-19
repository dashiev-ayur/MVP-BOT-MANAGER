import { getSession } from '../auth/session'
import type { ApiErrorBody } from './types'

/** Ошибка HTTP-ответа control-api (без логирования секретов). */
export class ApiError extends Error {
  readonly status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

/**
 * Базовый URL для fetch.
 * Приоритет: session.baseUrl → VITE_CONTROL_API_URL → '' (same-origin / Vite proxy).
 */
export function getBaseUrl(): string {
  const session = getSession()
  if (session?.baseUrl) {
    return stripTrailingSlash(session.baseUrl)
  }
  const fromEnv = import.meta.env.VITE_CONTROL_API_URL
  if (fromEnv) {
    return stripTrailingSlash(fromEnv)
  }
  // Пустая строка: в dev запросы идут на origin Vite и проксируются на control-api.
  return ''
}

function stripTrailingSlash(url: string): string {
  return url.replace(/\/+$/, '')
}

export type RequestOptions = {
  method?: string
  /** Тело JSON; сериализуется в snake_case-объект как есть. */
  body?: unknown
  /** Если false — не добавлять Authorization (для /healthz). */
  auth?: boolean
  signal?: AbortSignal
}

/**
 * Тонкая обёртка над fetch: base URL, JSON, Bearer из session.
 * Токен в логи не пишем.
 */
export async function apiRequest<T>(
  path: string,
  options: RequestOptions = {},
): Promise<T> {
  const { method = 'GET', body, auth = true, signal } = options
  const headers = new Headers()

  if (body !== undefined) {
    headers.set('Content-Type', 'application/json')
  }

  if (auth) {
    const token = getSession()?.token
    if (token) {
      headers.set('Authorization', `Bearer ${token}`)
    }
  }

  const url = `${getBaseUrl()}${path.startsWith('/') ? path : `/${path}`}`

  const response = await fetch(url, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    signal,
  })

  if (!response.ok) {
    const message = await readErrorMessage(response)
    throw new ApiError(response.status, message)
  }

  // /healthz отдаёт text/plain; для пустых 204 — undefined как T.
  const contentType = response.headers.get('content-type') ?? ''
  if (contentType.includes('application/json')) {
    return (await response.json()) as T
  }

  const text = await response.text()
  return text as T
}

async function readErrorMessage(response: Response): Promise<string> {
  try {
    const data = (await response.json()) as ApiErrorBody
    if (data?.error) {
      return data.error
    }
  } catch {
    // не JSON — ниже fallback
  }
  return `HTTP ${response.status}`
}
