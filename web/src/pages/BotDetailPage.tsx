import { Link, useParams } from 'react-router'
import { listBots, listNodes } from '../api/lists'
import type { Bot, Node } from '../api/types'
import { EmptyBlock, ErrorBlock, LoadingBlock, PageToolbar } from '../layout/PageStates'
import { StatePill } from '../layout/StatePill'
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

/**
 * Карточка бота (/bots/:id) — минимальный паспорт из list (заглушка до UI-3).
 * Events / start/stop/migrate — вне scope UI-2.
 */
export function BotDetailPage() {
  const { id = '' } = useParams<{ id: string }>()
  const { data, error, loading, refresh } = useFetchList(fetchDetailSnapshot)
  const showInitial = loading && data === null

  const bot = data?.bots.find((b) => b.id === id) ?? null
  const nodeHostname =
    bot?.assigned_node_id && data
      ? (data.nodesById.get(bot.assigned_node_id)?.hostname ?? null)
      : null

  return (
    <main>
      <PageToolbar
        title={bot ? bot.name : 'Бот'}
        onRefresh={refresh}
        refreshing={loading && data !== null}
        actions={
          <Link to="/bots" className="btn btn--secondary">
            К списку
          </Link>
        }
      />

      {error ? <ErrorBlock message={error} onRetry={refresh} /> : null}

      {showInitial ? <LoadingBlock /> : null}

      {!showInitial && !error && !bot ? (
        <EmptyBlock
          message={`Бот ${shortId(id)} не найден`}
          action={
            <Link to="/bots" className="btn btn--secondary">
              К списку ботов
            </Link>
          }
        />
      ) : null}

      {!showInitial && bot ? (
        <div className="bot-passport">
          <p className="bot-passport__hint">
            Минимальный паспорт (полный экран + события — UI-3).
          </p>

          <div className="bot-passport__states">
            <div>
              <span className="bot-passport__label">desired</span>
              <StatePill label={bot.desired_state} />
            </div>
            <div>
              <span className="bot-passport__label">actual</span>
              <StatePill label={bot.actual_state} />
            </div>
          </div>

          {bot.last_error ? (
            <div className="bot-passport__error" role="status">
              {bot.last_error}
            </div>
          ) : null}

          <dl className="bot-passport__fields">
            <div>
              <dt>id</dt>
              <dd className="mono">{bot.id}</dd>
            </div>
            <div>
              <dt>type</dt>
              <dd className="mono">{bot.bot_type}</dd>
            </div>
            <div>
              <dt>channel</dt>
              <dd>{bot.channel}</dd>
            </div>
            <div>
              <dt>port</dt>
              <dd className="mono">{bot.port}</dd>
            </div>
            <div>
              <dt>run_mode</dt>
              <dd>{bot.run_mode}</dd>
            </div>
            <div>
              <dt>node</dt>
              <dd className="mono" title={bot.assigned_node_id ?? undefined}>
                {nodeHostname ??
                  (bot.assigned_node_id ? shortId(bot.assigned_node_id) : '—')}
              </dd>
            </div>
            <div>
              <dt>client</dt>
              <dd className="mono">{bot.client_id ?? '—'}</dd>
            </div>
            <div>
              <dt>token_ref</dt>
              <dd className="mono">{bot.token_ref}</dd>
            </div>
            <div>
              <dt>runtime_id</dt>
              <dd className="mono">{bot.runtime_id ?? '—'}</dd>
            </div>
            {bot.custom_name ? (
              <div>
                <dt>custom_name</dt>
                <dd>{bot.custom_name}</dd>
              </div>
            ) : null}
          </dl>
        </div>
      ) : null}
    </main>
  )
}
