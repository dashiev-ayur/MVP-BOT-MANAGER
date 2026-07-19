import { Navigate, Outlet, useLocation } from 'react-router'
import { hasSession } from './session'

/**
 * Защита маршрутов: без сессии — только редирект на /login.
 * Сохраняем from, чтобы после входа можно было вернуться (на будущее).
 */
export function RequireAuth() {
  const location = useLocation()

  if (!hasSession()) {
    return (
      <Navigate
        to="/login"
        replace
        state={{ from: location.pathname, reason: 'auth_required' }}
      />
    )
  }

  return <Outlet />
}

/**
 * Гостевой маршрут: авторизованный пользователь с /login уходит на /.
 */
export function RedirectIfAuthenticated() {
  if (hasSession()) {
    return <Navigate to="/" replace />
  }
  return <Outlet />
}
