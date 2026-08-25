import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Dev-only proxy so the browser sees the SPA and the API as the same
// origin (required for the __Host- session cookie / SameSite=Strict model
// described in docs/SECURITY.md §9 to work during local development).
// In production the built assets are served behind the same reverse proxy
// as wr-core's admin listener; see docs/DEPLOYMENT.md.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:9443',
        changeOrigin: true,
        ws: true,
      },
    },
  },
})
