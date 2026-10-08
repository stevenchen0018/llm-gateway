import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// In dev, proxy API calls to the gateway so the console runs same-origin.
const target = process.env.GATEWAY_URL || 'http://localhost:8080'

export default defineConfig({
  plugins: [vue()],
  build: {
    rollupOptions: {
      output: {
        manualChunks: (id) => {
          if (id.includes('node_modules/echarts') || id.includes('node_modules/zrender')) return 'echarts'
          if (id.includes('node_modules/element-plus') || id.includes('@element-plus')) return 'element-plus'
          if (id.includes('node_modules')) return 'vendor'
        },
      },
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/admin': target,
      '/v1': target,
      '/healthz': target,
      '/readyz': target,
      '/openapi.json': target,
    },
  },
})
