import type { ReactNode } from 'react'
import { Link, useParams } from 'react-router'
import { listBotEvents, listBots, listNodes } from '../api/lists'
import type { Bot, BotEvent, Node } from '../api/types'
import { EmptyBlock, ErrorBlock, LoadingBlock, PageToolbar } from '../layout/PageStates'
import { StatePill } from '../layout/StatePill'
import { formatAbsoluteShort, formatRelativeRu } from '../lib/formatTime'
import { shortId } from '../lib/shortId'
import { useFetchList } from '../lib/useFetchList'

type DetailSnapshot = {
  bots: Bot[]
  nodesById: Map<string, Node>
}

async function fetchDetailSnapshot(signal: AbortSignal): Promise<DetailSnapshot> {
  // Отдельного GET /v1/bots/{id} нет — берём из списка (docs/frontend.md §5.3).
  const [bots, nodes] = await Promise.all([listBots(signal), listNodes(signal)])
  return {
    bots,
    nodesById: new Map(nodes.map((n) => [n.id, n])),
  }
}

/** Пустое значение в паспорте. */
function dash(value: string | number | null | undefined): string {
  if (value === null || value === undefined || value === '') {
    return '—'
  }
  return String(value)
}

/** scenario_config — компактный JSON или прочерк (редактирование — P1). */
function formatScenarioConfig(config: Record<string, unknown> | null): string {
  if (config === null || Object.keys(config).length === 0) {
    return '—'
  }
  try {
    return JSON.stringify(config)
  } catch {
    return '—'
  }
}

/**
 * Сортировка событий: новые сверху (по at, затем id).
 * API может отдать любой порядок — нормализуем на клиенте.
 */
function sortEventsNewestFirst(events: BotEvent[]): BotEvent[] {
  return [...events].sort((a, b) => {
    const ta = Date.parse(a.at)
    const tb = Date.parse(b.at)
    if (!Number.isNaN(ta) && !Number.isNaN(tb) && ta !== tb) {
      return tb - ta
    }
    return b.id.localeCompare(a.id)
  })
}

/**
 * Карточка бота (/bots/:id) — паспорт + лента событий (UI-3).
 * Start/Stop/Migrate / PATCH — вне scope (UI-4/5).
 *
 * key={id} снаружи: useFetchList не зависит от id, при смене :id нужен remount.
 */
export function BotDetailPage() {
  const { id = '' } = useParams<{ id: string }>()
  // Remount при смене :id — иначе useFetchList оставит старый snapshot.
  return <BotDetailBody key={id} id={id} />
}

function BotDetailBody({ id }: { id: string }) {
  const passport = useFetchList(fetchDetailSnapshot)
  const eventsState = useFetchList((signal) => listBotEvents(id, signal))

  const showPassportInitial = passport.loading && passport.data === null
  const bot = passport.data?.bots.find((b) => b.id === id) ?? null
  const nodeHostname =
    bot?.assigned_node_id && passport.data
      ? (passport.data.nodesById.get(bot.assigned_node_id)?.hostname ?? null)
      : null

  /** Обновить паспорт и ленту одним кликом в тулбаре. */
  function refreshAll() {
    passport.refresh()
    eventsState.refresh()
  }

  const refreshing =
    (passport.loading && passport.data !== null) ||
    (eventsState.loading && eventsState.data !== null)

  return (
    <main>
      <PageToolbar
        title={bot ? bot.name : 'Бот'}
        onRefresh={refreshAll}
        refreshing={refreshing}
        actions={
          <Link to="/bots" className="btn btn--secondary">
            К списку
          </Link>
        }
      />

      {passport.error ? (
        <ErrorBlock message={passport.error} onRetry={passport.refresh} />
      ) : null}

      {showPassportInitial ? <LoadingBlock /> : null}

      {!showPassportInitial && !passport.error && !bot ? (
        <EmptyBlock
          message={`Бот ${shortId(id)} не найден`}
          action={
            <Link to="/bots" className="btn btn--secondary">
              К списку ботов
            </Link>
          }
        />
      ) : null}

      {!showPassportInitial && bot ? (
        <BotPassport bot={bot} nodeHostname={nodeHostname} />
      ) : null}

      {/* Лента событий — отдельный loading/error/refresh (GET /v1/bots/{id}/events). */}
      {id ? (
        <BotEventsSection
          events={eventsState.data}
          error={eventsState.error}
          loading={eventsState.loading}
          onRefresh={eventsState.refresh}
        />
      ) : null}
    </main>
  )
}

type BotPassportProps = {
  bot: Bot
  nodeHostname: string | null
}

/** Паспорт: все ключевые поля §4.2; desired/actual крупно; last_error плашкой. */
function BotPassport({ bot, nodeHostname }: BotPassportProps) {
  return (
    <section className="bot-passport" aria-label="Паспорт бота">
      <div className="bot-passport__states bot-passport__states--large">
        <div className="bot-passport__state">
          <span className="bot-passport__label">desired</span>
          <StatePill label={bot.desired_state} />
        </div>
        <div className="bot-passport__state">
          <span className="bot-passport__label">actual</span>
          <StatePill label={bot.actual_state} />
        </div>
      </div>

      {bot.last_error ? (
        <div className="bot-passport__error" role="status">
          <span className="bot-passport__error-label">last_error</span>
          {bot.last_error}
        </div>
      ) : null}

      <dl className="bot-passport__fields">
        <Field label="id">
          <span className="mono">{bot.id}</span>
        </Field>
        <Field label="name">{bot.name}</Field>
        <Field label="bot_type">
          <span className="mono">{bot.bot_type}</span>
        </Field>
        <Field label="custom_name">{dash(bot.custom_name)}</Field>
        <Field label="channel">{bot.channel}</Field>
        <Field label="run_mode">{bot.run_mode}</Field>
        <Field label="port">
          <span className="mono">{bot.port}</span>
        </Field>
        <Field label="token_ref">
          {/* Уже masked от API — plaintext не запрашиваем и не угадываем. */}
          <span className="mono" title="Маскированный ref из API">
            {bot.token_ref}
          </span>
        </Field>
        <Field label="client_id">
          <span className="mono">{dash(bot.client_id)}</span>
        </Field>
        <Field label="assigned_node_id">
          {bot.assigned_node_id ? (
            <Link
              to="/nodes"
              className="bot-passport__link mono"
              title={bot.assigned_node_id}
            >
              {nodeHostname ?? shortId(bot.assigned_node_id)}
            </Link>
          ) : (
            '—'
          )}
        </Field>
        <Field label="runtime_id">
          {bot.runtime_id ? (
            <Link
              to="/runtimes"
              className="bot-passport__link mono"
              title={bot.runtime_id}
            >
              {shortId(bot.runtime_id)}
            </Link>
          ) : (
            '—'
          )}
        </Field>
        <Field label="artifact_path">
          <span className="mono">{dash(bot.artifact_path)}</span>
        </Field>
        <Field label="repo_url">
          <span className="mono">{dash(bot.repo_url)}</span>
        </Field>
        <Field label="start_command">
          <span className="mono">{dash(bot.start_command)}</span>
        </Field>
        <Field label="config_version">
          <span className="mono">{bot.config_version}</span>
        </Field>
        <Field label="scenario_config">
          <span className="mono bot-passport__scenario">
            {formatScenarioConfig(bot.scenario_config)}
          </span>
        </Field>
        <Field label="created_at">
          <time dateTime={bot.created_at} title={formatAbsoluteShort(bot.created_at)}>
            {formatRelativeRu(bot.created_at)}
          </time>
        </Field>
        <Field label="updated_at">
          <time dateTime={bot.updated_at} title={formatAbsoluteShort(bot.updated_at)}>
            {formatRelativeRu(bot.updated_at)}
          </time>
        </Field>
      </dl>
    </section>
  )
}

function Field({
  label,
  children,
}: {
  label: string
  children: ReactNode
}) {
  return (
    <div>
      <dt>{label}</dt>
      <dd>{children}</dd>
    </div>
  )
}

type BotEventsSectionProps = {
  events: BotEvent[] | null
  error: string | null
  loading: boolean
  onRefresh: () => void
}

/** Лента GET /v1/bots/{id}/events: at · type · message; новые сверху. */
function BotEventsSection({ events, error, loading, onRefresh }: BotEventsSectionProps) {
  const showInitial = loading && events === null
  const sorted = events ? sortEventsNewestFirst(events) : []

  return (
    <section className="bot-events" aria-label="События бота">
      <div className="bot-events__toolbar">
        <h2 className="bot-events__title">События</h2>
        <button
          type="button"
          className="btn btn--secondary"
          onClick={onRefresh}
          disabled={loading && events !== null}
        >
          {loading && events !== null ? 'Обновление…' : 'Обновить'}
        </button>
      </div>

      {error ? <ErrorBlock message={error} onRetry={onRefresh} /> : null}

      {showInitial ? <LoadingBlock label="Загрузка событий…" /> : null}

      {!showInitial && !error && sorted.length === 0 ? (
        <EmptyBlock message="Событий пока нет" />
      ) : null}

      {!showInitial && sorted.length > 0 ? (
        <div className="table-wrap">
          <table className="data-table bot-events__table">
            <thead>
              <tr>
                <th>at</th>
                <th>type</th>
                <th>message</th>
              </tr>
            </thead>
            <tbody>
              {sorted.map((ev) => (
                <tr key={ev.id}>
                  <td className="mono" title={formatAbsoluteShort(ev.at)}>
                    <time dateTime={ev.at}>{formatRelativeRu(ev.at)}</time>
                  </td>
                  <td className="mono">{ev.type}</td>
                  <td>{ev.message}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </section>
  )
}
