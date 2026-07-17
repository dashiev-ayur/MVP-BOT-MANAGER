// Package migrations — SQL-файлы goose и embed.FS для cmd/migrate.
//
// Файлы лежат рядом (*.sql); go:embed не допускает «..» в путях,
// поэтому embed живёт в этом каталоге, а не в cmd/migrate.
package migrations

import "embed"

// FS — все *.sql миграции для goose.SetBaseFS / goose.Up.
//
//go:embed *.sql
var FS embed.FS
