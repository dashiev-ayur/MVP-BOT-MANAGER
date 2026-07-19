import { useEffect, useState } from 'react'
import { checkHealthz } from '../api/client'
import type { ApiStatus } from './StatusPill'

/** Интервал опроса /healthz (ТЗ: 10–15s). */
const POLL_MS = 12_000

/**
 * Периодический poll GET /healthz (без auth).
 * online / offline для индикатора в AppShell.
 */
export function useApiHealth(): ApiStatus {
  const [status, setStatus] = useState<ApiStatus>('checking')

  useEffect(() => {
    let cancelled = false

    async function check() {
      try {
        await checkHealthz()
        if (!cancelled) {
          setStatus('online')
        }
      } catch {
        if (!cancelled) {
          setStatus('offline')
        }
      }
    }

    void check()
    const id = window.setInterval(() => {
      void check()
    }, POLL_MS)

    // После возврата на вкладку сразу перепроверить (не ждать интервал).
    function onVisible() {
      if (document.visibilityState === 'visible') {
        void check()
      }
    }
    document.addEventListener('visibilitychange', onVisible)

    return () => {
      cancelled = true
      window.clearInterval(id)
      document.removeEventListener('visibilitychange', onVisible)
    }
  }, [])

  return status
}
