import { useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { ApiError } from '../api/client'
import { listBots, listNodes } from '../api/lists'
import { buildPatchBotBody, patchBot } from '../api/mutations'
import type { Bot, Node } from '../api/types'
import { EmptyBlock, ErrorBlock, LoadingBlock, PageToolbar } from '../layout/PageStates'
import { shortId } from '../lib/shortId'
import { useFetchList } from '../lib/useFetchList'

type EditSnapshot = {
  bots: Bot[]
  nodes: Node[]
}

async function fetchEditSnapshot(signal: AbortSignal): Promise<EditSnapshot> {
  // Отдельного GET /v1/bots/{id} нет — берём из списка (docs/frontend.md §5.3).
  const [bots, nodes] = await Promise.all([listBots(signal), listNodes(signal)])
  return { bots, nodes }
}

/**
 * Текст для textarea scenario_config из текущего значения бота.
 * Пустой/null → пустая строка (omit при submit, если не меняли).
 */
function formatScenarioConfigText(config: Record<string, unknown> | null): string {
  if (config === null || Object.keys(config).length === 0) {
    return ''
  }
  try {
    return JSON.stringify(config, null, 2)
  } catch {
    return ''
  }
}

/**
 * Редактирование бота (/bots/:id/edit) — PATCH полей без Start/Stop (UI-6.2).
 * token_ref: пустое поле = не менять (ключ не попадает в JSON).
 *
 * key={id} снаружи: useFetchList не зависит от id, при смене :id нужен remount.
 */
export function BotEditPage() {
  const { id = '' } = useParams<{ id: string }>()
  return <BotEditBody key={id} id={id} />
}

function BotEditBody({ id }: { id: string }) {
  const snapshot = useFetchList(fetchEditSnapshot)
  const showInitial = snapshot.loading && snapshot.data === null
  const bot = snapshot.data?.bots.find((b) => b.id === id) ?? null
  const nodes = snapshot.data?.nodes ?? []

  return (
    <main>
      <PageToolbar
        title={bot ? `Редактировать: ${bot.name}` : 'Редактировать бота'}
        onRefresh={snapshot.refresh}
        refreshing={snapshot.loading && snapshot.data !== null}
        actions={
          <>
            {id ? (
              <Link to={`/bots/${id}`} className="btn btn--secondary">
                К карточке
              </Link>
            ) : null}
            <Link to="/bots" className="btn btn--secondary">
              К списку
            </Link>
          </>
        }
      />

      {snapshot.error ? (
        <ErrorBlock message={snapshot.error} onRetry={snapshot.refresh} />
      ) : null}

      {showInitial ? <LoadingBlock /> : null}

      {!showInitial && !snapshot.error && !bot ? (
        <EmptyBlock
          message={`Бот ${shortId(id)} не найден`}
          action={
            <Link to="/bots" className="btn btn--secondary">
              К списку ботов
            </Link>
          }
        />
      ) : null}

      {/* Форма с key=bot.id — сброс локального state при смене загруженного бота. */}
      {!showInitial && bot ? (
        <BotEditForm key={bot.id} bot={bot} nodes={nodes} />
      ) : null}
    </main>
  )
}

type BotEditFormProps = {
  bot: Bot
  nodes: Node[]
}

type EditFormState = {
  /** Новый token_ref; пусто = omit в PATCH (не затирать секрет). */
  token_ref: string
  assigned_node_id: string
  scenario_config_text: string
}

/**
 * Форма PATCH: token_ref / assigned_node_id / scenario_config.
 * Start/Stop намеренно отсутствуют — lifecycle на карточке.
 */
function BotEditForm({ bot, nodes }: BotEditFormProps) {
  const navigate = useNavigate()
  const originalScenarioText = formatScenarioConfigText(bot.scenario_config)

  const [form, setForm] = useState<EditFormState>(() => ({
    token_ref: '',
    assigned_node_id: bot.assigned_node_id ?? '',
    scenario_config_text: originalScenarioText,
  }))
  const [fieldError, setFieldError] = useState<string | null>(null)
  const [apiError, setApiError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const currentNodeMissing =
    !!bot.assigned_node_id && !nodes.some((n) => n.id === bot.assigned_node_id)

  function patchField<K extends keyof EditFormState>(key: K, value: EditFormState[K]) {
    setForm((prev) => ({ ...prev, [key]: value }))
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setFieldError(null)
    setApiError(null)

    const { body, clientError } = buildPatchBotBody({
      token_ref: form.token_ref,
      assigned_node_id: form.assigned_node_id,
      original_assigned_node_id: bot.assigned_node_id,
      scenario_config_text: form.scenario_config_text,
      original_scenario_config_text: originalScenarioText,
    })
    if (clientError) {
      setFieldError(clientError)
      return
    }

    setSubmitting(true)
    try {
      // Ответ API с masked token_ref — на карточку, plaintext не логируем.
      const updated = await patchBot(bot.id, body)
      navigate(`/bots/${updated.id}`)
    } catch (err) {
      if (err instanceof ApiError) {
        setApiError(err.message)
      } else {
        setApiError(err instanceof Error ? err.message : 'Не удалось сохранить бота')
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <form className="bot-form" onSubmit={handleSubmit} noValidate>
      <fieldset className="bot-form__group" disabled={submitting}>
        <legend>Секрет</legend>
        <p className="bot-form__hint">
          Текущий token_ref (masked):{' '}
          <span className="mono">{bot.token_ref}</span>
        </p>
        <label className="bot-form__field">
          <span>новый token_ref</span>
          <input
            type="password"
            name="token_ref"
            value={form.token_ref}
            onChange={(e) => patchField('token_ref', e.target.value)}
            autoComplete="off"
            placeholder="оставьте пустым, чтобы не менять"
          />
        </label>
      </fieldset>

      <fieldset className="bot-form__group" disabled={submitting}>
        <legend>Размещение</legend>
        <label className="bot-form__field">
          <span>assigned_node_id</span>
          <select
            name="assigned_node_id"
            value={form.assigned_node_id}
            onChange={(e) => patchField('assigned_node_id', e.target.value)}
          >
            {!form.assigned_node_id ? (
              <option value="">не назначена</option>
            ) : null}
            {/* Текущая нода могла исчезнуть из списка — оставляем выбранной. */}
            {currentNodeMissing && bot.assigned_node_id ? (
              <option value={bot.assigned_node_id}>
                {bot.assigned_node_id} (текущая)
              </option>
            ) : null}
            {nodes.map((node) => (
              <option key={node.id} value={node.id}>
                {node.hostname} ({node.status})
              </option>
            ))}
          </select>
        </label>
      </fieldset>

      <fieldset className="bot-form__group" disabled={submitting}>
        <legend>Сценарий</legend>
        <label className="bot-form__field">
          <span>scenario_config (JSON)</span>
          <textarea
            name="scenario_config"
            className="bot-form__textarea"
            rows={10}
            value={form.scenario_config_text}
            onChange={(e) => patchField('scenario_config_text', e.target.value)}
            spellCheck={false}
            placeholder="оставьте пустым или без изменений, чтобы не менять"
          />
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
          {submitting ? 'Сохранение…' : 'Сохранить'}
        </button>
        <Link to={`/bots/${bot.id}`} className="btn btn--secondary">
          Отмена
        </Link>
      </div>
    </form>
  )
}
