import { useEffect, useMemo, useState } from 'react'
import type { Node } from '../api/types'

export type MigrateDialogProps = {
  open: boolean
  botName: string
  /** Ноды из GET /v1/nodes (уже загружены на карточке). */
  nodes: Node[]
  /** Текущая assigned_node_id бота — исключается из выбора / disabled. */
  currentNodeId: string | null
  busy?: boolean
  /** Вызыва только после Confirm с выбранным to_node_id (не при открытии). */
  onConfirm: (toNodeId: string) => void
  onCancel: () => void
}

/**
 * Online выше offline/draining; при равном статусе — по hostname.
 * Нужно для select целевой ноды (docs/frontend.md §7.8).
 */
function sortNodesForMigrate(nodes: Node[]): Node[] {
  return [...nodes].sort((a, b) => {
    const rank = (status: Node['status']) => (status === 'online' ? 0 : 1)
    const byStatus = rank(a.status) - rank(b.status)
    if (byStatus !== 0) {
      return byStatus
    }
    return a.hostname.localeCompare(b.hostname, 'ru')
  })
}

/**
 * Dialog переноса бота (UI-5): select to_node_id + confirm.
 * Submit disabled без выбора; POST уходит только из onConfirm родителя.
 */
export function MigrateDialog({
  open,
  botName,
  nodes,
  currentNodeId,
  busy = false,
  onConfirm,
  onCancel,
}: MigrateDialogProps) {
  const [toNodeId, setToNodeId] = useState('')

  // При каждом открытии сбрасываем выбор — нельзя «тихо» унаследовать прошлый.
  useEffect(() => {
    if (open) {
      setToNodeId('')
    }
  }, [open])

  const sortedNodes = useMemo(() => sortNodesForMigrate(nodes), [nodes])

  const selected = sortedNodes.find((n) => n.id === toNodeId) ?? null
  const canSubmit = toNodeId !== '' && selected !== null && selected.id !== currentNodeId

  if (!open) {
    return null
  }

  return (
    <div
      className="confirm-dialog"
      role="presentation"
      onClick={(e) => {
        if (!busy && e.target === e.currentTarget) {
          onCancel()
        }
      }}
    >
      <div
        className="confirm-dialog__panel confirm-dialog__panel--migrate"
        role="dialog"
        aria-modal="true"
        aria-labelledby="migrate-dialog-title"
      >
        <h2 id="migrate-dialog-title" className="confirm-dialog__title">
          Перенести бота?
        </h2>

        <div className="confirm-dialog__message">
          <p>
            Бот «{botName}» будет перенесён на выбранную ноду. Во время migrate
            возможен краткий downtime: actual станет <span className="mono">migrating</span>,
            затем reconcile на новой ноде.
          </p>

          <label className="migrate-dialog__field">
            <span>Целевая нода (to_node_id)</span>
            <select
              value={toNodeId}
              disabled={busy}
              onChange={(e) => setToNodeId(e.target.value)}
              aria-required="true"
            >
              <option value="">Выберите ноду…</option>
              {sortedNodes.map((node) => {
                const isCurrent = currentNodeId !== null && node.id === currentNodeId
                return (
                  <option key={node.id} value={node.id} disabled={isCurrent}>
                    {node.hostname} ({node.status})
                    {isCurrent ? ' — текущая' : ''}
                  </option>
                )
              })}
            </select>
          </label>

          {selected && canSubmit ? (
            <p className="migrate-dialog__confirm-hint">
              Перенос на «{selected.hostname}» ({selected.status}). Подтвердите, чтобы
              отправить команду.
            </p>
          ) : null}
        </div>

        <div className="confirm-dialog__actions">
          <button
            type="button"
            className="btn btn--secondary"
            onClick={onCancel}
            disabled={busy}
          >
            Отмена
          </button>
          <button
            type="button"
            className="btn btn--primary"
            disabled={!canSubmit || busy}
            onClick={() => {
              // Двойная защита: без to_node_id запрос не уходит.
              if (!canSubmit || busy) {
                return
              }
              onConfirm(toNodeId)
            }}
          >
            {busy ? 'Выполнение…' : 'Migrate'}
          </button>
        </div>
      </div>
    </div>
  )
}
