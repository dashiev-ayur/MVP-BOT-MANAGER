/**
 * Пилюля статуса сущности (нода / desired / actual).
 * Отдельно от StatusPill API в шапке — тот же визуальный язык, другие тона.
 */

export type StatePillTone =
  | 'ok'
  | 'danger'
  | 'warn'
  | 'muted'
  | 'info'
  | 'neutral'

type StatePillProps = {
  /** Текст на пилюле (обычно сырое enum-значение). */
  label: string
  tone?: StatePillTone
}

/** Маппинг известных статусов ноды / actual / desired → тон. */
export function toneForState(value: string): StatePillTone {
  switch (value) {
    case 'online':
    case 'running':
      return 'ok'
    case 'offline':
    case 'failed':
      return 'danger'
    case 'draining':
    case 'starting':
    case 'stopping':
    case 'migrating':
      return 'warn'
    case 'unknown':
    case 'stopped':
      return 'muted'
    default:
      return 'neutral'
  }
}

export function StatePill({ label, tone }: StatePillProps) {
  const resolved = tone ?? toneForState(label)
  return (
    <span className={`state-pill state-pill--${resolved}`}>
      <span className="state-pill__dot" aria-hidden />
      {label}
    </span>
  )
}
