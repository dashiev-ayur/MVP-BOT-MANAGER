import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'

// Dev-proxy: /v1 и /healthz → control-api (см. docs/frontend.md §3.3).
// Цель берётся из VITE_CONTROL_API_URL или по умолчанию 127.0.0.1:8080.
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const apiTarget = env.VITE_CONTROL_API_URL || 'http://127.0.0.1:8080'

  return {
    plugins: [react()],
    server: {
      proxy: {
        '/v1': {
          target: apiTarget,
          changeOrigin: true,
        },
        '/healthz': {
          target: apiTarget,
          changeOrigin: true,
        },
      },
    },
  }
})
