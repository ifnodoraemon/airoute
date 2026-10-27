import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import PlaygroundView from './PlaygroundView';

// The key list only returns a public key ID (credentials are stored as digests),
// so the playground must never use a listed key as a credential. The plaintext a
// caller pastes for the session is what actually signs the request; everything
// below pins that boundary.
vi.mock('../utils/clipboard', () => ({ copyToClipboard: vi.fn(async () => true) }));

// Regression guard for a crash that reached the browser:
//
//   Uncaught ReferenceError: Cannot access 'j' before initialization
//     at PlaygroundView-<hash>.js:1:625
//
// The off-peak footer derived `activeChatModel` from `playModel`, but the block
// sat above `const [playModel, setPlayModel] = useState(...)`. Reading a `const`
// binding inside its temporal dead zone throws, and because the block runs on
// every render, the whole playground route was a blank page. Nothing in the suite
// rendered this component, and `no-undef` cannot see the problem: `playModel` is
// declared, just too late.
//
// Mounting the view is therefore the actual assertion — the broken version throws
// before anything reaches the DOM.

const baseProps = {
  keys: [{ id: 1, key_id: 'ak_1f0013dea24034ac0001', allowed_models: ['*'] }],
  models: ['gpt-4o', 'claude-3-5-sonnet'],
  modelRoutes: [],
  prices: [],
  lang: 'zh',
};

describe('PlaygroundView', () => {
  it('mounts without a temporal-dead-zone ReferenceError', () => {
    render(<PlaygroundView {...baseProps} />);

    // The footer renders the "no discount" branch with empty pricing data.
    expect(screen.getAllByText('该模型未配置闲时折扣').length).toBeGreaterThan(0);
  });

  it('resolves the off-peak discount of the selected model', () => {
    render(
      <PlaygroundView
        {...baseProps}
        initialModel="gpt-4o"
        prices={[
          { model: 'gpt-4o', group_name: 'default', off_peak_enabled: true, off_peak_discount: 0.5 },
        ]}
      />
    );

    expect(screen.getAllByText('闲时折扣已配置 (5 折, 按时段生效)').length).toBeGreaterThan(0);
  });

  it('starts on the model passed through initialModel', () => {
    render(<PlaygroundView {...baseProps} initialModel="claude-3-5-sonnet" />);

    expect(screen.getByDisplayValue('claude-3-5-sonnet')).toBeInTheDocument();
  });
});


// ---------------------------------------------------------------------------
// Identifier / mask boundary
// ---------------------------------------------------------------------------

const MASK = '3a4b5c6d***9f8e';
const PLAINTEXT = 'sk-airoute-0123456789abcdef';

const maskedKeyProps = {
  keys: [{ id: 11, key_id: 'ak_11f0013dea24034ac011', tenant_id: 'acme', allowed_models: ['*'], status: 'active' }],
  models: ['gpt-4o'],
  modelRoutes: [],
  prices: [],
  lang: 'zh',
};

function renderMasked(props = {}) {
  const showToast = vi.fn();
  const utils = render(<PlaygroundView {...maskedKeyProps} showToast={showToast} {...props} />);
  return { ...utils, showToast };
}

function typeChatMessage(text = 'hello') {
  fireEvent.change(screen.getByPlaceholderText(/输入对话 Prompt/), { target: { value: text } });
}

describe('PlaygroundView masked API keys', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: false,
      status: 401,
      json: async () => ({ error: { message: 'invalid api key' } }),
    })));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('authenticates with the pasted plaintext, never with a listed identifier', async () => {
    renderMasked();

    fireEvent.change(screen.getByLabelText('会话 API Key 明文'), { target: { value: PLAINTEXT } });
    typeChatMessage();
    fireEvent.click(screen.getByRole('button', { name: /发送请求/ }));

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalled());
    const [url, options] = globalThis.fetch.mock.calls[0];
    expect(url).toBe('/v1/chat/completions');
    expect(options.headers.Authorization).toBe(`Bearer ${PLAINTEXT}`);
    expect(JSON.stringify(options.headers)).not.toContain('***');
    expect(JSON.stringify(options.headers)).not.toContain(MASK);
  });

  it('refuses to call without a pasted plaintext key', async () => {
    const { showToast } = renderMasked();

    typeChatMessage();
    fireEvent.click(screen.getByRole('button', { name: /发送请求/ }));

    expect(globalThis.fetch).not.toHaveBeenCalled();
    expect(showToast).toHaveBeenCalledWith(expect.stringContaining('明文'), 'warning');
  });

  it('rejects a pasted key ID and names it', async () => {
    const { showToast } = renderMasked();

    // The copy button on the list yields this value; it can never authenticate.
    fireEvent.change(screen.getByLabelText('会话 API Key 明文'), { target: { value: 'ak_11f0013dea24034ac011' } });
    typeChatMessage();
    fireEvent.click(screen.getByRole('button', { name: /发送请求/ }));

    expect(globalThis.fetch).not.toHaveBeenCalled();
    expect(showToast).toHaveBeenCalledWith(expect.stringContaining('密钥 ID'), 'warning');
  });

  it('rejects a pasted value that is still a mask', async () => {
    const { showToast } = renderMasked();

    fireEvent.change(screen.getByLabelText('会话 API Key 明文'), { target: { value: MASK } });
    typeChatMessage();
    fireEvent.click(screen.getByRole('button', { name: /发送请求/ }));

    expect(globalThis.fetch).not.toHaveBeenCalled();
    expect(showToast).toHaveBeenCalledWith(expect.any(String), 'warning');
  });

  it('puts a placeholder, not the mask, in the copied cURL when no plaintext exists', async () => {
    const { copyToClipboard } = await import('../utils/clipboard');
    copyToClipboard.mockClear();
    renderMasked();

    fireEvent.click(screen.getByRole('button', { name: /复制 cURL/ }));

    await waitFor(() => expect(copyToClipboard).toHaveBeenCalled());
    const snippet = copyToClipboard.mock.calls[0][0];
    expect(snippet).not.toContain('***');
    expect(snippet).toContain('<YOUR_API_KEY>');
    expect(snippet).toContain('Authorization: Bearer <YOUR_API_KEY>');
  });

  it('puts the pasted plaintext in the copied cURL so it matches the request', async () => {
    const { copyToClipboard } = await import('../utils/clipboard');
    copyToClipboard.mockClear();
    renderMasked();

    fireEvent.change(screen.getByLabelText('会话 API Key 明文'), { target: { value: PLAINTEXT } });
    fireEvent.click(screen.getByRole('button', { name: /复制 cURL/ }));

    await waitFor(() => expect(copyToClipboard).toHaveBeenCalled());
    const snippet = copyToClipboard.mock.calls[0][0];
    expect(snippet).toContain(`Authorization: Bearer ${PLAINTEXT}`);
    expect(snippet).not.toContain('***');
  });

  it('keys the entitlement context off the row id, not the key ID', () => {
    // The selected row's allowed_models still filter the model list; only the
    // credential is pasted separately.
    renderMasked({
      keys: [{ id: 11, key_id: 'ak_11f0013dea24034ac011', allowed_models: ['gpt-4o'], status: 'active' }],
      models: ['gpt-4o', 'claude-3-5-sonnet'],
    });

    expect(screen.getByDisplayValue('gpt-4o')).toBeInTheDocument();
    expect(screen.queryByDisplayValue('claude-3-5-sonnet')).not.toBeInTheDocument();
  });
});

describe('PlaygroundView unauthenticated mode', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: false,
      status: 401,
      json: async () => ({ error: { message: 'invalid api key' } }),
    })));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('keeps the public gateway usable when no key is configured', async () => {
    render(<PlaygroundView {...baseProps} keys={[]} showToast={vi.fn()} />);

    // With no listed key the only choice is to call without a credential.
    const select = screen.getByLabelText(/密钥上下文/);
    expect(Array.from(select.options).map(o => o.value)).toEqual(['__none__']);

    fireEvent.change(screen.getByPlaceholderText(/输入对话 Prompt/), { target: { value: 'hi' } });
    fireEvent.click(screen.getByRole('button', { name: /发送请求/ }));

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalled());
    const [, options] = globalThis.fetch.mock.calls[0];
    expect(options.headers.Authorization).toBeUndefined();
    expect(JSON.stringify(options.headers)).not.toContain('***');
  });
});
