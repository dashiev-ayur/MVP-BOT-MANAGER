import type { ReactNode } from 'react'

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
  /** Доп. действия справа (создать и т.п.). */
  actions?: ReactNode
}

/** Заголовок страницы + «Обновить» (+ опциональные actions). */
export function PageToolbar({ title, onRefresh, refreshing, actions }: PageToolbarProps) {
  return (
    <div className="page-toolbar">
      <h1 className="page-toolbar__title">{title}</h1>
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
