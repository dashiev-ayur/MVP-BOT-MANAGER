import type { ReactNode } from 'react'

type AppShellProps = {
  children: ReactNode
}

/**
 * Минимальная оболочка layout.
 * Навигация, healthz-индикатор — UI-1.
 */
export function AppShell({ children }: AppShellProps) {
  return (
    <div className="app-shell">
      <header className="app-shell__header">
        <strong>mvp-manager</strong>
      </header>
      <div className="app-shell__body">{children}</div>
    </div>
  )
}
