import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import OnboardingModal from './OnboardingModal';

// The list endpoint returns display masks (credentials are stored as digests),
// so the wizard must never put a listed key into a snippet or a live request.
// These cases pin that boundary for every snippet format and for the probe.

const MASK = '3a4b5c6d***9f8e';
const PLAINTEXT = 'sk-airoute-0123456789abcdef';

const keys = [
  { id: 7, key: MASK, key_fingerprint: 'kfp_7', tenant_id: 'acme', allowed_models: ['*'], rpm: 60, status: 'active' },
];
const models = ['gpt-4o', 'text-embedding-3-small'];

function setup(overrides = {}) {
  const showToast = vi.fn();
  const utils = render(
    <OnboardingModal
      isOpen
      onClose={vi.fn()}
      keys={keys}
      models={models}
      showToast={showToast}
      lang="zh"
      {...overrides}
    />
  );
  return { ...utils, showToast };
}

// Walks the wizard to step 3 (SDK & handshake), using the default OpenAI protocol.
function goToStep3() {
  fireEvent.click(screen.getByRole('button', { name: /下一步：选择模型与协议/ }));
  fireEvent.click(screen.getByRole('button', { name: /下一步：代码验证与握手/ }));
}

function snippetText() {
  return document.querySelector('pre').textContent;
}

describe('OnboardingModal masked keys', () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    globalThis.fetch = originalFetch;
  });

  it('keys the entitlement select off the row id, not the masked value', () => {
    setup();

    const select = screen.getByLabelText('密钥上下文（租户与模型范围）');
    expect(select.value).toBe('7');
    // The mask is not offered as a selectable credential value.
    expect(Array.from(select.options).map(o => o.value)).toEqual(['7']);
  });

  it('renders every SDK snippet with a placeholder, never the listed mask', () => {
    setup();
    goToStep3();

    expect(snippetText()).toContain('<YOUR_API_KEY>');
    expect(snippetText()).not.toContain('***');
    expect(snippetText()).not.toContain(MASK);

    fireEvent.click(screen.getByRole('button', { name: 'TypeScript / Node' }));
    expect(snippetText()).toContain('<YOUR_API_KEY>');
    expect(snippetText()).not.toContain('***');

    fireEvent.click(screen.getByRole('button', { name: 'cURL' }));
    expect(snippetText()).not.toContain('***');
    expect(snippetText()).toContain('Authorization: Bearer <YOUR_API_KEY>');

    fireEvent.click(screen.getByRole('button', { name: 'Claude SDK' }));
    expect(snippetText()).toContain('<YOUR_API_KEY>');
    expect(snippetText()).not.toContain('***');
  });

  it('disables the live probe until a usable plaintext is pasted', () => {
    setup();
    goToStep3();

    const probe = screen.getByRole('button', { name: /立即测试握手/ });
    expect(probe).toBeDisabled();

    fireEvent.change(screen.getByLabelText('会话 API Key 明文'), { target: { value: MASK } });
    // A pasted mask is still not a credential.
    expect(probe).toBeDisabled();

    fireEvent.change(screen.getByLabelText('会话 API Key 明文'), { target: { value: PLAINTEXT } });
    expect(probe).not.toBeDisabled();
  });

  it('sends the pasted plaintext in the probe and never the mask', async () => {
    globalThis.fetch.mockResolvedValue({
      ok: true,
      json: async () => ({ choices: [{ message: { content: 'pong' } }] }),
    });
    setup();
    goToStep3();

    fireEvent.change(screen.getByLabelText('会话 API Key 明文'), { target: { value: PLAINTEXT } });
    fireEvent.click(screen.getByRole('button', { name: /立即测试握手/ }));

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalled());
    const [url, options] = globalThis.fetch.mock.calls[0];
    expect(String(url)).toContain('/v1/chat/completions');
    expect(options.headers.Authorization).toBe(`Bearer ${PLAINTEXT}`);
    expect(JSON.stringify(options.headers)).not.toContain('***');
  });

  it('uses the protocol-specific header for a Claude probe', () => {
    setup();
    fireEvent.click(screen.getByRole('button', { name: /下一步：选择模型与协议/ }));
    fireEvent.click(screen.getByRole('button', { name: /Claude Messages/ }));
    fireEvent.click(screen.getByRole('button', { name: /下一步：代码验证与握手/ }));
    fireEvent.click(screen.getByRole('button', { name: 'cURL' }));

    expect(snippetText()).toContain('"x-api-key: <YOUR_API_KEY>"');
    expect(snippetText()).not.toContain('***');
  });
});
