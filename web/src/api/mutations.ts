import { apiRequest } from './client'
import { endpoints } from './endpoints'
import type {
  Bot,
  BotChannel,
  BotRunMode,
  BotType,
  DesiredState,
} from './types'
import { isUUID } from '../lib/uuid'

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
  /** Опциональный UUID клиента; omit если не задан. */
  client_id?: string
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

/**
 * Тело PATCH /v1/bots/{id} — только опциональные поля (snake_case).
 * Пустой token_ref на сервере затирает секрет: ключ в JSON только если есть новое значение.
 */
export type PatchBotBody = {
  token_ref?: string
  assigned_node_id?: string
  /** Пустая строка сбрасывает client_id в null. */
  client_id?: string
  scenario_config?: Record<string, unknown>
  config_version?: number
}

/** PATCH /v1/bots/{id} → 200 + Bot DTO (masked). */
export function patchBot(
  id: string,
  body: PatchBotBody,
  signal?: AbortSignal,
): Promise<Bot> {
  return apiRequest<Bot>(endpoints.bot(id), {
    method: 'PATCH',
    body,
    signal,
  })
}

/**
 * Собрать PATCH-тело только из изменённых/явно заданных полей.
 * token_ref: пустой input → omit (иначе сервер запишет "").
 * scenario_config: пусто или без изменений → omit; невалидный JSON → clientError.
 */
export function buildPatchBotBody(input: {
  token_ref: string
  assigned_node_id: string
  original_assigned_node_id: string | null
  client_id: string
  original_client_id: string | null
  scenario_config_text: string
  original_scenario_config_text: string
}): { body: PatchBotBody; clientError: string | null } {
  const body: PatchBotBody = {}

  const tokenRef = input.token_ref.trim()
  if (tokenRef) {
    body.token_ref = tokenRef
  }

  const nodeId = input.assigned_node_id.trim()
  const originalNode = (input.original_assigned_node_id ?? '').trim()
  if (nodeId !== originalNode) {
    // Пустую строку не шлём — assigned_node_id на сервере *string; clear не в scope UI-6.2.
    if (!nodeId) {
      return { body: {}, clientError: 'Укажите assigned_node_id' }
    }
    body.assigned_node_id = nodeId
  }

  const clientId = input.client_id.trim()
  const originalClient = (input.original_client_id ?? '').trim()
  if (clientId !== originalClient) {
    if (clientId && !isUUID(clientId)) {
      return { body: {}, clientError: 'Некорректный client_id (нужен UUID)' }
    }
    // Пустая строка на сервере сбрасывает client_id в null.
    body.client_id = clientId
  }

  const scenarioText = input.scenario_config_text.trim()
  const originalScenario = input.original_scenario_config_text.trim()
  // Непустой текст: всегда валидируем JSON (в т.ч. до submit при правках).
  // Пусто = «не менять» (omit). Явная очистка конфига — ввести {}.
  if (scenarioText) {
    if (scenarioText !== originalScenario) {
      try {
        const parsed: unknown = JSON.parse(scenarioText)
        if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
          return {
            body: {},
            clientError: 'scenario_config должен быть JSON-объектом',
          }
        }
        body.scenario_config = parsed as Record<string, unknown>
      } catch {
        return { body: {}, clientError: 'Невалидный JSON в scenario_config' }
      }
    }
  }

  if (
    body.token_ref === undefined &&
    body.assigned_node_id === undefined &&
    body.client_id === undefined &&
    body.scenario_config === undefined
  ) {
    return { body: {}, clientError: 'Нет изменений для сохранения' }
  }

  return { body, clientError: null }
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
  client_id: string
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

  const clientId = input.client_id.trim()
  if (clientId) {
    body.client_id = clientId
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
