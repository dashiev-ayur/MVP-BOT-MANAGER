/**
 * Типы JSON control-api в каноническом snake_case (docs/frontend.md §4, §5.5).
 * Даты — RFC3339-строки, как отдаёт Go time.Time.
 */

export type NodeStatus = 'online' | 'offline' | 'draining'

export type RuntimeKind = 'bot_runner' | 'custom_bot'

export type DesiredState = 'running' | 'stopped'

export type ActualState =
  | 'unknown'
  | 'starting'
  | 'running'
  | 'stopping'
  | 'stopped'
  | 'failed'
  | 'migrating'

export type BotType = 'custom' | 'default' | 'default_extended'

export type BotChannel = 'telegram' | 'max'

export type BotRunMode = 'webhook' | 'polling'

/** Тело ошибки API: {"error":"..."} */
export type ApiErrorBody = {
  error: string
}

export type Node = {
  id: string
  hostname: string
  status: NodeStatus
  last_seen_at: string
  agent_version: string | null
  meta: Record<string, unknown> | null
  created_at: string
  updated_at: string
}

export type Runtime = {
  id: string
  kind: RuntimeKind
  name: string
  start_command: string
  workdir: string | null
  env: Record<string, unknown> | null
  desired_state: DesiredState
  actual_state: ActualState
  assigned_node_id: string | null
  lease_owner: string | null
  lease_until: string | null
  pid: number | null
  exit_code: number | null
  last_error: string | null
  config_version: number
  created_at: string
  updated_at: string
}

export type Bot = {
  id: string
  client_id: string | null
  name: string
  bot_type: BotType
  custom_name: string | null
  channel: BotChannel
  run_mode: BotRunMode
  port: number
  /** В ответах API уже замаскирован — plaintext не показывать. */
  token_ref: string
  runtime_id: string | null
  artifact_path: string | null
  repo_url: string | null
  start_command: string | null
  desired_state: DesiredState
  actual_state: ActualState
  assigned_node_id: string | null
  last_error: string | null
  config_version: number
  scenario_config: Record<string, unknown> | null
  created_at: string
  updated_at: string
}

export type BotEvent = {
  id: string
  bot_id: string
  type: string
  message: string
  at: string
  meta: Record<string, unknown> | null
}
