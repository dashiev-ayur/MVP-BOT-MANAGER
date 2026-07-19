import { listNodes, listRuntimes } from '../api/lists'
import type { Node, Runtime } from '../api/types'
import { EmptyBlock, ErrorBlock, LoadingBlock, PageToolbar } from '../layout/PageStates'
import { StatePill } from '../layout/StatePill'
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

/**
 * Минимальный read-only список runtimes (полный экран — UI-6).
 * Таблица: kind, name, node, desired, actual, pid, last_error.
 */
export function RuntimesPage() {
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
                <th>last_error</th>
              </tr>
            </thead>
            <tbody>
              {runtimes.map((rt) => {
                const mismatch = rt.desired_state !== rt.actual_state
                const nodeLabel = resolveNodeLabel(rt.assigned_node_id, nodesById)
                return (
                  <tr
                    key={rt.id}
                    className={mismatch ? 'data-table__row--mismatch' : undefined}
                  >
                    <td className="mono">{rt.kind}</td>
                    <td>{rt.name}</td>
                    <td className="mono" title={rt.assigned_node_id ?? undefined}>
                      {nodeLabel}
                    </td>
                    <td>
                      <StatePill label={rt.desired_state} />
                    </td>
                    <td>
                      <StatePill label={rt.actual_state} />
                    </td>
                    <td className="mono">{rt.pid ?? '—'}</td>
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
