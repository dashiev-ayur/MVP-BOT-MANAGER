// Package config — конфигурация приложения из переменных окружения.
//
// На этапе Phase 0.2 читаются NODE_ID и STORE (по умолчанию memory).
// Неизвестный STORE возвращает понятную ошибку. DATABASE_URL зарезервирован
// под PostgreSQL (Phase PG) и не требует реального Postgres сейчас.
//
// Файл .env сам по себе не подхватывается: см. .env.example и корневой README
// (export / set -a; source .env).
package config
