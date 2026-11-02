import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import PlaygroundView from './PlaygroundView';

const fragments = ['<|chan', 'nel>thought\n', 'Check the calculation.', '<chan', 'nel|>', '\nThe answer is **four**.'];
const props = {
  keys: [{ id: 1, key_id: 'ak_test', allowed_models: ['*'], status: 'active' }],
  models: ['gemma-4-31b-it'], modelRoutes: [], prices: [], lang: 'zh',
};

afterEach(() => vi.unstubAllGlobals());

describe('Gemma content in the playground', () => {
  it.each([true, false])('separates thinking from the answer (stream=%s)', async (stream) => {
    const frames = fragments.map((content) => `data: ${JSON.stringify({ choices: [{ delta: { content } }] })}\n\n`);
    frames.push('data: [DONE]\n\n');
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: true,
      headers: new Headers(),
      json: async () => ({ choices: [{ message: { content: fragments.join('') } }] }),
      body: new ReadableStream({
        start(controller) {
          frames.forEach((frame) => controller.enqueue(new TextEncoder().encode(frame)));
          controller.close();
        },
      }),
    })));
    const { container } = render(<PlaygroundView {...props} />);
    fireEvent.change(screen.getByLabelText('会话 API Key 明文'), { target: { value: 'sk-airoute-gemma-test' } });
    const toggle = screen.getByLabelText('开启 SSE 流式输出 (Streaming)');
    if (toggle.checked !== stream) fireEvent.click(toggle);
    fireEvent.change(screen.getByPlaceholderText(/输入对话 Prompt/), { target: { value: 'What is two plus two?' } });
    fireEvent.click(screen.getByRole('button', { name: /发送请求/ }));
    await waitFor(() => expect(screen.getByText('four')).toBeInTheDocument());
    expect(screen.getByText('Check the calculation.')).toBeInTheDocument();
    expect(container.textContent).not.toMatch(/<\|channel>|<channel\|>/);
    const sent = JSON.parse(globalThis.fetch.mock.calls[0][1].body);
    expect(sent.model).toBe('gemma-4-31b-it');
    expect(sent.stream).toBe(stream);
  });
});
