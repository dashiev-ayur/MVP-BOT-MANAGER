// Package config — конфигурация приложения из переменных окружения.
//
// Читает:
//   - NODE_ID (обязателен), STORE (по умолчанию postgres);
//   - DATABASE_URL — DSN PostgreSQL при STORE=postgres;
//   - MEMORY_STORE_PATH — общий JSON для agent/ctl при STORE=memory;
//   - RECONCILE_INTERVAL, HEARTBEAT_INTERVAL, SHUTDOWN_GRACE;
//   - PUBLIC_URL (опционально, launch contract).
//
// Неизвестный STORE возвращает понятную ошибку.
// Файл .env сам по себе не подхватывается: см. .env.example и корневой README.
package config
