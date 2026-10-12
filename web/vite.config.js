import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

const proxyTarget = process.env.VITE_PROXY_TARGET || process.env.PUBLIC_URL || `http://127.0.0.1:${process.env.GATEWAY_PORT || '8080'}`;

export default defineConfig({
  plugins: [react()],
  base: './',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks: {
          vendor: ['react', 'react-dom'],
          icons: ['lucide-react'],
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
