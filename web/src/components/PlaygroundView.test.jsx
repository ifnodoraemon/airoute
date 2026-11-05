import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import PlaygroundView from './PlaygroundView';

// Console calls use the login session and a selected non-secret key record.
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


const listedKeys = [
  { id: 11, key_id: 'ak_11f0013dea24034ac011', tenant_id: 'acme', allowed_models: ['gpt-4o'], status: 'active' },
  { id: 12, key_id: 'ak_12f0013dea24034ac012', tenant_id: 'beta', allowed_models: ['claude-3-5-sonnet'], status: 'active' },
];

function typeChatMessage(text = 'hello') {
  fireEvent.change(screen.getByPlaceholderText(/输入消息/), { target: { value: text } });
}

describe('PlaygroundView session-based key selection', () => {
  beforeEach(() => {
    document.cookie = 'airoute_csrf=console-csrf; path=/';
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, status: 200, json: async () => ({ choices: [{ message: { content: 'answer' } }] }) })));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    document.cookie = 'airoute_csrf=; max-age=0; path=/';
  });

  it('calls immediately with the selected record and login session, without a key secret', async () => {
    const { container } = render(<PlaygroundView {...baseProps} keys={listedKeys} />);
    expect(container.querySelector('input[type="password"]')).toBeNull();
    typeChatMessage();
    fireEvent.click(screen.getByRole('button', { name: /发送请求/ }));
    await waitFor(() => expect(fetch).toHaveBeenCalledOnce());
    const [url, options] = fetch.mock.calls[0];
    expect(url).toBe('/api/v1/playground/v1/chat/completions');
    expect(options.credentials).toBe('same-origin');
    expect(options.headers).toEqual({ 'Content-Type': 'application/json', 'X-Airoute-Key-ID': '11', 'X-CSRF-Token': 'console-csrf' });
    expect(JSON.parse(options.body).model).toBe('gpt-4o');
    expect(JSON.stringify(options)).not.toContain('ak_');
  });

  it('switches both model scope and the billed key together', async () => {
    render(<PlaygroundView {...baseProps} keys={listedKeys} />);
    fireEvent.change(screen.getByLabelText('选择 API 访问密钥'), { target: { value: '12' } });
    expect(screen.getByDisplayValue('claude-3-5-sonnet')).toBeInTheDocument();
    expect(screen.queryByDisplayValue('gpt-4o')).not.toBeInTheDocument();
    typeChatMessage();
    fireEvent.click(screen.getByRole('button', { name: /发送请求/ }));
    await waitFor(() => expect(fetch).toHaveBeenCalledOnce());
    expect(fetch.mock.calls[0][1].headers['X-Airoute-Key-ID']).toBe('12');
    expect(JSON.parse(fetch.mock.calls[0][1].body).model).toBe('claude-3-5-sonnet');
  });

  it('exports an SDK placeholder without leaking session cookies or a selected record as a credential', async () => {
    const { copyToClipboard } = await import('../utils/clipboard');
    copyToClipboard.mockClear();
    render(<PlaygroundView {...baseProps} keys={listedKeys} />);
    fireEvent.click(screen.getByRole('button', { name: /复制 cURL/ }));
    await waitFor(() => expect(copyToClipboard).toHaveBeenCalled());
    const snippet = copyToClipboard.mock.calls[0][0];
    expect(snippet).toContain('/v1/chat/completions');
    expect(snippet).toContain('Authorization: Bearer <YOUR_API_KEY>');
    expect(snippet).not.toContain('/api/v1/playground');
    expect(snippet).not.toMatch(/ak_|console-csrf|airoute_session|X-Airoute-Key-ID/);
  });

  it('requires an active key and links directly to key creation', () => {
    const showToast = vi.fn(), onCreateKey = vi.fn();
    render(<PlaygroundView {...baseProps} keys={listedKeys.map(k => ({ ...k, status: 'disabled' }))} showToast={showToast} onCreateKey={onCreateKey} />);
    expect(screen.getByLabelText('选择 API 访问密钥')).toBeDisabled();
    expect(screen.queryByRole('option', { name: /免密|手动/ })).not.toBeInTheDocument();
    typeChatMessage();
    fireEvent.click(screen.getByRole('button', { name: /发送请求/ }));
    expect(fetch).not.toHaveBeenCalled();
    expect(showToast).toHaveBeenCalledWith(expect.stringContaining('创建并选择'), 'warning');
    fireEvent.click(screen.getByRole('button', { name: '创建访问密钥' }));
    expect(onCreateKey).toHaveBeenCalledOnce();
  });
});
