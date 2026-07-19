import { useEffect, useRef, useState } from 'react'
import { ApiError } from '../api/client'

export type FetchListState<T> = {
  data: T | null
  error: string | null
  /** Идёт запрос (первичная загрузка или refresh). */
  loading: boolean
  /** Перезагрузить данные (кнопка «Обновить»). */
  refresh: () => void
}

/**
 * Загрузка списка/снимка с AbortSignal, loading/error и ручным refresh.
 * fetcher читается из ref — актуальный на каждом tick, без лишних перезапусков.
 */
export function useFetchList<T>(
  fetcher: (signal: AbortSignal) => Promise<T>,
): FetchListState<T> {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [tick, setTick] = useState(0)
  const fetcherRef = useRef(fetcher)
  fetcherRef.current = fetcher

  useEffect(() => {
    const ac = new AbortController()
    let alive = true

    setLoading(true)
    setError(null)

    fetcherRef
      .current(ac.signal)
      .then((result) => {
        if (!alive) return
        setData(result)
      })
      .catch((err: unknown) => {
        if (!alive || ac.signal.aborted) return
        // Предыдущие данные не затираем — удобнее при ошибке refresh.
        const message =
          err instanceof ApiError
            ? err.message
            : err instanceof Error
              ? err.message
              : 'Ошибка загрузки'
        setError(message)
      })
      .finally(() => {
        if (alive) setLoading(false)
      })

    return () => {
      alive = false
      ac.abort()
    }
  }, [tick])

  return {
    data,
    error,
    loading,
    refresh: () => setTick((n) => n + 1),
  }
}
