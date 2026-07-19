/**
 * Относительное время на русском для last_seen_at и подобных полей.
 * Абсолютное значение оставляем в title/атрибуте.
 */
export function formatRelativeRu(iso: string, nowMs: number = Date.now()): string {
  const then = Date.parse(iso)
  if (Number.isNaN(then)) {
    return iso
  }

  const diffSec = Math.round((nowMs - then) / 1000)
  if (diffSec < 0) {
    // Часы клиента/сервера разъехались — показываем абсолют кратко.
    return formatAbsoluteShort(iso)
  }
  if (diffSec < 5) {
    return 'только что'
  }
  if (diffSec < 60) {
    return `${diffSec}с назад`
  }

  const diffMin = Math.floor(diffSec / 60)
  if (diffMin < 60) {
    return `${diffMin}м назад`
  }

  const diffH = Math.floor(diffMin / 60)
  if (diffH < 48) {
    return `${diffH}ч назад`
  }

  const diffD = Math.floor(diffH / 24)
  return `${diffD}д назад`
}

/** Короткая абсолютная дата для title / fallback. */
export function formatAbsoluteShort(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) {
    return iso
  }
  return d.toLocaleString('ru-RU', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}
