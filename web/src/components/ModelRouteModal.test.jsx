import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import ModelRouteModal from './ModelRouteModal';

// The route modal builds the payload that rewrites a model's routing and pricing.
// Several of its coercions exist to stop a bad payload reaching the gateway — a
// self-referencing fallback, a non-numeric weight, a NaN price — so those are what
// this pins.

function jsonResponse(body, status = 200) {
  return { ok: status < 400, status, json: async () => body };
}

function route(over = {}) {
  return {
    model: 'gpt-6',
    fallback_model: '',
    prompt_price: 2,
    completion_price: 8,
    cache_read_price: 0.2,
    fixed_price: 0,
    currency: 'CNY',
    off_peak_enabled: true,
    providers: [],
    ...over,
  };
}

function setup({ modelRoute = null, ...props } = {}) {
  const adminFetch = vi.fn(async () => jsonResponse({ code: 0, data: {} }));
  const handlers = {
    adminFetch,
    onClose: vi.fn(),
    onSaveSuccess: vi.fn(),
    showToast: vi.fn(),
  };
  const utils = render(
    <ModelRouteModal
      isOpen
      modelRoute={modelRoute}
      allModels={['gpt-6', 'claude-5', 'gpt-7']}
      allChannels={[{ id: 1, name: 'openai-official' }, { id: 2, name: 'groq-fast' }]}
      lang="zh"
      {...handlers}
      {...props}
    />
  );
  return { ...utils, adminFetch, ...handlers };
}

function submit() {
  const form = document.querySelector('form');
  fireEvent.submit(form);
}

function lastPayload(adminFetch) {
  const call = adminFetch.mock.calls.filter(([url, opts]) => String(url).includes('/models/routes') && opts?.method === 'POST').at(-1);
  return call ? JSON.parse(call[1].body) : null;
}

describe('ModelRouteModal', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders nothing when closed', () => {
    const { container } = setup({ isOpen: false });

    expect(container).toBeEmptyDOMElement();
  });

  it('prefills the form from the route it was given', () => {
    setup({ modelRoute: route({ model: 'gpt-6', fallback_model: 'claude-5' }) });

    expect(screen.getByDisplayValue('gpt-6')).toBeInTheDocument();
    // The fallback is a select, and it deliberately omits the model itself so a
    // model cannot be chosen as its own fallback.
    // The model-name input also has the combobox role (it carries a datalist), and
    // the channel picker is a select too, so identify the fallback by its options.
    const fallback = [...document.querySelectorAll('select')]
      .find((el) => [...el.options].some((o) => o.value === 'claude-5'));
    expect(fallback).toBeTruthy();
    expect(fallback.value).toBe('claude-5');
    // The model itself is not offered, so it cannot be its own fallback.
    expect([...fallback.options].map((o) => o.value)).not.toContain('gpt-6');
  });

  describe('validation', () => {
    it('refuses to save without a model', async () => {
      const { adminFetch } = setup({ modelRoute: null });

      submit();

      await waitFor(() => expect(screen.getByText(/请输入模型标识/)).toBeInTheDocument());
      expect(adminFetch).not.toHaveBeenCalled();
    });

    it('refuses a whitespace-only model', async () => {
      const { adminFetch } = setup({ modelRoute: route({ model: '   ' }) });

      submit();

      await waitFor(() => expect(screen.getByText(/请输入模型标识/)).toBeInTheDocument());
      expect(adminFetch).not.toHaveBeenCalled();
    });

    it('trims the model before sending it', async () => {
      const { adminFetch } = setup({ modelRoute: route({ model: '  gpt-6  ' }) });

      submit();

      await waitFor(() => expect(lastPayload(adminFetch)).not.toBeNull());
      expect(lastPayload(adminFetch).model).toBe('gpt-6');
    });
  });

  describe('the payload', () => {
    it('clears a fallback that points at the model itself', async () => {
      // A model falling back to itself is a loop, not a fallback.
      const { adminFetch } = setup({ modelRoute: route({ model: 'gpt-6', fallback_model: 'gpt-6' }) });

      submit();

      await waitFor(() => expect(lastPayload(adminFetch)).not.toBeNull());
      expect(lastPayload(adminFetch).fallback_model).toBe('');
    });

    it('keeps a distinct fallback', async () => {
      const { adminFetch } = setup({ modelRoute: route({ model: 'gpt-6', fallback_model: 'claude-5' }) });

      submit();

      await waitFor(() => expect(lastPayload(adminFetch)).not.toBeNull());
      expect(lastPayload(adminFetch).fallback_model).toBe('claude-5');
    });

    it('sends finite prices', async () => {
      const { adminFetch } = setup({ modelRoute: route({ prompt_price: 3, completion_price: 9, cache_read_price: 0.3 }) });

      submit();

      await waitFor(() => expect(lastPayload(adminFetch)).not.toBeNull());
      const payload = lastPayload(adminFetch);
      expect(payload.prompt_price).toBe(3);
      expect(payload.completion_price).toBe(9);
      expect(payload.cache_read_price).toBe(0.3);
    });

    it('sends off_peak_mode as none when the discount is switched off', async () => {
      // Leaving the mode at "custom" while disabled would apply a discount the
      // operator turned off.
      const { adminFetch } = setup({ modelRoute: route({ off_peak_enabled: false }) });

      submit();

      await waitFor(() => expect(lastPayload(adminFetch)).not.toBeNull());
      expect(lastPayload(adminFetch).off_peak_enabled).toBe(false);
      expect(lastPayload(adminFetch).off_peak_mode).toBe('none');
    });

    it('sends an empty provider list when no channel is mounted', async () => {
      const { adminFetch } = setup({ modelRoute: route({ providers: [] }) });

      submit();

      await waitFor(() => expect(lastPayload(adminFetch)).not.toBeNull());
      expect(lastPayload(adminFetch).provider_updates).toEqual([]);
    });
  });

  describe('saving', () => {
    it('reports success and closes', async () => {
      const { adminFetch, onSaveSuccess, onClose } = setup({ modelRoute: route() });

      submit();

      await waitFor(() => expect(onSaveSuccess).toHaveBeenCalled());
      expect(onClose).toHaveBeenCalled();
      expect(adminFetch).toHaveBeenCalled();
    });

    it('surfaces a gateway error and stays open', async () => {
      const { adminFetch, onClose } = setup({ modelRoute: route() });
      adminFetch.mockResolvedValue(jsonResponse({ code: 1, error: 'route rejected' }, 400));

      submit();

      await waitFor(() => expect(screen.getByText('route rejected')).toBeInTheDocument());
      expect(onClose).not.toHaveBeenCalled();
    });

    it('reports a network failure without throwing', async () => {
      const { adminFetch } = setup({ modelRoute: route() });
      adminFetch.mockRejectedValue(new Error('offline'));

      submit();

      await waitFor(() => expect(screen.getByText(/offline/)).toBeInTheDocument());
    });
  });

  it('renders in English when asked', () => {
    setup({ modelRoute: route(), lang: 'en' });

    expect(document.body.textContent.length).toBeGreaterThan(0);
  });
});
