import type { ActualState, Bot, BotChannel, BotType, DesiredState } from '../api/types'

/** Ключи query-фильтров списка ботов (docs/frontend.md §7.1 / §7.4). */
export const BOT_FILTER_KEYS = [
  'bot_type',
  'desired_state',
  'actual_state',
  'assigned_node_id',
  'client_id',
  'channel',
] as const

export type BotFilterKey = (typeof BOT_FILTER_KEYS)[number]

export type BotFilters = {
  bot_type: string
  desired_state: string
  actual_state: string
  assigned_node_id: string
  client_id: string
  channel: string
}

/** Прочитать фильтры из URLSearchParams. */
export function parseBotFilters(params: URLSearchParams): BotFilters {
  return {
    bot_type: params.get('bot_type') ?? '',
    desired_state: params.get('desired_state') ?? '',
    actual_state: params.get('actual_state') ?? '',
    assigned_node_id: params.get('assigned_node_id') ?? '',
    client_id: params.get('client_id') ?? '',
    channel: params.get('channel') ?? '',
  }
}

/**
 * Собрать URLSearchParams только с непустыми фильтрами
 * (чтобы ссылка оставалась короткой).
 */
export function botFiltersToSearchParams(filters: BotFilters): URLSearchParams {
  const next = new URLSearchParams()
  for (const key of BOT_FILTER_KEYS) {
    const value = filters[key].trim()
    if (value) next.set(key, value)
  }
  return next
}

/** Есть ли хотя бы один активный фильтр. */
export function hasActiveBotFilters(filters: BotFilters): boolean {
  return BOT_FILTER_KEYS.some((key) => filters[key].trim() !== '')
}

/** Client-side фильтрация полного GET /v1/bots. */
export function filterBots(bots: Bot[], filters: BotFilters): Bot[] {
  return bots.filter((bot) => {
    if (filters.bot_type && bot.bot_type !== filters.bot_type) return false
    if (filters.desired_state && bot.desired_state !== filters.desired_state) return false
    if (filters.actual_state && bot.actual_state !== filters.actual_state) return false
    if (filters.assigned_node_id && bot.assigned_node_id !== filters.assigned_node_id) {
      return false
    }
    if (filters.client_id && (bot.client_id ?? '') !== filters.client_id) return false
    if (filters.channel && bot.channel !== filters.channel) return false
    return true
  })
}

export const BOT_TYPE_OPTIONS: BotType[] = ['custom', 'default', 'default_extended']
export const DESIRED_STATE_OPTIONS: DesiredState[] = ['running', 'stopped']
export const ACTUAL_STATE_OPTIONS: ActualState[] = [
  'unknown',
  'starting',
  'running',
  'stopping',
  'stopped',
  'failed',
  'migrating',
]
export const CHANNEL_OPTIONS: BotChannel[] = ['telegram', 'max']
