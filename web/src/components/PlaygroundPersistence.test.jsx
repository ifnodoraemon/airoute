import { it, expect, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import PlaygroundView from './PlaygroundView';

it('preserves drafts and completed chat output when switching modality panels', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => ({
    ok: true, status: 200,
    body: new ReadableStream({ start(controller) {
      controller.enqueue(new TextEncoder().encode('data: {"choices":[{"delta":{"content":"persistent answer"}}]}\n\ndata: [DONE]'));
      controller.close();
    } }),
  })));
  try {
    render(<PlaygroundView keys={[{ id: 1, status: 'active', allowed_models: ['*'] }]} models={['gpt-4o', 'dall-e-3']} modelRoutes={[]} prices={[]} lang="zh" />);
    fireEvent.change(screen.getByPlaceholderText(/输入消息/), { target: { value: 'question' } });
    fireEvent.click(screen.getByRole('button', { name: /发送请求/ }));
    await waitFor(() => expect(screen.getByText('persistent answer')).toBeInTheDocument());
    fireEvent.change(screen.getByPlaceholderText(/输入消息/), { target: { value: 'next chat draft' } });
    fireEvent.click(screen.getByRole('button', { name: 'Images 绘图' }));
    const imagePrompt = screen.getAllByRole('textbox').find(input => input.tagName === 'TEXTAREA');
    fireEvent.change(imagePrompt, { target: { value: 'image draft' } });
    fireEvent.click(screen.getByRole('button', { name: 'Chat 对话' }));
    expect(screen.getByPlaceholderText(/输入消息/)).toHaveValue('next chat draft');
    expect(screen.getByText('persistent answer')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Images 绘图' }));
    expect(screen.getByDisplayValue('image draft')).toBeInTheDocument();
  } finally {
    vi.unstubAllGlobals();
  }
});
