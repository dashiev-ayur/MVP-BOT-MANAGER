import { listNodes } from '../api/lists'
import { EmptyBlock, ErrorBlock, LoadingBlock, PageToolbar } from '../layout/PageStates'
import { StatePill } from '../layout/StatePill'
import { formatAbsoluteShort, formatRelativeRu } from '../lib/formatTime'
import { shortId } from '../lib/shortId'
import { useFetchList } from '../lib/useFetchList'

/**
 * Список нод (/nodes) — read-only таблица UI-2.
 * Колонки: id, hostname, status, last_seen_at, agent_version.
 */
export function NodesListPage() {
  const { data, error, loading, refresh } = useFetchList(listNodes)
  const nodes = data ?? []
  const showInitial = loading && data === null

  return (
    <main>
      <PageToolbar
        title="Ноды"
        onRefresh={refresh}
        refreshing={loading && data !== null}
      />

      {error ? <ErrorBlock message={error} onRetry={refresh} /> : null}

      {showInitial ? <LoadingBlock /> : null}

      {!showInitial && !error && nodes.length === 0 ? (
        <EmptyBlock message="Нет нод" />
      ) : null}

      {!showInitial && nodes.length > 0 ? (
        <div className="table-wrap">
          <table className="data-table">
            <thead>
              <tr>
                <th>id</th>
                <th>hostname</th>
                <th>status</th>
                <th>last_seen_at</th>
                <th>agent_version</th>
              </tr>
            </thead>
            <tbody>
              {nodes.map((node) => (
                <tr key={node.id}>
                  <td>
                    <code className="mono" title={node.id}>
                      {shortId(node.id)}
                    </code>
                  </td>
                  <td className="mono">{node.hostname}</td>
                  <td>
                    <StatePill label={node.status} />
                  </td>
                  <td title={formatAbsoluteShort(node.last_seen_at)}>
                    {formatRelativeRu(node.last_seen_at)}
                  </td>
                  <td className="mono">{node.agent_version ?? '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </main>
  )
}
