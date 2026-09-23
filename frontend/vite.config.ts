import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// Goサーバーのポート。ローカルの.envでPORTを変えている場合はBACKEND_PORTで上書きする。
const backendPort = process.env.BACKEND_PORT || process.env.PORT || '8080'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5174,
    proxy: {
      '/api': {
        target: `http://localhost:${backendPort}`,
        changeOrigin: true,
      },
    },
  },
})
