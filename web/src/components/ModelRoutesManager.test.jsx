import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import ModelRoutesManager from './ModelRoutesManager';

// The routing screen is admin-only. Its two load-bearing behaviours are the gate
// that keeps a regular user out (the endpoints behind it are super-admin only, and
// calling them anyway produced a wall of permission errors) and the filter, which
// searches the model name and the provider channel names together.

function jsonResponse(body, status = 200) {
  return { ok: status < 400, status, json: async () => body };
}

function route(over = {}) {
  return {
    model: 'gpt-6',
    modality: 'chat',
    fallback_model: '',
    providers: [{ channel_id: 1, channel_name: 'openai-official', weight: 10, priority: 1 }],
    ...over,
  };
}

function setup({ routes = [], isAdmin = true, ...props } = {}) {
  const adminFetch = vi.fn(async (url) => {
    if (String(url).includes('/models/routes')) return jsonResponse({ code: 0, data: routes });
    if (String(url).includes('/channels')) return jsonResponse({ code: 0, data: [] });
    return jsonResponse({ code: 0, data: [] });
  });
  const handlers = {
    adminFetch,
    showToast: vi.fn(),
    onNavigateToPlayground: vi.fn(),
    setCurrentTab: vi.fn(),
  };
  const utils = render(
    <ModelRoutesManager isAdmin={isAdmin} lang="zh" {...handlers} {...props} />
  );
  return { ...utils, adminFetch, ...handlers };
}

describe('ModelRoutesManager', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // The filters live in the URL (useUrlFilters) so a filtered route list survives
    // a module switch. That also means a search typed by one test is still there for
    // the next one, so the query string is reset between them.
    window.history.replaceState(null, '', window.location.pathname);
  });

  describe('the admin gate', () => {
    it('tells a regular user the screen is restricted', () => {
      setup({ isAdmin: false });

      expect(screen.getByText(/仅超级管理员可查看与配置集群路由拓扑/)).toBeInTheDocument();
    });

    it('does not call the super-admin endpoints for a regular user', async () => {
      // This is the fix for the 403 storm: the endpoints are super-admin only, so
      // requesting them anyway produced a wall of permission errors instead of a
      // clear message.
      const { adminFetch } = setup({ isAdmin: false });

      await waitFor(() => expect(adminFetch).not.toHaveBeenCalled());
    });

    it('loads the routes for an administrator', async () => {
      const { adminFetch } = setup({ routes: [route()] });

      await waitFor(() => {
        expect(adminFetch.mock.calls.some(([url]) => String(url).includes('/models/routes'))).toBe(true);
      });
      expect(await screen.findByText('gpt-6')).toBeInTheDocument();
    });
  });

  describe('filtering', () => {
    const routes = [
      route({ model: 'gpt-6', modality: 'chat' }),
      route({ model: 'flux-pro', modality: 'images', providers: [{ channel_id: 2, channel_name: 'fal-ai', weight: 10, priority: 1 }] }),
      route({ model: 'whisper-v3', modality: 'audio_transcription', providers: [{ channel_id: 3, channel_name: 'groq', weight: 10, priority: 1 }] }),
    ];

    it('lists every route by default', async () => {
      setup({ routes });

      expect(await screen.findByText('gpt-6')).toBeInTheDocument();
      expect(screen.getByText('flux-pro')).toBeInTheDocument();
      expect(screen.getByText('whisper-v3')).toBeInTheDocument();
    });

    it('filters by modality', async () => {
      setup({ routes });
      await screen.findByText('gpt-6');

      fireEvent.click(screen.getByRole('button', { name: /生图|Image/ }));

      expect(screen.getByText('flux-pro')).toBeInTheDocument();
      expect(screen.queryByText('gpt-6')).not.toBeInTheDocument();
    });

    it('searches the model name', async () => {
      setup({ routes });
      await screen.findByText('gpt-6');

      fireEvent.change(screen.getByPlaceholderText(/搜索|Search/), { target: { value: 'whisper' } });

      expect(screen.getByText('whisper-v3')).toBeInTheDocument();
      expect(screen.queryByText('gpt-6')).not.toBeInTheDocument();
    });

    it('searches the provider channel name too', async () => {
      // An operator looking for "groq" wants the models served by it.
      setup({ routes });
      await screen.findByText('gpt-6');

      fireEvent.change(screen.getByPlaceholderText(/搜索|Search/), { target: { value: 'groq' } });

      expect(screen.getByText('whisper-v3')).toBeInTheDocument();
      expect(screen.queryByText('gpt-6')).not.toBeInTheDocument();
    });

    it('is case-insensitive', async () => {
      setup({ routes });
      await screen.findByText('gpt-6');

      fireEvent.change(screen.getByPlaceholderText(/搜索|Search/), { target: { value: 'GPT-6' } });

      expect(screen.getByText('gpt-6')).toBeInTheDocument();
    });

    it('shows nothing when the search matches no route', async () => {
      setup({ routes });
      await screen.findByText('gpt-6');

      fireEvent.change(screen.getByPlaceholderText(/搜索|Search/), { target: { value: 'zzzz' } });

      expect(screen.queryByText('gpt-6')).not.toBeInTheDocument();
      expect(screen.queryByText('flux-pro')).not.toBeInTheDocument();
    });
  });

  it('deletes the complete model identifier and refreshes the route list', async () => {
    const model = 'vendor/team/模型 +%2F?#&';
    const { adminFetch, showToast } = setup({ routes: [route({ model })] });
    await screen.findByText(model);
    adminFetch.mockImplementation(async (_url, options) => (
      options?.method === 'DELETE' ? jsonResponse({ code: 0 }) : jsonResponse({ code: 0, data: [] })
    ));
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true);
    try {
      fireEvent.click(screen.getByTitle('删除路由'));
      expect(confirm).toHaveBeenCalledWith(expect.stringContaining('渠道支持的模型配置将保留'));
      await waitFor(() => expect(showToast).toHaveBeenCalledWith(`已删除模型 [${model}] 路由，渠道模型配置已保留`, 'success'));
      const [target] = adminFetch.mock.calls.find(([, options]) => options?.method === 'DELETE');
      const url = new URL(target, window.location.origin);
      expect(url.pathname).toBe('/api/v1/admin/models/routes');
      expect(url.searchParams.get('model')).toBe(model);
      await waitFor(() => expect(screen.queryByText(model)).not.toBeInTheDocument());
    } finally {
      confirm.mockRestore();
    }
  });

  describe('modality badges', () => {
    it('labels each modality it knows', async () => {
      setup({
        routes: [
          route({ model: 'm-chat', modality: 'chat' }),
          route({ model: 'm-image', modality: 'images' }),
          route({ model: 'm-tts', modality: 'audio_speech' }),
          route({ model: 'm-stt', modality: 'audio_transcription' }),
          route({ model: 'm-video', modality: 'videos' }),
          route({ model: 'm-embed', modality: 'embeddings' }),
          route({ model: 'm-rerank', modality: 'rerank' }),
        ],
      });

      await screen.findByText('m-chat');
      for (const label of ['对话/文本', '生图', '语音TTS', '语音STT', '视频', '嵌入', '重排']) {
        expect(screen.getAllByText(label).length).toBeGreaterThan(0);
      }
    });

    it('falls back to the chat label for an unknown modality', async () => {
      // A modality the gateway adds later must still render something sensible.
      setup({ routes: [route({ model: 'm-unknown', modality: 'not-a-modality' })] });

      await screen.findByText('m-unknown');
      expect(screen.getAllByText('对话/文本').length).toBeGreaterThan(0);
    });
  });

  it('renders in English when asked', async () => {
    setup({ routes: [route()], lang: 'en' });

    expect(await screen.findByText(/Model Routes/)).toBeInTheDocument();
  });
});
