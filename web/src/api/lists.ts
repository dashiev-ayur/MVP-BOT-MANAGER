import { apiRequest } from './client'
import { endpoints } from './endpoints'
import type { Bot, BotEvent, Node, Runtime } from './types'

/** GET /v1/nodes — полный список нод (массив DTO). */
export function listNodes(signal?: AbortSignal): Promise<Node[]> {
  return apiRequest<Node[]>(endpoints.nodes, { signal })
}

/** GET /v1/bots — полный список ботов (массив DTO). */
export function listBots(signal?: AbortSignal): Promise<Bot[]> {
  return apiRequest<Bot[]>(endpoints.bots, { signal })
}

/** GET /v1/runtimes — полный список runtimes (массив DTO). */
export function listRuntimes(signal?: AbortSignal): Promise<Runtime[]> {
  return apiRequest<Runtime[]>(endpoints.runtimes, { signal })
}

/** GET /v1/bots/{id}/events — лента событий бота. */
export function listBotEvents(botId: string, signal?: AbortSignal): Promise<BotEvent[]> {
  return apiRequest<BotEvent[]>(endpoints.botEvents(botId), { signal })
}
