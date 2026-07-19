import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from 'react'

export type ToastKind = 'success' | 'error'

type ToastItem = {
  id: number
  kind: ToastKind
  message: string
}

type ToastApi = {
  /** Успех команды (start/stop/migrate/create/patch). */
  success: (message: string) => void
  /** Ошибка команды / API. */
  error: (message: string) => void
}

const ToastContext = createContext<ToastApi | null>(null)

/** Сколько держать toast на экране (мс). */
const TOAST_TTL_MS = 4500

/**
 * Простой стек toasts без внешней библиотеки (UI-6.3).
 * Провайдер выше Router outlet — сообщения переживают navigate после create/patch.
 */
export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([])

  const push = useCallback((kind: ToastKind, message: string) => {
    const id = Date.now() + Math.floor(Math.random() * 1000)
    setItems((prev) => [...prev, { id, kind, message }])
    window.setTimeout(() => {
      setItems((prev) => prev.filter((t) => t.id !== id))
    }, TOAST_TTL_MS)
  }, [])

  const api = useMemo<ToastApi>(
    () => ({
      success: (message) => push('success', message),
      error: (message) => push('error', message),
    }),
    [push],
  )

  function dismiss(id: number) {
    setItems((prev) => prev.filter((t) => t.id !== id))
  }

  return (
    <ToastContext.Provider value={api}>
      {children}
      <div className="toast-stack" aria-live="polite" aria-relevant="additions">
        {items.map((t) => (
          <div
            key={t.id}
            className={
              t.kind === 'success' ? 'toast toast--success' : 'toast toast--error'
            }
            role={t.kind === 'error' ? 'alert' : 'status'}
          >
            <span className="toast__message">{t.message}</span>
            <button
              type="button"
              className="toast__dismiss"
              onClick={() => dismiss(t.id)}
              aria-label="Закрыть"
            >
              ×
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  )
}

/** Доступ к toast API; должен вызываться внутри ToastProvider. */
export function useToast(): ToastApi {
  const ctx = useContext(ToastContext)
  if (!ctx) {
    throw new Error('useToast: нет ToastProvider')
  }
  return ctx
}
