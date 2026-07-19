/**
 * Текст для индикатора stale (§7.11): «Обновлено N с назад».
 * updatedAtMs — epoch успешного fetch; nowMs — для тикающего UI.
 */
export function formatUpdatedAgo(
  updatedAtMs: number,
  nowMs: number = Date.now(),
): string {
  const diffSec = Math.max(0, Math.floor((nowMs - updatedAtMs) / 1000))
  if (diffSec < 5) {
    return 'Обновлено только что'
  }
  if (diffSec < 60) {
    return `Обновлено ${diffSec} с назад`
  }
  const diffMin = Math.floor(diffSec / 60)
  if (diffMin < 60) {
    return `Обновлено ${diffMin} м назад`
  }
  const diffH = Math.floor(diffMin / 60)
  return `Обновлено ${diffH} ч назад`
}
