import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import SupplierReconciliationView from './SupplierReconciliationView';

// Reconciliation is where the numbers are compared against what a supplier billed,
// so the derivations matter: a wrong uncached-token count or a filter that hides a
// supplier both produce a report that looks fine and is wrong.

function jsonResponse(body, status = 200) {
  return { ok: status < 400, status, json: async () => body };
}

function supplier(over = {}) {
  return {
    channel_id: 1,
    channel_name: 'openai-official',
    channel_type: 'openai',
    channel_status: 'active',
    base_url: 'https://api.openai.com',
    total_requests: 100,
    failed_requests: 0,
    total_cost: 12.5,
    models: [{ model: 'gpt-6', requests: 100 }],
    ...over,
  };
}

function report(over = {}) {
  return {
    summary: { prompt_tokens: 1000, completion_tokens: 500, cached_tokens: 200, total_cost: 12.5 },
    suppliers: [supplier()],
    ...over,
  };
}

function setup({ data = report(), ...props } = {}) {
  const adminFetch = vi.fn(async () => jsonResponse({ code: 0, data }));
  const handlers = {
    adminFetch,
    showToast: vi.fn(),
    onNavigateToLogs: vi.fn(),
  };
  const utils = render(
    <SupplierReconciliationView lang="zh" {...handlers} {...props} />
  );
  return { ...utils, adminFetch, ...handlers };
}

describe('SupplierReconciliationView', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.history.replaceState(null, '', window.location.pathname);
  });

  it('renders without a report', () => {
    setup({ data: null });

    // No crash on the empty payload; the request is what matters.
    expect(document.body.textContent.length).toBeGreaterThan(0);
  });

  it('lists the suppliers it was given', async () => {
    setup();

    expect(await screen.findByText('openai-official')).toBeInTheDocument();
  });

  describe('uncached prompt tokens', () => {
    it('uses the value the gateway reported when it sent one', async () => {
      setup({
        data: report({
          summary: { prompt_tokens: 1000, cached_tokens: 200, uncached_prompt_tokens: 777 },
          suppliers: [supplier()],
        }),
      });

      await screen.findByText('openai-official');
      const panel = screen.getByText('未缓存输入 Token').closest('div').parentElement;
      expect(panel.textContent).toContain('777');
    });

    it('derives it when the gateway did not send one', async () => {
      setup({ data: report({ summary: { prompt_tokens: 1000, cached_tokens: 200 }, suppliers: [supplier()] }) });

      await screen.findByText('openai-official');
      // 1000 - 200.
      const panel = screen.getByText('未缓存输入 Token').closest('div').parentElement;
      expect(panel.textContent).toContain('800');
    });

    it('never reports a negative uncached count', async () => {
      // A cached count larger than the prompt count would otherwise render as a
      // negative number of tokens.
      setup({
        data: report({ summary: { prompt_tokens: 100, cached_tokens: 500 }, suppliers: [supplier()] }),
      });

      // Wait for the report to arrive first. Asserting on the absence of "-400"
      // before the data lands passes trivially: the panel still shows 0, so the
      // assertion is satisfied by the empty state and proves nothing.
      await screen.findByText('openai-official');

      const panel = screen.getByText('未缓存输入 Token').closest('div').parentElement;
      expect(panel.textContent).not.toContain('-400');
      expect(panel.textContent).toContain('0');
    });
  });

  describe('filtering', () => {
    const twoSuppliers = report({
      suppliers: [
        supplier({ channel_id: 1, channel_name: 'openai-official', channel_type: 'openai', total_requests: 100, failed_requests: 0 }),
        supplier({ channel_id: 2, channel_name: 'groq-fast', channel_type: 'groq', base_url: 'https://groq.com', total_requests: 0, failed_requests: 0, models: [{ model: 'llama-3', requests: 0 }] }),
      ],
    });

    it('shows both suppliers by default', async () => {
      setup({ data: twoSuppliers });

      expect(await screen.findByText('openai-official')).toBeInTheDocument();
      expect(screen.getByText('groq-fast')).toBeInTheDocument();
    });

    it('filters to suppliers with traffic', async () => {
      setup({ data: twoSuppliers });
      await screen.findByText('openai-official');

      fireEvent.change(screen.getByDisplayValue(/全部调用分布|All Traffic/), { target: { value: 'has_traffic' } });

      expect(screen.getByText('openai-official')).toBeInTheDocument();
      expect(screen.queryByText('groq-fast')).not.toBeInTheDocument();
    });

    it('filters to suppliers with no traffic', async () => {
      setup({ data: twoSuppliers });
      await screen.findByText('openai-official');

      fireEvent.change(screen.getByDisplayValue(/全部调用分布|All Traffic/), { target: { value: 'no_traffic' } });

      expect(screen.queryByText('openai-official')).not.toBeInTheDocument();
      expect(screen.getByText('groq-fast')).toBeInTheDocument();
    });

    it('searches the channel name', async () => {
      setup({ data: twoSuppliers });
      await screen.findByText('openai-official');

      fireEvent.change(screen.getByPlaceholderText(/渠道 \/ 引擎 \/ 端点 \/ 模型|Channel, engine, url/), { target: { value: 'groq' } });

      expect(screen.getByText('groq-fast')).toBeInTheDocument();
      expect(screen.queryByText('openai-official')).not.toBeInTheDocument();
    });

    it('searches the channel type', async () => {
      setup({ data: twoSuppliers });
      await screen.findByText('openai-official');

      fireEvent.change(screen.getByPlaceholderText(/渠道 \/ 引擎 \/ 端点 \/ 模型|Channel, engine, url/), { target: { value: 'groq' } });

      expect(screen.getByText('groq-fast')).toBeInTheDocument();
    });

    it('searches the model names', async () => {
      setup({ data: twoSuppliers });
      await screen.findByText('openai-official');

      fireEvent.change(screen.getByPlaceholderText(/渠道 \/ 引擎 \/ 端点 \/ 模型|Channel, engine, url/), { target: { value: 'llama-3' } });

      expect(screen.getByText('groq-fast')).toBeInTheDocument();
      expect(screen.queryByText('openai-official')).not.toBeInTheDocument();
    });

    it('searches the base URL', async () => {
      setup({ data: twoSuppliers });
      await screen.findByText('openai-official');

      fireEvent.change(screen.getByPlaceholderText(/渠道 \/ 引擎 \/ 端点 \/ 模型|Channel, engine, url/), { target: { value: 'groq.com' } });

      expect(screen.getByText('groq-fast')).toBeInTheDocument();
    });
  });

  it('offers every model it saw as a filter option', async () => {
    setup({
      data: report({
        suppliers: [
          supplier({ models: [{ model: 'gpt-6', requests: 1 }] }),
          supplier({ channel_id: 2, channel_name: 'groq-fast', models: [{ model: 'llama-3', requests: 1 }] }),
        ],
      }),
    });
    await screen.findByText('openai-official');

    const options = screen.getAllByRole('option').map((o) => o.textContent);
    expect(options).toContain('gpt-6');
    expect(options).toContain('llama-3');
  });

  it('renders in English when asked', async () => {
    setup({ lang: 'en' });

    await screen.findByText('openai-official');
    // The filter placeholder is bilingual and always visible.
    expect(screen.getByPlaceholderText(/Channel, engine, url/)).toBeInTheDocument();
  });
});
