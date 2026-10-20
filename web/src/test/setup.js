// Shared setup for the frontend test suite.

import { afterEach } from 'vitest';
import { cleanup } from '@testing-library/react';
import '@testing-library/jest-dom/vitest';

// React Testing Library unmounts whatever a test rendered between tests — but only
// if it can see a global afterEach to hook into. This project keeps vitest's
// `globals` off on purpose (an injected global is an undefined identifier to
// ESLint's no-undef rule, which is the rule used to catch shipped
// ReferenceErrors), so that hook was never registered and every component test
// leaked its DOM into the next one.
//
// The symptom is confusing rather than obvious: a query for one element finds the
// previous test's copy as well, or a checkbox count silently multiplies.
afterEach(() => {
  cleanup();
});
