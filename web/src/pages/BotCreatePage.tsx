import { useMemo, useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router'
import { ApiError } from '../api/client'
import { listBots, listNodes } from '../api/lists'
import { buildCreateBotBody, createBot } from '../api/mutations'
import type { BotChannel, BotRunMode, BotType, DesiredState } from '../api/types'
import { ErrorBlock, LoadingBlock, PageToolbar } from '../layout/PageStates'
import { useFetchList } from '../lib/useFetchList'
import { isUUID } from '../lib/uuid'
import { useToast } from '../toast/ToastContext'

const BOT_TYPE_OPTIONS: BotType[] = ['default', 'default_extended', 'custom']
const CHANNEL_OPTIONS: BotChannel[] = ['telegram', 'max']
const RUN_MODE_OPTIONS: BotRunMode[] = ['webhook', 'polling']

type FormState = {
  name: string
  bot_type: BotType
  channel: BotChannel
  custom_name: string
  client_id: string
  assigned_node_id: string
  port: string
  run_mode: BotRunMode
  token_ref: string
  artifact_path: string
  start_command: string
  workdir: string
  desired_state: DesiredState
}

const INITIAL_FORM: FormState = {
  name: '',
  bot_type: 'default',
  channel: 'telegram',
  custom_name: '',
  client_id: '',
  assigned_node_id: '',
  port: '',
  run_mode: 'webhook',
  token_ref: '',
  artifact_path: '',
  start_command: '',
  workdir: '',
  desired_state: 'stopped',
}

type CreateSnapshot = {
  bots: Awaited<ReturnType<typeof listBots>>
  nodes: Awaited<ReturnType<typeof listNodes>>
}

async function fetchCreateSnapshot(signal: AbortSignal): Promise<CreateSnapshot> {
  const [bots, nodes] = await Promise.all([listBots(signal), listNodes(signal)])
  return { bots, nodes }
}

/**
 * Клиентская валидация до POST (docs/frontend.md §7.6).
 * Возвращает RU-сообщение или null, если ок.
 */
function validateForm(form: FormState, occupiedPorts: Set<number>): string | null {
  if (!form.name.trim()) {
    return 'Укажите имя'
  }

  const port = Number(form.port)
  if (!Number.isInteger(port) || port <= 0) {
    return 'Некорректный порт'
  }
  if (occupiedPorts.has(port)) {
    return 'Порт уже занят'
  }

  if (form.bot_type === 'custom') {
    if (!form.custom_name.trim()) {
      return 'Для custom укажите custom_name'
    }
    if (!form.start_command.trim()) {
      return 'Укажите start_command'
    }
  }

  if (!form.token_ref.trim()) {
    return 'Укажите token_ref'
  }

  const clientId = form.client_id.trim()
  if (clientId && !isUUID(clientId)) {
    return 'Некорректный client_id (нужен UUID)'
  }

  return null
}

/**
 * Создать бота (/bots/new) — форма групп полей + POST /v1/bots (UI-4).
 * Успех 201 → /bots/:id. Секрет token_ref в логи не пишем.
 */
export function BotCreatePage() {
  const navigate = useNavigate()
  const toast = useToast()
  const snapshot = useFetchList(fetchCreateSnapshot)

  const [form, setForm] = useState<FormState>(INITIAL_FORM)
  const [fieldError, setFieldError] = useState<string | null>(null)
  const [apiError, setApiError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const occupiedPorts = useMemo(() => {
    const set = new Set<number>()
    for (const bot of snapshot.data?.bots ?? []) {
      set.add(bot.port)
    }
    return set
  }, [snapshot.data?.bots])

  const isCustom = form.bot_type === 'custom'
  const showInitial = snapshot.loading && snapshot.data === null

  function patch<K extends keyof FormState>(key: K, value: FormState[K]) {
    setForm((prev) => ({ ...prev, [key]: value }))
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setFieldError(null)
    setApiError(null)

    const clientError = validateForm(form, occupiedPorts)
    if (clientError) {
      setFieldError(clientError)
      return
    }

    const port = Number(form.port)
    const body = buildCreateBotBody({
      name: form.name.trim(),
      bot_type: form.bot_type,
      channel: form.channel,
      run_mode: form.run_mode,
      port,
      token_ref: form.token_ref.trim(),
      desired_state: form.desired_state,
      assigned_node_id: form.assigned_node_id,
      client_id: form.client_id,
      custom_name: form.custom_name,
      artifact_path: form.artifact_path,
      start_command: form.start_command,
      workdir: form.workdir,
    })

    setSubmitting(true)
    try {
      const bot = await createBot(body)
      toast.success(`Бот «${bot.name}» создан`)
      navigate(`/bots/${bot.id}`)
    } catch (err) {
      // 400/409 и прочие — текст error из ApiError (без токена).
      const message =
        err instanceof ApiError
          ? err.message
          : err instanceof Error
            ? err.message
            : 'Не удалось создать бота'
      setApiError(message)
      toast.error(message)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <main>
      <PageToolbar
        title="Создать бота"
        onRefresh={snapshot.refresh}
        refreshing={snapshot.loading && snapshot.data !== null}
        actions={
          <Link to="/bots" className="btn btn--secondary">
            К списку
          </Link>
        }
      />

      {snapshot.error ? (
        <ErrorBlock message={snapshot.error} onRetry={snapshot.refresh} />
      ) : null}

      {showInitial ? <LoadingBlock /> : null}

      {!showInitial && !snapshot.error ? (
        <form className="bot-form" onSubmit={handleSubmit} noValidate>
          <fieldset className="bot-form__group" disabled={submitting}>
            <legend>Идентичность</legend>
            <label className="bot-form__field">
              <span>name</span>
              <input
                type="text"
                name="name"
                value={form.name}
                onChange={(e) => patch('name', e.target.value)}
                autoComplete="off"
                required
              />
            </label>
            <label className="bot-form__field">
              <span>bot_type</span>
              <select
                name="bot_type"
                value={form.bot_type}
                onChange={(e) => patch('bot_type', e.target.value as BotType)}
              >
                {BOT_TYPE_OPTIONS.map((v) => (
                  <option key={v} value={v}>
                    {v}
                  </option>
                ))}
              </select>
            </label>
            <label className="bot-form__field">
              <span>channel</span>
              <select
                name="channel"
                value={form.channel}
                onChange={(e) => patch('channel', e.target.value as BotChannel)}
              >
                {CHANNEL_OPTIONS.map((v) => (
                  <option key={v} value={v}>
                    {v}
                  </option>
                ))}
              </select>
            </label>
            <label className="bot-form__field">
              <span>client_id</span>
              <input
                type="text"
                name="client_id"
                className="mono"
                value={form.client_id}
                onChange={(e) => patch('client_id', e.target.value)}
                autoComplete="off"
                placeholder="необязательно, UUID"
                spellCheck={false}
              />
            </label>
            {isCustom ? (
              <label className="bot-form__field">
                <span>custom_name</span>
                <input
                  type="text"
                  name="custom_name"
                  value={form.custom_name}
                  onChange={(e) => patch('custom_name', e.target.value)}
                  autoComplete="off"
                />
              </label>
            ) : null}
          </fieldset>

          <fieldset className="bot-form__group" disabled={submitting}>
            <legend>Размещение</legend>
            <label className="bot-form__field">
              <span>assigned_node_id</span>
              <select
                name="assigned_node_id"
                value={form.assigned_node_id}
                onChange={(e) => patch('assigned_node_id', e.target.value)}
              >
                <option value="">по умолчанию (NODE_ID API)</option>
                {(snapshot.data?.nodes ?? []).map((node) => (
                  <option key={node.id} value={node.id}>
                    {node.hostname} ({node.status})
                  </option>
                ))}
              </select>
            </label>
            <label className="bot-form__field">
              <span>port</span>
              <input
                type="number"
                name="port"
                min={1}
                step={1}
                value={form.port}
                onChange={(e) => patch('port', e.target.value)}
                required
              />
            </label>
            <label className="bot-form__field">
              <span>run_mode</span>
              <select
                name="run_mode"
                value={form.run_mode}
                onChange={(e) => patch('run_mode', e.target.value as BotRunMode)}
              >
                {RUN_MODE_OPTIONS.map((v) => (
                  <option key={v} value={v}>
                    {v}
                  </option>
                ))}
              </select>
            </label>
          </fieldset>

          <fieldset className="bot-form__group" disabled={submitting}>
            <legend>Секрет</legend>
            <label className="bot-form__field">
              <span>token_ref</span>
              <input
                type="password"
                name="token_ref"
                value={form.token_ref}
                onChange={(e) => patch('token_ref', e.target.value)}
                autoComplete="off"
                required
              />
            </label>
          </fieldset>

          {isCustom ? (
            <fieldset className="bot-form__group" disabled={submitting}>
              <legend>Custom</legend>
              <label className="bot-form__field">
                <span>artifact_path</span>
                <input
                  type="text"
                  name="artifact_path"
                  value={form.artifact_path}
                  onChange={(e) => patch('artifact_path', e.target.value)}
                  autoComplete="off"
                />
              </label>
              <label className="bot-form__field">
                <span>start_command</span>
                <input
                  type="text"
                  name="start_command"
                  value={form.start_command}
                  onChange={(e) => patch('start_command', e.target.value)}
                  autoComplete="off"
                />
              </label>
              <label className="bot-form__field">
                <span>workdir</span>
                <input
                  type="text"
                  name="workdir"
                  value={form.workdir}
                  onChange={(e) => patch('workdir', e.target.value)}
                  autoComplete="off"
                />
              </label>
            </fieldset>
          ) : null}

          <fieldset className="bot-form__group" disabled={submitting}>
            <legend>Старт</legend>
            <label className="bot-form__field">
              <span>desired_state</span>
              <select
                name="desired_state"
                value={form.desired_state}
                onChange={(e) => patch('desired_state', e.target.value as DesiredState)}
              >
                <option value="stopped">stopped</option>
                <option value="running">running</option>
              </select>
            </label>
          </fieldset>

          {fieldError ? (
            <p className="bot-form__error" role="alert">
              {fieldError}
            </p>
          ) : null}
          {apiError ? (
            <p className="bot-form__error" role="alert">
              {apiError}
            </p>
          ) : null}

          <div className="bot-form__actions">
            <button type="submit" className="btn btn--primary" disabled={submitting}>
              {submitting ? 'Создание…' : 'Создать'}
            </button>
            <Link to="/bots" className="btn btn--secondary">
              Отмена
            </Link>
          </div>
        </form>
      ) : null}
    </main>
  )
}
