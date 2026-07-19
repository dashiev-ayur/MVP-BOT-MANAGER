/**
 * Пути control-api (docs/frontend.md §5.3).
 * Тонкий слой констант — без логики запросов.
 */

export const endpoints = {
  healthz: '/healthz',
  nodes: '/v1/nodes',
  bots: '/v1/bots',
  bot: (id: string) => `/v1/bots/${encodeURIComponent(id)}`,
  botStart: (id: string) => `/v1/bots/${encodeURIComponent(id)}/start`,
  botStop: (id: string) => `/v1/bots/${encodeURIComponent(id)}/stop`,
  botMigrate: (id: string) => `/v1/bots/${encodeURIComponent(id)}/migrate`,
  botEvents: (id: string) => `/v1/bots/${encodeURIComponent(id)}/events`,
  runtimes: '/v1/runtimes',
} as const
