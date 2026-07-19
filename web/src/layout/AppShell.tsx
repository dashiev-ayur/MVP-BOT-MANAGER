import { NavLink, Outlet, useNavigate } from 'react-router'
import { clearSession, getSession, maskToken } from '../auth/session'
import { StatusPill } from './StatusPill'
import { useApiHealth } from './useApiHealth'

const NAV_ITEMS = [
  { to: '/', label: 'Обзор', end: true },
  { to: '/bots', label: 'Боты', end: false },
  { to: '/nodes', label: 'Ноды', end: false },
  { to: '/runtimes', label: 'Runtimes', end: false },
] as const

/**
 * Оболочка: шапка (API + маска токена + Выйти), сайдбар-навигация, Outlet.
 */
export function AppShell() {
  const navigate = useNavigate()
  const apiStatus = useApiHealth()
  const session = getSession()
  const tokenHint = session ? maskToken(session.token) : '••••'

  function handleLogout() {
    clearSession()
    navigate('/login', { replace: true })
  }

  return (
    <div className="app-shell">
      <header className="app-shell__header">
        <strong className="app-shell__brand">mvp-manager</strong>
        <div className="app-shell__header-right">
          <StatusPill status={apiStatus} />
          <span className="app-shell__token" title="Токен сессии (маскирован)">
            token {tokenHint}
          </span>
          <button type="button" className="app-shell__logout" onClick={handleLogout}>
            Выйти
          </button>
        </div>
      </header>

      {apiStatus === 'offline' ? (
        <div className="app-shell__banner" role="status">
          API offline: control-api не отвечает на /healthz. Данные могут быть недоступны.
        </div>
      ) : null}

      <div className="app-shell__main">
        <nav className="app-shell__nav" aria-label="Основная навигация">
          {NAV_ITEMS.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                isActive ? 'app-shell__nav-link app-shell__nav-link--active' : 'app-shell__nav-link'
              }
            >
              {item.label}
            </NavLink>
          ))}
        </nav>
        <div className="app-shell__body">
          <Outlet />
        </div>
      </div>
    </div>
  )
}
