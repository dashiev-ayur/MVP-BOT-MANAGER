import { Link, useNavigate } from 'react-router'
import { listNodes, listRuntimes } from '../api/lists'
import type { Node, Runtime } from '../api/types'
import { EmptyBlock, ErrorBlock, LoadingBlock, PageToolbar } from '../layout/PageStates'
import { StatePill } from '../layout/StatePill'
import { formatAbsoluteShort, isLeaseExpired } from '../lib/formatTime'
import { shortId } from '../lib/shortId'
import { useFetchList } from '../lib/useFetchList'

type RuntimesSnapshot = {
  runtimes: Runtime[]
  nodesById: Map<string, Node>
}

async function fetchRuntimesSnapshot(signal: AbortSignal): Promise<RuntimesSnapshot> {
  // Ноды — только для hostname в колонке node; мутаций нет.
  const [runtimes, nodes] = await Promise.all([listRuntimes(signal), listNodes(signal)])
  return {
    runtimes,
    nodesById: new Map(nodes.map((n) => [n.id, n])),
  }
}

/** URL списка ботов, отфильтрованный по runtime (docs/frontend.md §7.10). */
function botsByRuntimeHref(runtimeId: string): string {
  return `/bots?runtime_id=${encodeURIComponent(runtimeId)}`
}

/**
 * Список runtimes (/runtimes) — P1 таблица + lease warning + связи с ботами/нодой.
 * Только GET; клик по строке → /bots?runtime_id=, по node → /nodes/:id.
 */
export function RuntimesPage() {
  const navigate = useNavigate()
  const { data, error, loading, refresh } = useFetchList(fetchRuntimesSnapshot)
  const runtimes = data?.runtimes ?? []
  const nodesById = data?.nodesById ?? new Map<string, Node>()
  const showInitial = loading && data === null

  return (
    <main>
      <PageToolbar
        title="Runtimes"
        onRefresh={refresh}
        refreshing={loading && data !== null}
      />

      {error ? <ErrorBlock message={error} onRetry={refresh} /> : null}

      {showInitial ? <LoadingBlock /> : null}

      {!showInitial && !error && runtimes.length === 0 ? (
        <EmptyBlock message="Нет runtimes" />
      ) : null}

      {!showInitial && runtimes.length > 0 ? (
        <div className="table-wrap">
          <table className="data-table">
            <thead>
              <tr>
                <th>kind</th>
                <th>name</th>
                <th>node</th>
                <th>desired</th>
                <th>actual</th>
                <th>pid</th>
                <th>lease_owner</th>
                <th>lease_until</th>
                <th>last_error</th>
              </tr>
            </thead>
            <tbody>
              {runtimes.map((rt) => {
                const mismatch = rt.desired_state !== rt.actual_state
                const leaseStale = isLeaseExpired(rt.lease_until)
                const nodeId = rt.assigned_node_id
                const nodeLabel = resolveNodeLabel(nodeId, nodesById)
                const botsHref = botsByRuntimeHref(rt.id)
                const rowClass = [
                  'data-table__row--clickable',
                  mismatch ? 'data-table__row--mismatch' : '',
                  leaseStale ? 'data-table__row--lease-stale' : '',
                ]
                  .filter(Boolean)
                  .join(' ')

                return (
                  <tr
                    key={rt.id}
                    className={rowClass}
                    onClick={() => navigate(botsHref)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault()
                        navigate(botsHref)
                      }
                    }}
                    tabIndex={0}
                    role="link"
                    aria-label={`Runtime ${rt.name}: боты с этим runtime`}
                  >
                    <td className="mono">{rt.kind}</td>
                    <td>
                      <Link
                        to={botsHref}
                        className="table-link"
                        onClick={(e) => e.stopPropagation()}
                        title={rt.id}
                      >
                        {rt.name}
                      </Link>
                    </td>
                    <td className="mono" title={nodeId ?? undefined}>
                      {nodeId ? (
                        <Link
                          to={`/nodes/${nodeId}`}
                          className="table-link"
                          onClick={(e) => e.stopPropagation()}
                        >
                          {nodeLabel}
                        </Link>
                      ) : (
                        '—'
                      )}
                    </td>
                    <td>
                      <StatePill label={rt.desired_state} />
                    </td>
                    <td>
                      <StatePill label={rt.actual_state} />
                    </td>
                    <td className="mono">{rt.pid ?? '—'}</td>
                    <td className="mono" title={rt.lease_owner ?? undefined}>
                      {rt.lease_owner ? shortId(rt.lease_owner) : '—'}
                    </td>
                    <td
                      className={leaseStale ? 'cell-lease-warning' : undefined}
                      title={
                        rt.lease_until
                          ? leaseStale
                            ? `Lease просрочен: ${formatAbsoluteShort(rt.lease_until)}`
                            : formatAbsoluteShort(rt.lease_until)
                          : undefined
                      }
                    >
                      {rt.lease_until ? (
                        <>
                          <time dateTime={rt.lease_until}>
                            {formatAbsoluteShort(rt.lease_until)}
                          </time>
                          {leaseStale ? (
                            <span className="lease-stale-badge" role="status">
                              просрочен
                            </span>
                          ) : null}
                        </>
                      ) : (
                        '—'
                      )}
                    </td>
                    <td className="cell-error" title={rt.last_error ?? undefined}>
                      {rt.last_error ?? '—'}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      ) : null}
    </main>
  )
}

function resolveNodeLabel(
  nodeId: string | null,
  nodesById: Map<string, Node>,
): string {
  if (!nodeId) return '—'
  const node = nodesById.get(nodeId)
  if (node) return node.hostname
  return shortId(nodeId)
}
