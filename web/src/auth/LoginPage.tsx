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

    setSubmitting(true)

    try {
      // 1) Найти рабочий base: proxy → прямой :8080 (API должен быть запущен).
      let trimmedBase: string
      try {
        trimmedBase = await resolveReachableApiBase(baseUrl.trim())
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
          В dev поле Base URL лучше оставить пустым (запросы идут через Vite proxy на{' '}
          <code>:8080</code>).
        </p>

        <form className="login-form" onSubmit={handleSubmit}>
          <label className="login-form__field">
            <span>Base URL API</span>
            <input
              type="text"
              name="baseUrl"
              autoComplete="url"
              placeholder="оставьте пустым"
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

/**
 * Нормализация Base URL для логина.
 * - пусто → Vite proxy
 * - `127.0.0.1:8080` без схемы → иначе fetch считает путь относительным и получает HTML
 * - локальный :8080 → пустой base (proxy)
 */
function normalizeLoginBaseUrl(raw: string): string {
  if (!raw) {
    return ''
  }
  let candidate = raw
  // Нет схемы (http/https) — для URL() и fetch это не абсолютный адрес.
  if (!/^[a-zA-Z][a-zA-Z0-9+.-]*:/.test(candidate)) {
    candidate = `http://${candidate}`
  }
  try {
    const u = new URL(candidate)
    const localHost = u.hostname === '127.0.0.1' || u.hostname === 'localhost'
    if (localHost && u.port === '8080') {
      return ''
    }
    return u.origin
  } catch {
    return raw
  }
}

/**
 * Перебираем proxy и прямой URL: /healthz должен ответить «ok».
 * Типичная ошибка — control-api не запущен (в терминале после ^C его нет).
 */
async function resolveReachableApiBase(rawInput: string): Promise<string> {
  const preferred = normalizeLoginBaseUrl(rawInput)
  const candidates = uniqueBases([
    preferred,
    '',
    'http://127.0.0.1:8080',
    'http://localhost:8080',
  ])

  let lastErr: unknown
  for (const base of candidates) {
    try {
      await checkHealthz({ baseUrl: base })
      return base
    } catch (err) {
      lastErr = err
    }
  }
  throw lastErr instanceof Error ? lastErr : new ApiError(0, 'API unreachable')
}

function uniqueBases(bases: string[]): string[] {
  const out: string[] = []
  for (const b of bases) {
    if (!out.includes(b)) {
      out.push(b)
    }
  }
  return out
}

/** Сообщение, когда /healthz недоступен. */
function formatUnavailableError(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.message.includes('unexpected healthz')) {
      return (
        'Ответ /healthz не от control-api. Откройте UI через npm run dev, Base URL очистите; ' +
        'не вводите адрес без http://.'
      )
    }
    if (err.status === 502 || err.status === 503 || err.status === 504) {
      return (
        'Vite proxy не достучался до control-api (HTTP ' +
        err.status +
        '). В отдельном терминале из корня репозитория запустите и не останавливайте:\n' +
        'export NODE_ID=node-1 STORE=memory CONTROL_API_TOKEN=dev-token\n' +
        'go run ./cmd/control-api'
      )
    }
    if (err.status === 0) {
      return (
        'control-api не отвечает на 127.0.0.1:8080. Запустите его в отдельном терминале ' +
        '(и оставьте работать, не нажимайте Ctrl+C):\n' +
        'export NODE_ID=node-1 STORE=memory CONTROL_API_TOKEN=dev-token\n' +
        'go run ./cmd/control-api\n' +
        'UI: cd web && npm run dev — Base URL пустой, токен dev-token.'
      )
    }
    return `API недоступен (HTTP ${err.status}). Проверьте, что control-api запущен.`
  }
  return (
    'control-api недоступен. Запустите: go run ./cmd/control-api ' +
    '(NODE_ID, CONTROL_API_TOKEN) и npm run dev в web/.'
  )
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
