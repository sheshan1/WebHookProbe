import { defineConfig } from 'vite';

export default defineConfig({
  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      '/api/': { target: 'http://127.0.0.1:8081', rewrite: p => p.replace(/^\/api/, '') },
      '/demo/': { target: 'http://127.0.0.1:8080', rewrite: p => p.replace(/^\/demo/, '') },
    },
  },
});
