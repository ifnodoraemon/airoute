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
    // The login session authorizes the selected key record.
    keys: [{ id: 1, key_id: 'ak_1f0013dea24034ac0001', status: 'active' }],
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


describe('AiCopilot session-based key selection', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
    Element.prototype.scrollIntoView = vi.fn();
  });
  afterEach(() => vi.unstubAllGlobals());

  const twoKeys = () => ([
    { id: 1, key_id: 'ak_1111aaaa2222bbbb3333', tenant_id: 'acme', allowed_models: ['chat-a'], status: 'active' },
    { id: 2, key_id: 'ak_4444cccc5555dddd6666', tenant_id: 'beta', allowed_models: ['chat-b'], status: 'active' },
  ]);

  it('selects a listed key and sends immediately without asking for its secret', async () => {
    globalThis.fetch.mockResolvedValue(sseResponse(['data: {"choices":[{"delta":{"content":"ok"}}]}\n\n']).response);
    const { container } = render(<AiCopilot {...baseProps()} />);
    expect(container.querySelector('input[type="password"]')).toBeNull();
    expect(screen.queryByRole('option', { name: /unauthenticated|Paste a key/i })).not.toBeInTheDocument();
    sendFirstBenchmark();
    await waitFor(() => expect(fetch).toHaveBeenCalledOnce());
    const [url, options] = fetch.mock.calls[0];
    expect(url).toBe('/api/v1/playground/v1/chat/completions');
    expect(options.headers['X-Airoute-Key-ID']).toBe('1');
    expect(options.headers.Authorization).toBeUndefined();
    expect(options.credentials).toBe('same-origin');
  });

  it('switches model access and the selected billing key together', async () => {
    globalThis.fetch.mockResolvedValue(sseResponse(['data: {"choices":[{"delta":{"content":"ok"}}]}\n\n']).response);
    render(<AiCopilot {...baseProps({ keys: twoKeys(), models: ['chat-a', 'chat-b'] })} />);
    expect(screen.getByLabelText('Copilot model')).toHaveValue('chat-a');
    expect(screen.queryByRole('option', { name: 'chat-b' })).not.toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('Select API key'), { target: { value: '2' } });
    expect(screen.getByLabelText('Copilot model')).toHaveValue('chat-b');
    sendFirstBenchmark();
    await waitFor(() => expect(fetch).toHaveBeenCalledOnce());
    expect(fetch.mock.calls[0][1].headers['X-Airoute-Key-ID']).toBe('2');
    expect(JSON.parse(fetch.mock.calls[0][1].body).model).toBe('chat-b');
  });

  it('blocks requests when there is no active key and opens key creation', () => {
    const showToast = vi.fn(), onCreateKey = vi.fn(), setCurrentTab = vi.fn();
    render(<AiCopilot {...baseProps({ keys: [], showToast, onCreateKey, setCurrentTab })} />);
    sendFirstBenchmark();
    expect(fetch).not.toHaveBeenCalled();
    expect(showToast).toHaveBeenCalledWith(expect.stringContaining('Create and select'), 'warning');
    fireEvent.click(screen.getByRole('button', { name: 'Create API key' }));
    expect(setCurrentTab).toHaveBeenCalledWith('keys');
    expect(onCreateKey).toHaveBeenCalledOnce();
  });

  it('adopts asynchronously loaded keys and drops revoked selections', () => {
    const { rerender } = render(<AiCopilot {...baseProps({ keys: [] })} />);
    expect(screen.getByLabelText('Select API key')).toBeDisabled();
    rerender(<AiCopilot {...baseProps({ keys: twoKeys() })} />);
    expect(screen.getByLabelText('Select API key')).toHaveValue('1');
    fireEvent.change(screen.getByLabelText('Select API key'), { target: { value: '2' } });
    rerender(<AiCopilot {...baseProps({ keys: twoKeys().map(k => k.id === 2 ? { ...k, status: 'disabled' } : k) })} />);
    expect(screen.getByLabelText('Select API key')).toHaveValue('1');
  });

  it('names a revoked selection and offers an actionable remedy without requesting plaintext', async () => {
    globalThis.fetch.mockResolvedValue({ ok: false, status: 403, text: async () => JSON.stringify({ error: { code: 'api_key_revoked', message: 'The API key has been revoked or disabled.' } }) });
    const showToast = vi.fn();
    render(<AiCopilot {...baseProps({ showToast })} />);
    sendFirstBenchmark();
    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toMatch(/deleted or disabled/);
    expect(alert.textContent).toMatch(/API Keys page/);
    expect(alert.textContent).not.toMatch(/paste|plaintext/i);
  });
});
