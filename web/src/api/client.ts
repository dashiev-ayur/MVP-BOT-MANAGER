import { clearSession, getSession } from '../auth/session'
import { endpoints } from './endpoints'
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
  /**
   * Переопределить base URL на один запрос (проверка при логине до saveSession).
   * Пустая строка = same-origin / proxy.
   */
  baseUrl?: string
  /**
   * Переопределить Bearer-токен на один запрос (пробный вход).
   * При явном token 401 не сбрасывает сохранённую сессию.
   */
  token?: string
}

/** Колбэк на 401 из сохранённой сессии (редирект на /login). Токен не передаём. */
type UnauthorizedHandler = () => void

let unauthorizedHandler: UnauthorizedHandler | null = null

/** Зарегистрировать обработчик 401 (вызывать из auth-слоя / App). */
export function setUnauthorizedHandler(handler: UnauthorizedHandler | null): void {
  unauthorizedHandler = handler
}

/**
 * Тонкая обёртка над fetch: base URL, JSON, Bearer из session.
 * Токен в логи не пишем.
 */
export async function apiRequest<T>(
  path: string,
  options: RequestOptions = {},
): Promise<T> {
  const {
    method = 'GET',
    body,
    auth = true,
    signal,
    baseUrl: baseUrlOverride,
    token: tokenOverride,
  } = options
  const headers = new Headers()

  if (body !== undefined) {
    headers.set('Content-Type', 'application/json')
  }

  // Пробный логин передаёт token явно; иначе берём из session.
  if (auth) {
    const token = tokenOverride ?? getSession()?.token
    if (token) {
      headers.set('Authorization', `Bearer ${token}`)
    }
  }

  const base =
    baseUrlOverride !== undefined
      ? stripTrailingSlash(baseUrlOverride)
      : getBaseUrl()
  const url = `${base}${path.startsWith('/') ? path : `/${path}`}`

  let response: Response
  try {
    // cache: 'no-store' — иначе браузер может отдать закэшированный GET /healthz
    // и индикатор API останется online после остановки control-api.
    response = await fetch(url, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
      signal,
      cache: 'no-store',
    })
  } catch (err) {
    // Сеть / CORS / DNS — без деталей URL с возможными секретами в query.
    const reason = err instanceof Error ? err.message : 'network error'
    throw new ApiError(0, `Сеть недоступна: ${reason}`)
  }

  if (!response.ok) {
    const message = await readErrorMessage(response)
    // 401 на запросе с сохранённой сессией (не пробный логин с token override).
    if (response.status === 401 && auth && tokenOverride === undefined) {
      clearSession()
      unauthorizedHandler?.()
    }
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

/**
 * GET /healthz: ожидает text/plain «ok» (как control-api).
 * Отсекает закэшированный HTML или чужой 200.
 */
export async function checkHealthz(options: Pick<RequestOptions, 'baseUrl' | 'signal'> = {}): Promise<void> {
  const body = await apiRequest<string>(endpoints.healthz, {
    auth: false,
    baseUrl: options.baseUrl,
    signal: options.signal,
  })
  if (typeof body !== 'string' || body.trim() !== 'ok') {
    throw new ApiError(0, 'unexpected healthz body')
  }
}
