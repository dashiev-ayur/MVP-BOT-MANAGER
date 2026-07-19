/**
 * Заглушка сессии (localStorage).
 * Полноценный login UX — UI-1; здесь только чтение/запись для api/client.
 */

const STORAGE_KEY = 'mvp-manager.session'

export type Session = {
  /** Base URL control-api (может быть пустым — тогда same-origin / proxy). */
  baseUrl: string
  /** Bearer-токен (= CONTROL_API_TOKEN на сервере). Не логировать. */
  token: string
}

/** Прочитать сессию из localStorage; при битых данных — null. */
export function getSession(): Session | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) {
      return null
    }
    const parsed = JSON.parse(raw) as Partial<Session>
    if (typeof parsed.token !== 'string') {
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

/** Сохранить сессию (UI-1 будет вызывать после успешного входа). */
export function saveSession(session: Session): void {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(session))
}

/** Сбросить сессию (например после 401). */
export function clearSession(): void {
  localStorage.removeItem(STORAGE_KEY)
}
