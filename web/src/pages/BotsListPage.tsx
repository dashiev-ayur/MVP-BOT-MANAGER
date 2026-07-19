import { useMemo } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router'
import { listBots, listNodes } from '../api/lists'
import type { Bot, Node } from '../api/types'
import { EmptyBlock, ErrorBlock, LoadingBlock, PageToolbar } from '../layout/PageStates'
import { StatePill } from '../layout/StatePill'
import {
  ACTUAL_STATE_OPTIONS,
  BOT_TYPE_OPTIONS,
  CHANNEL_OPTIONS,
  DESIRED_STATE_OPTIONS,
  botFiltersToSearchParams,
  filterBots,
  hasActiveBotFilters,
  parseBotFilters,
  type BotFilters,
} from '../lib/botFilters'
import { shortId } from '../lib/shortId'
import { DEFAULT_POLL_INTERVAL_MS, useFetchList } from '../lib/useFetchList'

type BotsSnapshot = {
  bots: Bot[]
  nodesById: Map<string, Node>
}

async function fetchBotsSnapshot(signal: AbortSignal): Promise<BotsSnapshot> {
  const [bots, nodes] = await Promise.all([listBots(signal), listNodes(signal)])
  return {
    bots,
    nodesById: new Map(nodes.map((n) => [n.id, n])),
  }
}

/**
 * Список ботов (/bots) — фильтры + CTA «Создать» → /bots/new (UI-2/4).
 * Фильтры client-side, состояние в URL query; клик → /bots/:id.
 */
export function BotsListPage() {
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const filters = useMemo(() => parseBotFilters(searchParams), [searchParams])

  const { data, error, loading, refresh, updatedAt } = useFetchList(fetchBotsSnapshot, {
    pollIntervalMs: DEFAULT_POLL_INTERVAL_MS,
  })
  const bots = data?.bots ?? []
  const nodesById = data?.nodesById ?? new Map<string, Node>()
  const visible = useMemo(() => filterBots(bots, filters), [bots, filters])
  const showInitial = loading && data === null

  function updateFilter<K extends keyof BotFilters>(key: K, value: string) {
    const next: BotFilters = { ...filters, [key]: value }
    setSearchParams(botFiltersToSearchParams(next), { replace: true })
  }

  function resetFilters() {
    setSearchParams(new URLSearchParams(), { replace: true })
  }

  // Опции нод/клиентов из загруженных данных (уникальные значения).
  const nodeOptions = useMemo(() => {
    const ids = new Set<string>()
    for (const bot of bots) {
      if (bot.assigned_node_id) ids.add(bot.assigned_node_id)
    }
    return [...ids].sort()
  }, [bots])

  const clientOptions = useMemo(() => {
    const ids = new Set<string>()
    for (const bot of bots) {
      if (bot.client_id) ids.add(bot.client_id)
    }
    return [...ids].sort()
  }, [bots])

  // runtime_id из данных ботов + значение из URL (если пришли со /runtimes).
  const runtimeOptions = useMemo(() => {
    const ids = new Set<string>()
    for (const bot of bots) {
      if (bot.runtime_id) ids.add(bot.runtime_id)
    }
    if (filters.runtime_id) ids.add(filters.runtime_id)
    return [...ids].sort()
  }, [bots, filters.runtime_id])

  return (
    <main>
      <PageToolbar
        title="Боты"
        onRefresh={refresh}
        refreshing={loading && data !== null}
        updatedAt={updatedAt}
        actions={
          <Link to="/bots/new" className="btn btn--primary">
            Создать
          </Link>
        }
      />

      {error ? <ErrorBlock message={error} onRetry={refresh} /> : null}

      {showInitial ? <LoadingBlock /> : null}

      {!showInitial ? (
        <form
          className="filters"
          onSubmit={(e) => e.preventDefault()}
          aria-label="Фильтры ботов"
        >
          <label className="filters__field">
            <span>type</span>
            <select
              value={filters.bot_type}
              onChange={(e) => updateFilter('bot_type', e.target.value)}
            >
              <option value="">все</option>
              {BOT_TYPE_OPTIONS.map((v) => (
                <option key={v} value={v}>
                  {v}
                </option>
              ))}
            </select>
          </label>

          <label className="filters__field">
            <span>desired</span>
            <select
              value={filters.desired_state}
              onChange={(e) => updateFilter('desired_state', e.target.value)}
            >
              <option value="">все</option>
              {DESIRED_STATE_OPTIONS.map((v) => (
                <option key={v} value={v}>
                  {v}
                </option>
              ))}
            </select>
          </label>

          <label className="filters__field">
            <span>actual</span>
            <select
              value={filters.actual_state}
              onChange={(e) => updateFilter('actual_state', e.target.value)}
            >
              <option value="">все</option>
              {ACTUAL_STATE_OPTIONS.map((v) => (
                <option key={v} value={v}>
                  {v}
                </option>
              ))}
            </select>
          </label>

          <label className="filters__field">
            <span>node</span>
            <select
              value={filters.assigned_node_id}
              onChange={(e) => updateFilter('assigned_node_id', e.target.value)}
            >
              <option value="">все</option>
              {nodeOptions.map((id) => (
                <option key={id} value={id}>
                  {nodesById.get(id)?.hostname ?? shortId(id)}
                </option>
              ))}
            </select>
          </label>

          <label className="filters__field">
            <span>client</span>
            <select
              value={filters.client_id}
              onChange={(e) => updateFilter('client_id', e.target.value)}
            >
              <option value="">все</option>
              {clientOptions.map((id) => (
                <option key={id} value={id}>
                  {shortId(id)}
                </option>
              ))}
            </select>
          </label>

          <label className="filters__field">
            <span>channel</span>
            <select
              value={filters.channel}
              onChange={(e) => updateFilter('channel', e.target.value)}
            >
              <option value="">все</option>
              {CHANNEL_OPTIONS.map((v) => (
                <option key={v} value={v}>
                  {v}
                </option>
              ))}
            </select>
          </label>

          <label className="filters__field">
            <span>runtime</span>
            <select
              value={filters.runtime_id}
              onChange={(e) => updateFilter('runtime_id', e.target.value)}
            >
              <option value="">все</option>
              {runtimeOptions.map((id) => (
                <option key={id} value={id}>
                  {shortId(id)}
                </option>
              ))}
            </select>
          </label>

          <button
            type="button"
            className="btn btn--secondary"
            onClick={resetFilters}
            disabled={!hasActiveBotFilters(filters)}
          >
            Сброс
          </button>
        </form>
      ) : null}

      {!showInitial && !error && bots.length === 0 ? (
        <EmptyBlock
          message="Нет ботов"
          action={
            <Link to="/bots/new" className="btn btn--primary">
              Создать
            </Link>
          }
        />
      ) : null}

      {!showInitial && bots.length > 0 && visible.length === 0 ? (
        <EmptyBlock
          message="Нет ботов по выбранным фильтрам"
          action={
            <button type="button" className="btn btn--secondary" onClick={resetFilters}>
              Сбросить фильтры
            </button>
          }
        />
      ) : null}

      {!showInitial && visible.length > 0 ? (
        <div className="table-wrap">
          <table className="data-table">
            <thead>
              <tr>
                <th>name</th>
                <th>type</th>
                <th>channel</th>
                <th>port</th>
                <th>node</th>
                <th>desired</th>
                <th>actual</th>
                <th>client</th>
                <th>last_error</th>
              </tr>
            </thead>
            <tbody>
              {visible.map((bot) => {
                const mismatch = bot.desired_state !== bot.actual_state
                const nodeLabel = bot.assigned_node_id
                  ? (nodesById.get(bot.assigned_node_id)?.hostname ??
                    shortId(bot.assigned_node_id))
                  : '—'
                return (
                  <tr
                    key={bot.id}
                    className={[
                      'data-table__row--clickable',
                      mismatch ? 'data-table__row--mismatch' : '',
                    ]
                      .filter(Boolean)
                      .join(' ')}
                    onClick={() => navigate(`/bots/${bot.id}`)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault()
                        navigate(`/bots/${bot.id}`)
                      }
                    }}
                    tabIndex={0}
                    role="link"
                    aria-label={`Бот ${bot.name}`}
                  >
                    <td>
                      <Link
                        to={`/bots/${bot.id}`}
                        className="table-link"
                        onClick={(e) => e.stopPropagation()}
                      >
                        {bot.name}
                      </Link>
                    </td>
                    <td className="mono">{bot.bot_type}</td>
                    <td>{bot.channel}</td>
                    <td className="mono">{bot.port}</td>
                    <td className="mono" title={bot.assigned_node_id ?? undefined}>
                      {nodeLabel}
                    </td>
                    <td>
                      <StatePill label={bot.desired_state} />
                    </td>
                    <td>
                      <StatePill label={bot.actual_state} />
                    </td>
                    <td className="mono" title={bot.client_id ?? undefined}>
                      {bot.client_id ? shortId(bot.client_id) : '—'}
                    </td>
                    <td className="cell-error" title={bot.last_error ?? undefined}>
                      {bot.last_error ?? '—'}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      ) : null}

      {!showInitial && bots.length > 0 ? (
        <p className="page-meta">
          Показано {visible.length} из {bots.length}
          {hasActiveBotFilters(filters) ? ' (фильтр)' : ''}
        </p>
      ) : null}
    </main>
  )
}
