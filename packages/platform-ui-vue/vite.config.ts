import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import { fileURLToPath, URL } from 'node:url';

export default defineConfig({
  plugins: [vue()],
  appType: 'spa',
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    port: 4200,
    proxy: {
      // API calls - forward to backend as-is (backend paths include /api)
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/bff': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      // Direct auth endpoints (for redirects from OIDC callback etc.)
      '/auth/login': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/auth/logout': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/auth/me': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/auth/check-domain': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/auth/oidc': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/auth/tenant': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/oauth': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/.well-known': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/q': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
});
