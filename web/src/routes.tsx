import { Navigate, Route, Routes } from 'react-router'
import { LoginPage } from './auth/LoginPage'
import { RedirectIfAuthenticated, RequireAuth } from './auth/RequireAuth'
import { UnauthorizedListener } from './auth/UnauthorizedListener'
import { AppShell } from './layout/AppShell'
import { BotCreatePage } from './pages/BotCreatePage'
import { BotDetailPage } from './pages/BotDetailPage'
import { BotsListPage } from './pages/BotsListPage'
import { NodesListPage } from './pages/NodesListPage'
import { OverviewPage } from './pages/OverviewPage'
import { RuntimesPage } from './pages/RuntimesPage'

/**
 * Маршруты UI: /login + защищённая оболочка.
 * bots/new — выше bots/:id, иначе «new» попадёт в :id.
 */
export function AppRoutes() {
  return (
    <>
      <UnauthorizedListener />
      <Routes>
        <Route element={<RedirectIfAuthenticated />}>
          <Route path="/login" element={<LoginPage />} />
        </Route>

        <Route element={<RequireAuth />}>
          <Route element={<AppShell />}>
            <Route index element={<OverviewPage />} />
            <Route path="bots" element={<BotsListPage />} />
            <Route path="bots/new" element={<BotCreatePage />} />
            <Route path="bots/:id" element={<BotDetailPage />} />
            <Route path="nodes" element={<NodesListPage />} />
            <Route path="runtimes" element={<RuntimesPage />} />
          </Route>
        </Route>

        {/* Неизвестный путь: с сессией → обзор; без — RequireAuth уведёт на login */}
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </>
  )
}
