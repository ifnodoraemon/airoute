import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import GeminiPlayground from './GeminiPlayground';
import GeminiMediaOutput from './GeminiMediaOutput';

afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks(); });

describe('Gemini native playground', () => {
  it('renders native text, thoughts, images, audio and video and preserves signed history', async () => {
    vi.stubGlobal('URL', class extends URL { static createObjectURL = vi.fn(() => 'blob:gemini-test'); static revokeObjectURL = vi.fn(); });
    const parts = [{ text: 'thinking', thought: true }, { text: 'native answer' }, { text: '', thoughtSignature: 'keep-me==' },
      { inlineData: { mimeType: 'image/png', data: 'AA==' } }, { inlineData: { mimeType: 'audio/L16;rate=24000', data: 'AAA=' } }, { inlineData: { mimeType: 'video/mp4', data: 'AA==' } }];
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => ({ candidates: [{ content: { parts } }] }) })));
    const { container } = render(<GeminiPlayground models={['gemma-4-31b-it']} resolveCredentials={() => ({ key: 'sk-test' })} />);
    fireEvent.click(screen.getByRole('checkbox')); fireEvent.click(screen.getByRole('button', { name: '发送原生请求' }));
    await waitFor(() => expect(screen.getByText('native answer')).toBeInTheDocument());
    expect(screen.getByText('thinking')).toBeInTheDocument();
    expect(await screen.findByRole('img')).toBeInTheDocument(); expect(container.querySelector('audio')).toBeTruthy(); expect(container.querySelector('video')).toBeTruthy();
    const [path, options] = fetch.mock.calls[0]; expect(path).toBe('/v1beta/models/gemma-4-31b-it:generateContent'); expect(options.headers.Authorization).toBe('Bearer sk-test');
    fireEvent.click(screen.getByRole('button', { name: '将完整回复加入下一轮' }));
    const request = JSON.parse(screen.getByLabelText('Gemini 请求 JSON').value);
    expect(request.contents[1].parts).toEqual(parts);
  });

  it.each([
    [{ promptFeedback: { blockReason: 'SAFETY', safetyRatings: [{ category: 'danger', blocked: true }] } }, 'SAFETY'],
    [{ candidates: [{ finishReason: 'MAX_TOKENS', content: { parts: [{ text: 'partial answer' }] } }] }, 'MAX_TOKENS'],
    [{ candidates: [{ finishReason: 'RECITATION' }] }, 'RECITATION'],
    [{}, '未返回内容'],
  ])('explains native termination instead of leaving a blank result', async (frame, reason) => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => frame })));
    render(<GeminiPlayground models={['m']} resolveCredentials={() => ({ key: 'sk-test' })} />);
    fireEvent.click(screen.getByRole('checkbox'));
    fireEvent.click(screen.getByRole('button', { name: '发送原生请求' }));
    expect(await screen.findByRole('status')).toHaveTextContent(reason);
    expect(screen.queryByRole('button', { name: '将完整回复加入下一轮' })).not.toBeInTheDocument();
  });

  it('refuses a missing credential before making a request', () => {
    vi.stubGlobal('fetch', vi.fn());
    render(<GeminiPlayground models={['m']} resolveCredentials={() => null} />);
    fireEvent.click(screen.getByRole('button', { name: '发送原生请求' })); expect(fetch).not.toHaveBeenCalled();
  });

  it('downloads private Google media through the owned gateway route', async () => {
    vi.stubGlobal('URL', class extends URL { static createObjectURL = vi.fn(() => 'blob:private-video'); static revokeObjectURL = vi.fn(); });
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, blob: async () => new Blob(['video'], { type: 'video/mp4' }) })));
    const { container } = render(<GeminiMediaOutput item={{ mime: 'video/mp4', uri: 'https://generativelanguage.googleapis.com/v1beta/files/owned' }} index={0} apiKey="gateway-key" />);
    await waitFor(() => expect(container.querySelector('video')).toHaveAttribute('src', 'blob:private-video'));
    expect(fetch.mock.calls[0][0]).toBe('/v1beta/files/owned:download?alt=media');
    expect(fetch.mock.calls[0][1].headers.Authorization).toBe('Bearer gateway-key');
  });
});
