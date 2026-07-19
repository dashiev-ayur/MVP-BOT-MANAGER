import { useEffect, useRef, useState } from 'react'
import { ApiError } from '../api/client'

/** Рекомендуемый интервал авто-poll списков/карточки (3–10s, UI-6.3). */
export const DEFAULT_POLL_INTERVAL_MS = 5000

export type FetchListOptions = {
  /**
   * Интервал авто-poll (мс). В диапазоне 3–10s по ТЗ (§7.11 / UI-6.3).
   * Фоновый tick не ставит loading=true — таблица не мигает unmount.
   */
  pollIntervalMs?: number
}

export type FetchListState<T> = {
  data: T | null
  error: string | null
  /** Идёт запрос с индикацией (первичная загрузка или ручной refresh). */
  loading: boolean
  /** Перезагрузить данные (кнопка «Обновить») — с индикацией refreshing. */
  refresh: () => void
  /** Время последнего успешного ответа (ms epoch); для «Обновлено N с назад». */
  updatedAt: number | null
}

type FetchRequest = {
  tick: number
  /** silent: poll / фоновый refresh — не трогаем loading. */
  silent: boolean
}

/**
 * Загрузка списка/снимка с AbortSignal, loading/error, ручным refresh и опциональным poll.
 * fetcher читается из ref — актуальный на каждом tick, без лишних перезапусков.
 */
export function useFetchList<T>(
  fetcher: (signal: AbortSignal) => Promise<T>,
  options: FetchListOptions = {},
): FetchListState<T> {
  const { pollIntervalMs } = options
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [updatedAt, setUpdatedAt] = useState<number | null>(null)
  const [request, setRequest] = useState<FetchRequest>({ tick: 0, silent: false })
  const fetcherRef = useRef(fetcher)
  fetcherRef.current = fetcher
  // Актуальные data в effect без зависимости — чтобы silent смотрел «уже есть снимок».
  const dataRef = useRef<T | null>(null)
  dataRef.current = data

  useEffect(() => {
    const ac = new AbortController()
    let alive = true
    // Silent poll: не включаем loading, чтобы не unmount'ить таблицу (LoadingBlock).
    const silent = request.silent && dataRef.current !== null

    if (!silent) {
      setLoading(true)
    }
    // Ошибку сбрасываем только на «громком» запросе — иначе poll мигает баннером.
    if (!silent) {
      setError(null)
    }

    fetcherRef
      .current(ac.signal)
      .then((result) => {
        if (!alive) return
        setData(result)
        setUpdatedAt(Date.now())
        // Успешный silent poll снимает прошлую ошибку.
        setError(null)
      })
      .catch((err: unknown) => {
        if (!alive || ac.signal.aborted) return
        // Предыдущие данные не затираем — удобнее при ошибке refresh/poll.
        const message =
          err instanceof ApiError
            ? err.message
            : err instanceof Error
              ? err.message
              : 'Ошибка загрузки'
        setError(message)
      })
      .finally(() => {
        // Всегда снимаем loading у живого запроса: иначе abort громкого
        // silent-poll'ом оставит loading=true навсегда.
        if (alive) setLoading(false)
      })

    return () => {
      alive = false
      ac.abort()
    }
  }, [request])

  // Авто-poll: только silent tick после первой успешной загрузки.
  useEffect(() => {
    if (pollIntervalMs == null || pollIntervalMs <= 0) {
      return
    }
    const id = window.setInterval(() => {
      // Не дёргаем poll, пока нет снимка — иначе гонка с первичной загрузкой.
      if (dataRef.current === null) return
      setRequest((prev) => ({ tick: prev.tick + 1, silent: true }))
    }, pollIntervalMs)
    return () => window.clearInterval(id)
  }, [pollIntervalMs])

  return {
    data,
    error,
    loading,
    updatedAt,
    refresh: () => setRequest((prev) => ({ tick: prev.tick + 1, silent: false })),
  }
}
