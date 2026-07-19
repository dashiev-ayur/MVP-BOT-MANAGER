import { useMemo, type ReactNode } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { listBots, listNodes, listRuntimes } from '../api/lists'
import type { Bot, Node, Runtime } from '../api/types'
import { EmptyBlock, ErrorBlock, LoadingBlock, PageToolbar } from '../layout/PageStates'
import { StatePill } from '../layout/StatePill'
import { formatAbsoluteShort, formatRelativeRu, isLeaseExpired } from '../lib/formatTime'
import { useFetchList } from '../lib/useFetchList'

type NodeDetailSnapshot = {
  nodes: Node[]
  bots: Bot[]
  runtimes: Runtime[]
}

async function fetchNodeDetailSnapshot(signal: AbortSignal): Promise<NodeDetailSnapshot> {
  // Отдельного GET /v1/nodes/{id} нет — собираем из списков (как карточка бота).
  const [nodes, bots, runtimes] = await Promise.all([
    listNodes(signal),
    listBots(signal),
    listRuntimes(signal),
  ])
  return { nodes, bots, runtimes }
}

/** Пустое значение в паспорте. */
function dash(value: string | number | null | undefined): string {
  if (value === null || value === undefined || value === '') {
    return '—'
  }
  return String(value)
}

/** meta — компактный JSON или прочерк (детальный просмотр — P2). */
function formatMeta(meta: Record<string, unknown> | null): string {
  if (meta === null || Object.keys(meta).length === 0) {
    return '—'
  }
  try {
    return JSON.stringify(meta)
  } catch {
    return '—'
  }
}

/**
 * Карточка ноды (/nodes/:id) — паспорт + боты/runtimes на ноде (UI-6.1).
 * Фильтрация client-side по assigned_node_id; только GET.
 *
 * key={id} снаружи: useFetchList не зависит от id, при смене :id нужен remount.
 */
export function NodeDetailPage() {
  const { id = '' } = useParams<{ id: string }>()
  return <NodeDetailBody key={id} id={id} />
}

function NodeDetailBody({ id }: { id: string }) {
  const { data, error, loading, refresh } = useFetchList(fetchNodeDetailSnapshot)
  const showInitial = loading && data === null

  const node = data?.nodes.find((n) => n.id === id) ?? null

  // Боты и runtimes, назначенные на эту ноду.
  const botsOnNode = useMemo(
    () => (data ? data.bots.filter((b) => b.assigned_node_id === id) : []),
    [data, id],
  )
  const runtimesOnNode = useMemo(
    () => (data ? data.runtimes.filter((r) => r.assigned_node_id === id) : []),
    [data, id],
  )

  const title = node ? node.hostname : 'Нода'

  return (
    <main>
      <PageToolbar
        title={title}
        onRefresh={refresh}
        refreshing={loading && data !== null}
        actions={
          <Link to="/nodes" className="btn btn--secondary">
            К списку
          </Link>
        }
      />

      {error ? <ErrorBlock message={error} onRetry={refresh} /> : null}

      {showInitial ? <LoadingBlock /> : null}

      {!showInitial && !error && !node ? (
        <EmptyBlock
          message="Нода не найдена"
          action={
            <Link to="/nodes" className="btn btn--secondary">
              К списку нод
            </Link>
          }
        />
      ) : null}

      {!showInitial && node ? (
        <>
          <NodePassport node={node} />

          <BotsOnNodeSection bots={botsOnNode} />
          <RuntimesOnNodeSection runtimes={runtimesOnNode} />
        </>
      ) : null}
    </main>
  )
}

type NodePassportProps = {
  node: Node
}

/** Паспорт ноды: поля §4.1 + timestamps. */
function NodePassport({ node }: NodePassportProps) {
  return (
    <section className="bot-passport" aria-label="Паспорт ноды">
      <div className="bot-passport__states bot-passport__states--large">
        <div className="bot-passport__state">
          <span className="bot-passport__label">status</span>
          <StatePill label={node.status} />
        </div>
      </div>

      <dl className="bot-passport__fields">
        <Field label="id">
          <span className="mono">{node.id}</span>
        </Field>
        <Field label="hostname">
          <span className="mono">{node.hostname}</span>
        </Field>
        <Field label="last_seen_at">
          <time dateTime={node.last_seen_at} title={formatAbsoluteShort(node.last_seen_at)}>
            {formatRelativeRu(node.last_seen_at)}
          </time>
        </Field>
        <Field label="agent_version">
          <span className="mono">{dash(node.agent_version)}</span>
        </Field>
        <Field label="meta">
          <span className="mono bot-passport__scenario">{formatMeta(node.meta)}</span>
        </Field>
        <Field label="created_at">
          <time dateTime={node.created_at} title={formatAbsoluteShort(node.created_at)}>
            {formatRelativeRu(node.created_at)}
          </time>
        </Field>
        <Field label="updated_at">
          <time dateTime={node.updated_at} title={formatAbsoluteShort(node.updated_at)}>
            {formatRelativeRu(node.updated_at)}
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

type BotsOnNodeSectionProps = {
  bots: Bot[]
}

/** Секция «Боты на ноде» — client filter по assigned_node_id. */
function BotsOnNodeSection({ bots }: BotsOnNodeSectionProps) {
  const navigate = useNavigate()

  return (
    <section className="node-section" aria-label="Боты на ноде">
      <h2 className="node-section__title">Боты на ноде</h2>

      {bots.length === 0 ? (
        <EmptyBlock message="На этой ноде нет ботов" />
      ) : (
        <div className="table-wrap">
          <table className="data-table">
            <thead>
              <tr>
                <th>name</th>
                <th>type</th>
                <th>channel</th>
                <th>desired</th>
                <th>actual</th>
                <th>last_error</th>
              </tr>
            </thead>
            <tbody>
              {bots.map((bot) => {
                const mismatch = bot.desired_state !== bot.actual_state
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
                    <td>
                      <StatePill label={bot.desired_state} />
                    </td>
                    <td>
                      <StatePill label={bot.actual_state} />
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
      )}
    </section>
  )
}

type RuntimesOnNodeSectionProps = {
  runtimes: Runtime[]
}

/** Секция «Runtimes на ноде» — client filter по assigned_node_id. */
function RuntimesOnNodeSection({ runtimes }: RuntimesOnNodeSectionProps) {
  const navigate = useNavigate()

  return (
    <section className="node-section" aria-label="Runtimes на ноде">
      <h2 className="node-section__title">Runtimes на ноде</h2>

      {runtimes.length === 0 ? (
        <EmptyBlock message="На этой ноде нет runtimes" />
      ) : (
        <div className="table-wrap">
          <table className="data-table">
            <thead>
              <tr>
                <th>kind</th>
                <th>name</th>
                <th>desired</th>
                <th>actual</th>
                <th>pid</th>
                <th>lease_until</th>
                <th>last_error</th>
              </tr>
            </thead>
            <tbody>
              {runtimes.map((rt) => {
                const mismatch = rt.desired_state !== rt.actual_state
                const leaseStale = isLeaseExpired(rt.lease_until)
                const botsHref = `/bots?runtime_id=${encodeURIComponent(rt.id)}`
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
                    aria-label={`Runtime ${rt.name}`}
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
                    <td>
                      <StatePill label={rt.desired_state} />
                    </td>
                    <td>
                      <StatePill label={rt.actual_state} />
                    </td>
                    <td className="mono">{rt.pid ?? '—'}</td>
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
      )}

      {runtimes.length > 0 ? (
        <p className="page-meta">
          Runtimes: {runtimes.length}
          {runtimes.some((r) => isLeaseExpired(r.lease_until))
            ? ' · есть просроченные lease'
            : ''}
        </p>
      ) : null}
    </section>
  )
}
