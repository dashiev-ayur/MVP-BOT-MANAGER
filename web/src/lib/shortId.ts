/** Короткий id для таблиц; полный — в title. */
export function shortId(id: string, keep = 8): string {
  if (id.length <= keep) return id
  return id.slice(0, keep)
}
