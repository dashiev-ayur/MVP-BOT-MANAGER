import type { ReactNode } from 'react'

export type ConfirmDialogProps = {
  open: boolean
  title: string
  /** Текст подтверждения или произвольный React-контент. */
  message: ReactNode
  confirmLabel?: string
  cancelLabel?: string
  /** Пока идёт запрос — disabled на кнопках. */
  busy?: boolean
  /** Опасное действие (Stop) — красная кнопка confirm. */
  danger?: boolean
  onConfirm: () => void
  onCancel: () => void
}

/**
 * Простой confirm-dialog (UI-4 Stop и далее Migrate).
 * Без сторонних UI-библиотек: overlay + role="dialog".
 */
export function ConfirmDialog({
  open,
  title,
  message,
  confirmLabel = 'Подтвердить',
  cancelLabel = 'Отмена',
  busy = false,
  danger = false,
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  if (!open) {
    return null
  }

  return (
    <div
      className="confirm-dialog"
      role="presentation"
      onClick={(e) => {
        // Клик по backdrop — отмена (не во время запроса).
        if (!busy && e.target === e.currentTarget) {
          onCancel()
        }
      }}
    >
      <div
        className="confirm-dialog__panel"
        role="dialog"
        aria-modal="true"
        aria-labelledby="confirm-dialog-title"
      >
        <h2 id="confirm-dialog-title" className="confirm-dialog__title">
          {title}
        </h2>
        <div className="confirm-dialog__message">{message}</div>
        <div className="confirm-dialog__actions">
          <button
            type="button"
            className="btn btn--secondary"
            onClick={onCancel}
            disabled={busy}
          >
            {cancelLabel}
          </button>
          <button
            type="button"
            className={danger ? 'btn btn--danger' : 'btn btn--primary'}
            onClick={onConfirm}
            disabled={busy}
          >
            {busy ? 'Выполнение…' : confirmLabel}
          </button>
        </div>
      </div>
    </div>
  )
}
