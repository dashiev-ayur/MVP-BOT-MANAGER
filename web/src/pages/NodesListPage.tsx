import { Link, useNavigate } from 'react-router'
import { listNodes } from '../api/lists'
import { EmptyBlock, ErrorBlock, LoadingBlock, PageToolbar } from '../layout/PageStates'
import { StatePill } from '../layout/StatePill'
import { formatAbsoluteShort, formatRelativeRu } from '../lib/formatTime'
import { shortId } from '../lib/shortId'
import { useFetchList } from '../lib/useFetchList'

/**
 * Список нод (/nodes) — read-only таблица.
 * Клик по строке → /nodes/:id (карточка UI-6.1).
 */
export function NodesListPage() {
  const navigate = useNavigate()
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
                <tr
                  key={node.id}
                  className="data-table__row--clickable"
                  onClick={() => navigate(`/nodes/${node.id}`)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault()
                      navigate(`/nodes/${node.id}`)
                    }
                  }}
                  tabIndex={0}
                  role="link"
                  aria-label={`Нода ${node.hostname}`}
                >
                  <td>
                    <Link
                      to={`/nodes/${node.id}`}
                      className="table-link mono"
                      title={node.id}
                      onClick={(e) => e.stopPropagation()}
                    >
                      {shortId(node.id)}
                    </Link>
                  </td>
                  <td>
                    <Link
                      to={`/nodes/${node.id}`}
                      className="table-link mono"
                      onClick={(e) => e.stopPropagation()}
                    >
                      {node.hostname}
                    </Link>
                  </td>
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
