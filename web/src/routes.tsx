import { Navigate, Route, Routes } from 'react-router'
import { LoginPage } from './auth/LoginPage'
import { AppShell } from './layout/AppShell'
import { OverviewPage } from './pages/OverviewPage'

/**
 * Заглушка роутера: /login и /.
 * Защита маршрутов и остальные экраны — UI-1+.
 */
export function AppRoutes() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        path="/"
        element={
          <AppShell>
            <OverviewPage />
          </AppShell>
        }
      />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
