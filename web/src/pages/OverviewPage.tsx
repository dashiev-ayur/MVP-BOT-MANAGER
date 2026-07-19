import { Link } from 'react-router'
import { listBots, listNodes, listRuntimes } from '../api/lists'
import type { ActualState, Bot, Node, Runtime, RuntimeKind } from '../api/types'
import { EmptyBlock, ErrorBlock, LoadingBlock, PageToolbar } from '../layout/PageStates'
import { StatePill } from '../layout/StatePill'
import { shortId } from '../lib/shortId'
import { DEFAULT_POLL_INTERVAL_MS, useFetchList } from '../lib/useFetchList'

type OverviewSnapshot = {
  nodes: Node[]
  bots: Bot[]
  runtimes: Runtime[]
}

async function fetchOverview(signal: AbortSignal): Promise<OverviewSnapshot> {
  const [nodes, bots, runtimes] = await Promise.all([
    listNodes(signal),
    listBots(signal),
    listRuntimes(signal),
  ])
  return { nodes, bots, runtimes }
}

/**
 * Обзор (/) — сводки nodes/bots/runtimes + «Требуют внимания».
 * Только GET; клики ведут на списки/карточку.
 */
export function OverviewPage() {
  const { data, error, loading, refresh, updatedAt } = useFetchList(fetchOverview, {
    pollIntervalMs: DEFAULT_POLL_INTERVAL_MS,
  })
  const showInitial = loading && data === null

  const nodes = data?.nodes ?? []
  const bots = data?.bots ?? []
  const runtimes = data?.runtimes ?? []

  const nodeCounts = countBy(nodes, (n) => n.status)
  const botActualCounts = countBy(bots, (b) => b.actual_state)
  const runtimeKindCounts = countBy(runtimes, (r) => r.kind)
  const runtimeFailed = runtimes.filter((r) => r.actual_state === 'failed').length
  const runtimeUnknown = runtimes.filter((r) => r.actual_state === 'unknown').length

  const attentionBots = bots.filter(
    (b) => b.actual_state === 'failed' || Boolean(b.last_error?.trim()),
  )
  const attentionNodes = nodes.filter((n) => n.status === 'offline')
  const attentionRuntimes = runtimes.filter((r) => r.actual_state === 'failed')
  const attentionEmpty =
    attentionBots.length === 0 &&
    attentionNodes.length === 0 &&
    attentionRuntimes.length === 0

  return (
    <main>
      <PageToolbar
        title="Обзор"
        onRefresh={refresh}
        refreshing={loading && data !== null}
        updatedAt={updatedAt}
      />

      {error ? <ErrorBlock message={error} onRetry={refresh} /> : null}

      {showInitial ? <LoadingBlock /> : null}

      {!showInitial && data ? (
        <>
          <section className="overview-summaries" aria-label="Сводки">
            <div className="overview-line">
              <span className="overview-line__label">Ноды</span>
              <span className="overview-line__value">
                {nodeCounts.online ?? 0} online / {nodeCounts.offline ?? 0} offline
                {(nodeCounts.draining ?? 0) > 0
                  ? ` / ${nodeCounts.draining} draining`
                  : ''}
                <span className="overview-line__total"> (всего {nodes.length})</span>
              </span>
              <Link to="/nodes" className="overview-line__link">
                к списку
              </Link>
            </div>

            <div className="overview-line">
              <span className="overview-line__label">Боты</span>
              <span className="overview-line__value">
                {formatActualCounts(botActualCounts, bots.length)}
              </span>
              <Link to="/bots" className="overview-line__link">
                к списку
              </Link>
            </div>

            <div className="overview-line">
              <span className="overview-line__label">Runtimes</span>
              <span className="overview-line__value">
                {formatKindCounts(runtimeKindCounts)}
                {runtimeFailed > 0 || runtimeUnknown > 0
                  ? ` · failed ${runtimeFailed}, unknown ${runtimeUnknown}`
                  : ''}
                <span className="overview-line__total"> (всего {runtimes.length})</span>
              </span>
              <Link to="/runtimes" className="overview-line__link">
                к списку
              </Link>
            </div>
          </section>

          <section className="attention" aria-label="Требуют внимания">
            <h2 className="attention__title">Требуют внимания</h2>

            {attentionEmpty ? (
              <EmptyBlock message="Всё в порядке — проблемных сущностей нет" />
            ) : (
              <ul className="attention__list">
                {attentionBots.map((bot) => (
                  <li key={`bot-${bot.id}`} className="attention__item">
                    <StatePill label={bot.actual_state} />
                    <Link to={`/bots/${bot.id}`} className="attention__link">
                      Бот {bot.name}
                    </Link>
                    <span className="attention__detail cell-error">
                      {bot.last_error?.trim() || bot.actual_state}
                    </span>
                  </li>
                ))}
                {attentionNodes.map((node) => (
                  <li key={`node-${node.id}`} className="attention__item">
                    <StatePill label={node.status} />
                    <Link
                      to="/nodes"
                      className="attention__link"
                      title={node.id}
                    >
                      Нода {node.hostname}
                    </Link>
                    <span className="attention__detail mono">{shortId(node.id)}</span>
                  </li>
                ))}
                {attentionRuntimes.map((rt) => (
                  <li key={`rt-${rt.id}`} className="attention__item">
                    <StatePill label={rt.actual_state} />
                    <Link to="/runtimes" className="attention__link">
                      Runtime {rt.name}
                    </Link>
                    <span className="attention__detail cell-error">
                      {rt.last_error?.trim() || rt.kind}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </section>
        </>
      ) : null}
    </main>
  )
}

function countBy<T>(items: T[], keyFn: (item: T) => string): Record<string, number> {
  const out: Record<string, number> = {}
  for (const item of items) {
    const key = keyFn(item)
    out[key] = (out[key] ?? 0) + 1
  }
  return out
}

/** Компактная строка counts по actual_state. */
function formatActualCounts(
  counts: Record<string, number>,
  total: number,
): string {
  const order: ActualState[] = [
    'running',
    'failed',
    'starting',
    'stopping',
    'migrating',
    'stopped',
    'unknown',
  ]
  const parts = order
    .filter((k) => (counts[k] ?? 0) > 0)
    .map((k) => `${k} ${counts[k]}`)
  if (parts.length === 0) {
    return `всего ${total}`
  }
  return `${parts.join(' · ')} (всего ${total})`
}

function formatKindCounts(counts: Record<string, number>): string {
  const order: RuntimeKind[] = ['bot_runner', 'custom_bot']
  const parts = order
    .filter((k) => (counts[k] ?? 0) > 0)
    .map((k) => `${k} ${counts[k]}`)
  return parts.length > 0 ? parts.join(' · ') : 'нет'
}
