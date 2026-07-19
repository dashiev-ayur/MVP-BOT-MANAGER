export type ApiStatus = 'online' | 'offline' | 'checking'

type StatusPillProps = {
  status: ApiStatus
}

/**
 * Индикатор доступности control-api (результат poll /healthz).
 */
export function StatusPill({ status }: StatusPillProps) {
  const label =
    status === 'online' ? 'online' : status === 'offline' ? 'offline' : 'проверка…'

  return (
    <span
      className={`status-pill status-pill--${status}`}
      title="GET /healthz"
      aria-live="polite"
    >
      <span className="status-pill__dot" aria-hidden />
      API {label}
    </span>
  )
}
