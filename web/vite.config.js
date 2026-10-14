import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

const proxyTarget = process.env.VITE_PROXY_TARGET || process.env.PUBLIC_URL || `http://127.0.0.1:${process.env.GATEWAY_PORT || '8080'}`;

export default defineConfig({
  plugins: [react(), tailwindcss()],
  base: './',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        // Rolldown (Vite 8) requires manualChunks to be a function.
        manualChunks(id) {
          if (id.includes('node_modules')) {
            if (id.includes('lucide-react')) return 'icons';
            if (id.includes('/react-dom/') || id.includes('/react/') || id.includes('/scheduler/')) return 'vendor';
          }
        },
      },
    },
  },
  server: {
    proxy: {
      '/api': proxyTarget,
      '/v1': proxyTarget,
    },
  },
});
