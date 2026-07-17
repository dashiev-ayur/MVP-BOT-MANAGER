#!/bin/bash
# Выполняется только при первой инициализации тома Postgres
# (/docker-entrypoint-initdb.d). Создаёт e2e-БД рядом с POSTGRES_DB.
#
# Имя e2e-БД: POSTGRES_DB_E2E из окружения контейнера (compose передаёт
# из .env); fallback mvp_manager_e2e — совместимость со старыми томами /
# запуском без переменной.
set -euo pipefail

E2E_DB="${POSTGRES_DB_E2E:-mvp_manager_e2e}"

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
	SELECT 'CREATE DATABASE ${E2E_DB}'
	WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '${E2E_DB}')\gexec
EOSQL

echo "init-databases: ${POSTGRES_DB} (dev) + ${E2E_DB} готовы"
