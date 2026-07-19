import { useEffect, useState, type ReactNode } from 'react'
import { formatUpdatedAgo } from '../lib/formatUpdatedAgo'

type LoadingBlockProps = {
  label?: string
}

/** Индикатор загрузки в области контента (не блокирует chrome). */
export function LoadingBlock({ label = 'Загрузка…' }: LoadingBlockProps) {
  return (
    <p className="page-state page-state--loading" role="status">
      {label}
    </p>
  )
}

type ErrorBlockProps = {
  message: string
  onRetry?: () => void
}

/** Баннер ошибки + Retry. */
export function ErrorBlock({ message, onRetry }: ErrorBlockProps) {
  return (
    <div className="page-state page-state--error" role="alert">
      <p className="page-state__message">{message}</p>
      {onRetry ? (
        <button type="button" className="btn btn--secondary" onClick={onRetry}>
          Повторить
        </button>
      ) : null}
    </div>
  )
}

type EmptyBlockProps = {
  message: string
  /** Опциональный CTA (например «Создать»). */
  action?: ReactNode
}

/** Пустой список. */
export function EmptyBlock({ message, action }: EmptyBlockProps) {
  return (
    <div className="page-state page-state--empty">
      <p className="page-state__message">{message}</p>
      {action}
    </div>
  )
}

type PageToolbarProps = {
  title: string
  onRefresh: () => void
  refreshing?: boolean
  /**
   * Epoch последнего успешного fetch — индикатор «Обновлено N с назад» (§7.11).
   * Без значения (ещё не загрузили) — ничего не показываем.
   */
  updatedAt?: number | null
  /** Доп. действия справа (создать и т.п.). */
  actions?: ReactNode
}

/** Заголовок страницы + stale + «Обновить» (+ опциональные actions). */
export function PageToolbar({
  title,
  onRefresh,
  refreshing,
  updatedAt,
  actions,
}: PageToolbarProps) {
  return (
    <div className="page-toolbar">
      <div className="page-toolbar__heading">
        <h1 className="page-toolbar__title">{title}</h1>
        {updatedAt != null ? <StaleAge updatedAt={updatedAt} /> : null}
      </div>
      <div className="page-toolbar__actions">
        <button
          type="button"
          className="btn btn--secondary"
          onClick={onRefresh}
          disabled={refreshing}
        >
          {refreshing ? 'Обновление…' : 'Обновить'}
        </button>
        {actions}
      </div>
    </div>
  )
}

type StaleAgeProps = {
  updatedAt: number
}

/** Тикающий stale-индикатор без мигания таблицы (UI-6.3 / §7.11). */
function StaleAge({ updatedAt }: StaleAgeProps) {
  const [nowMs, setNowMs] = useState(() => Date.now())

  useEffect(() => {
    const id = window.setInterval(() => setNowMs(Date.now()), 1000)
    return () => window.clearInterval(id)
  }, [])

  return (
    <span className="stale-age" role="status">
      {formatUpdatedAgo(updatedAt, nowMs)}
    </span>
  )
}
