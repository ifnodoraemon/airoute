import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

// Test-only configuration, deliberately separate from vite.config.js so the
// production build (and its Tailwind plugin) is unaffected by the test harness.
//
// `globals` stays off: every test imports describe/it/expect/vi from 'vitest'.
// Relying on injected globals would make them undefined identifiers to ESLint's
// no-undef rule, which is the rule this project uses to catch shipped
// ReferenceErrors.
export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.{js,jsx}'],
    setupFiles: ['./src/test/setup.js'],
  },
});
