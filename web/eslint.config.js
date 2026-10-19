// Minimal, high-signal lint configuration.
//
// The console had two shipped ReferenceErrors (an unimported component and a
// call to a function that was never defined). Vite/esbuild cannot detect those:
// an undefined identifier is only a runtime error in JavaScript. `no-undef` is
// therefore the primary rule here — it fails the build before such a bug can
// reach a browser.
import js from '@eslint/js';
import globals from 'globals';
import reactHooks from 'eslint-plugin-react-hooks';

export default [
  {
    ignores: ['dist/**', 'node_modules/**'],
  },
  js.configs.recommended,
  {
    files: ['src/**/*.{js,jsx}'],
    languageOptions: {
      ecmaVersion: 2023,
      sourceType: 'module',
      parserOptions: { ecmaFeatures: { jsx: true } },
      globals: {
        ...globals.browser,
        ...globals.es2021,
      },
    },
    plugins: { 'react-hooks': reactHooks },
    rules: {
      // Keep the hook dependency checks the codebase already relies on via
      // inline eslint-disable comments; without the plugin registered those
      // comments are themselves lint errors.
      'react-hooks/rules-of-hooks': 'error',
      'react-hooks/exhaustive-deps': 'warn',
      // The rules that would have caught the shipped ReferenceErrors.
      'no-undef': 'error',
      'no-unused-vars': ['warn', { args: 'none', varsIgnorePattern: '^_', caughtErrors: 'none' }],
      // Keep the signal clean: these are stylistic and noisy in a large JSX file.
      'no-empty': ['error', { allowEmptyCatch: true }],
    },
  },
];
