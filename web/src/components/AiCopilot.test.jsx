import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import AiCopilot from './AiCopilot';
import { HARNESS_BENCHMARKS } from './copilotHarness';

// The copilot streams through /v1/chat/completions. Its error handling used to be
// a regex over the failure text, so any gateway error that did not literally say
// "stream error" (quota, rate limit, auth) was swallowed: the assistant bubble
// stayed empty and nothing was reported.

function baseProps(overrides = {}) {
  return {
    isOpen: true,
    onClose: () => {},
    onToggle: () => {},
    setCurrentTab: () => {},
    setViewMode: () => {},
    models: ['deepseek-v4-flash'],
    channels: [],
    keys: [{ id: 1, key: 'sk-test', status: 'active' }],
    logs: [],
    modelRoutes: [],
    prices: [],
    stats: {},
    lang: 'en',
    t: {},
    showToast: vi.fn(),
    adminUser: { role: 'admin' },
    adminFetch: vi.fn(),
    ...overrides,
  };
}

// A response whose body yields the given SSE chunks, recording release calls so
// the test can prove the reader is not leaked when an error event unwinds.
function sseResponse(chunks) {
  const released = { cancel: 0, release: 0 };
  let index = 0;
  const response = {
    ok: true,
    status: 200,
    body: {
      getReader: () => ({
        read: async () => (index < chunks.length
          ? { value: new TextEncoder().encode(chunks[index++]), done: false }
          : { value: undefined, done: true }),
        cancel: async () => { released.cancel += 1; },
        releaseLock: () => { released.release += 1; },
      }),
    },
  };
  return { response, released };
}

function sendFirstBenchmark() {
  const benchmark = screen.getAllByRole('button').find((button) => button.getAttribute('aria-pressed') === 'false');
  fireEvent.click(benchmark);
}

describe('AiCopilot error surfacing', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
    // jsdom does not implement scrollIntoView; the copilot scrolls to the newest
    // message on every render.
    Element.prototype.scrollIntoView = vi.fn();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    delete Element.prototype.scrollIntoView;
  });

  it('reports a structured stream error instead of leaving an empty answer', async () => {
    const { response, released } = sseResponse([
      'data: {"error":{"message":"insufficient quota for this key","type":"insufficient_quota"}}\n\n',
    ]);
    globalThis.fetch.mockResolvedValue(response);
    const showToast = vi.fn();
    render(<AiCopilot {...baseProps({ showToast })} />);

    sendFirstBenchmark();

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toContain('insufficient quota for this key');
    expect(showToast).toHaveBeenCalledWith(expect.stringContaining('insufficient quota for this key'), 'error');
    // The stream reader is released even though the error unwound the loop.
    await waitFor(() => expect(released.cancel).toBeGreaterThan(0));
    expect(released.release).toBeGreaterThan(0);
  });

  it('reports the gateway message of a non-2xx response and labels the local fallback', async () => {
    globalThis.fetch.mockResolvedValue({
      ok: false,
      status: 429,
      text: async () => JSON.stringify({ error: { message: 'rate limit exceeded, retry later' } }),
    });
    const showToast = vi.fn();
    render(<AiCopilot {...baseProps({ showToast })} />);

    sendFirstBenchmark();

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toContain('rate limit exceeded, retry later');
    expect(showToast).toHaveBeenCalledWith(expect.stringContaining('rate limit exceeded, retry later'), 'error');
    // No live text arrived, so the offline template is shown — and must say so.
    expect(await screen.findByText(/Offline template/i)).toBeInTheDocument();
  });

  it('keeps the partial answer visible and marks it failed when the stream breaks mid-answer', async () => {
    const { response } = sseResponse([
      'data: {"choices":[{"delta":{"content":"partial answer"}}]}\n\n',
      'data: {"error":{"message":"upstream aborted the stream"}}\n\n',
    ]);
    globalThis.fetch.mockResolvedValue(response);
    render(<AiCopilot {...baseProps()} />);

    sendFirstBenchmark();

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toContain('upstream aborted the stream');
    // Partial content is preserved, and it is not presented as an offline template.
    expect(screen.getByText(/partial answer/)).toBeInTheDocument();
    expect(screen.queryByText(/Offline template/i)).toBeNull();
  });
});

// Keep the benchmark list reference alive so a future rename does not silently
// turn "sendFirstBenchmark" into a no-op.
describe('AiCopilot benchmark catalogue', () => {
  it('exposes at least one runnable benchmark', () => {
    expect(HARNESS_BENCHMARKS.length).toBeGreaterThan(0);
  });
});
