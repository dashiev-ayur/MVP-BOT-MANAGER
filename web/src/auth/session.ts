/**
 * Сессия оператора в localStorage: base URL + Bearer token.
 * Токен не логировать и не включать в сообщения об ошибках.
 */

const STORAGE_KEY = 'mvp-manager.session'

export type Session = {
  /** Base URL control-api (может быть пустым — тогда same-origin / proxy). */
  baseUrl: string
  /** Bearer-токен (= CONTROL_API_TOKEN на сервере). Не логировать. */
  token: string
}

/** Есть ли сохранённая сессия с непустым токеном. */
export function hasSession(): boolean {
  return getSession() !== null
}

/** Прочитать сессию из localStorage; при битых данных — null. */
export function getSession(): Session | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) {
      return null
    }
    const parsed = JSON.parse(raw) as Partial<Session>
    // Пустой токен не считаем валидной сессией.
    if (typeof parsed.token !== 'string' || parsed.token.length === 0) {
      return null
    }
    return {
      baseUrl: typeof parsed.baseUrl === 'string' ? parsed.baseUrl : '',
      token: parsed.token,
    }
  } catch {
    return null
  }
}

/** Сохранить сессию после успешного входа. */
export function saveSession(session: Session): void {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(session))
}

/** Сбросить сессию (Выйти или 401). */
export function clearSession(): void {
  localStorage.removeItem(STORAGE_KEY)
}

/**
 * Маска токена для шапки (без полного секрета).
 * Показываем только намёк, что сессия есть.
 */
export function maskToken(token: string): string {
  if (token.length <= 4) {
    return '••••'
  }
  return `••••${token.slice(-4)}`
}
