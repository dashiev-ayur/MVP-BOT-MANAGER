/// <reference types="vite/client" />

/** Переменные окружения Vite, доступные в клиенте (префикс VITE_). */
interface ImportMetaEnv {
  /** Базовый URL control-api (например http://127.0.0.1:8080). */
  readonly VITE_CONTROL_API_URL?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
