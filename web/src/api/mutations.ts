import { apiRequest } from './client'
import { endpoints } from './endpoints'
import type {
  Bot,
  BotChannel,
  BotRunMode,
  BotType,
  DesiredState,
} from './types'

/**
 * Тело POST /v1/bots (snake_case).
 * Для default / default_extended не включаем custom_name и custom-only поля —
 * сервер DisallowUnknownFields; лишние ключи → 400, а для non-custom
 * custom_name по контракту UI не передаём.
 */
export type CreateBotBody = {
  name: string
  bot_type: BotType
  channel: BotChannel
  run_mode: BotRunMode
  port: number
  token_ref: string
  desired_state: DesiredState
  /** Если пусто — API возьмёт NODE_ID из конфига. */
  assigned_node_id?: string
  /** Только для bot_type=custom. */
  custom_name?: string
  artifact_path?: string
  start_command?: string
  workdir?: string
}

/** Ответ start/stop: desired уже выставлен; actual догонит agent. */
export type LifecycleOk = {
  status: string
  desired: string
  bot_id: string
}

/** POST /v1/bots → 201 + Bot DTO (masked). */
export function createBot(body: CreateBotBody, signal?: AbortSignal): Promise<Bot> {
  return apiRequest<Bot>(endpoints.bots, {
    method: 'POST',
    body,
    signal,
  })
}

/** POST /v1/bots/{id}/start — тело пустое. */
export function startBot(id: string, signal?: AbortSignal): Promise<LifecycleOk> {
  return apiRequest<LifecycleOk>(endpoints.botStart(id), {
    method: 'POST',
    signal,
  })
}

/** POST /v1/bots/{id}/stop — тело пустое. */
export function stopBot(id: string, signal?: AbortSignal): Promise<LifecycleOk> {
  return apiRequest<LifecycleOk>(endpoints.botStop(id), {
    method: 'POST',
    signal,
  })
}

/** Ответ migrate: assignment переведён; actual догонит agent (migrating → …). */
export type MigrateOk = {
  status: string
  bot_id: string
  to_node_id: string
}

/**
 * POST /v1/bots/{id}/migrate — тело `{ to_node_id }` (обязателен).
 * Без выбора ноды клиент не должен вызывать; сервер вернёт 400.
 */
export function migrateBot(
  id: string,
  toNodeId: string,
  signal?: AbortSignal,
): Promise<MigrateOk> {
  return apiRequest<MigrateOk>(endpoints.botMigrate(id), {
    method: 'POST',
    body: { to_node_id: toNodeId },
    signal,
  })
}

/**
 * Собрать тело create без лишних полей.
 * Для non-custom не кладём custom_name / artifact_path / start_command / workdir.
 */
export function buildCreateBotBody(input: {
  name: string
  bot_type: BotType
  channel: BotChannel
  run_mode: BotRunMode
  port: number
  token_ref: string
  desired_state: DesiredState
  assigned_node_id: string
  custom_name: string
  artifact_path: string
  start_command: string
  workdir: string
}): CreateBotBody {
  const body: CreateBotBody = {
    name: input.name,
    bot_type: input.bot_type,
    channel: input.channel,
    run_mode: input.run_mode,
    port: input.port,
    token_ref: input.token_ref,
    desired_state: input.desired_state,
  }

  const nodeId = input.assigned_node_id.trim()
  if (nodeId) {
    body.assigned_node_id = nodeId
  }

  if (input.bot_type === 'custom') {
    body.custom_name = input.custom_name.trim()
    body.start_command = input.start_command.trim()
    const artifact = input.artifact_path.trim()
    if (artifact) {
      body.artifact_path = artifact
    }
    const workdir = input.workdir.trim()
    if (workdir) {
      body.workdir = workdir
    }
  }

  return body
}
