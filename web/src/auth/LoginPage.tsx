import { useState, type FormEvent } from 'react'
import { useLocation, useNavigate } from 'react-router'
import { ApiError, apiRequest, checkHealthz } from '../api/client'
import { endpoints } from '../api/endpoints'
import { saveSession } from './session'

type LoginLocationState = {
  reason?: 'unauthorized' | 'auth_required'
  from?: string
}

/**
 * Экран входа: Base URL + Bearer token.
 * Проверка: GET /healthz (доступность) + GET /v1/nodes (валидность токена).
 */
export function LoginPage() {
  const navigate = useNavigate()
  const location = useLocation()
  const state = (location.state ?? null) as LoginLocationState | null

  const [baseUrl, setBaseUrl] = useState('')
  const [token, setToken] = useState('')
  const [error, setError] = useState<string | null>(() => initialMessage(state?.reason))
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setError(null)

    const trimmedToken = token.trim()
    if (!trimmedToken) {
      setError('Укажите Bearer-токен.')
      return
    }

    // Пустой base URL = same-origin / Vite proxy (удобно в dev).
    const trimmedBase = baseUrl.trim()
    setSubmitting(true)

    try {
      // 1) Доступность API — без auth; тело должно быть «ok».
      try {
        await checkHealthz({ baseUrl: trimmedBase })
      } catch (err) {
        setError(formatUnavailableError(err))
        return
      }

      // 2) Валидность токена — пробный защищённый запрос (токен не сохраняем до успеха).
      try {
        await apiRequest(endpoints.nodes, {
          baseUrl: trimmedBase,
          token: trimmedToken,
        })
      } catch (err) {
        setError(formatAuthError(err))
        return
      }

      saveSession({ baseUrl: trimmedBase, token: trimmedToken })
      // Токен в state/URL не кладём — только переход в оболочку.
      navigate('/', { replace: true })
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <main className="login-page">
      <div className="login-page__card">
        <h1>Вход</h1>
        <p className="login-page__hint">
          Подключение к control-api. Токен = значение <code>CONTROL_API_TOKEN</code>.
        </p>

        <form className="login-form" onSubmit={handleSubmit}>
          <label className="login-form__field">
            <span>Base URL API</span>
            <input
              type="text"
              name="baseUrl"
              autoComplete="url"
              placeholder="пусто = этот хост / proxy"
              value={baseUrl}
              onChange={(e) => setBaseUrl(e.target.value)}
              disabled={submitting}
            />
          </label>

          <label className="login-form__field">
            <span>Bearer token</span>
            <input
              type="password"
              name="token"
              autoComplete="current-password"
              placeholder="CONTROL_API_TOKEN"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              disabled={submitting}
              required
            />
          </label>

          {error ? (
            <p className="login-form__error" role="alert">
              {error}
            </p>
          ) : null}

          <button type="submit" className="login-form__submit" disabled={submitting}>
            {submitting ? 'Проверка…' : 'Подключиться'}
          </button>
        </form>
      </div>
    </main>
  )
}

function initialMessage(reason: LoginLocationState['reason']): string | null {
  if (reason === 'unauthorized') {
    return 'Сессия истекла или токен отклонён (401). Войдите снова.'
  }
  if (reason === 'auth_required') {
    return 'Для доступа нужна авторизация.'
  }
  return null
}

/** Сообщение, когда /healthz недоступен. */
function formatUnavailableError(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.status === 0) {
      return 'API недоступен: нет сети или неверный Base URL.'
    }
    return `API недоступен (HTTP ${err.status}). Проверьте Base URL и что control-api запущен.`
  }
  return 'API недоступен. Проверьте Base URL и что control-api запущен.'
}

/** Сообщение при ошибке пробного /v1/nodes (токен). */
function formatAuthError(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.status === 401) {
      return 'Неверный токен: control-api отклонил Authorization (401).'
    }
    if (err.status === 0) {
      return 'Не удалось проверить токен: сеть недоступна.'
    }
    // healthz прошёл, а nodes упал иначе — показываем статус без тела секрета.
    return `Не удалось проверить токен (HTTP ${err.status}).`
  }
  return 'Не удалось проверить токен.'
}
