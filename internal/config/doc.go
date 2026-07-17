// Package config — конфигурация приложения из переменных окружения.
//
// Phase 1 читает:
//   - NODE_ID (обязателен), STORE (по умолчанию memory);
//   - MEMORY_STORE_PATH — общий JSON для agent/ctl при STORE=memory;
//   - RECONCILE_INTERVAL, HEARTBEAT_INTERVAL, SHUTDOWN_GRACE;
//   - PUBLIC_URL (опционально, launch contract);
//   - DATABASE_URL зарезервирован под Phase PG.
//
// Неизвестный STORE возвращает понятную ошибку.
// Файл .env сам по себе не подхватывается: см. .env.example и корневой README.
package config
