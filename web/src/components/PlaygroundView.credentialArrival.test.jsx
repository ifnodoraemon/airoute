import { describe, expect, it } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import PlaygroundView from './PlaygroundView';
import { UNAUTHENTICATED_KEY } from '../lib/apiKey';

const key = { id: 7, tenant_id: 'new-customer', key_id: 'ak_12345678901234567890', allowed_models: ['*'], status: 'active' };
describe('asynchronously loaded playground credentials', () => {
  it('selects a key that arrives after the initial empty-list default', () => {
    const { rerender } = render(<PlaygroundView keys={[]} models={['chat-model']} />);
    rerender(<PlaygroundView keys={[key]} models={['chat-model']} />);
    expect(screen.getByLabelText('密钥上下文（租户与模型范围）')).toHaveValue('7');
    expect(screen.getByLabelText('会话 API Key 明文')).toBeInTheDocument();
  });

  it('preserves the public context when the user explicitly selected it', () => {
    const { rerender } = render(<PlaygroundView keys={[key]} models={['chat-model']} />);
    fireEvent.change(screen.getByLabelText('密钥上下文（租户与模型范围）'), { target: { value: UNAUTHENTICATED_KEY } });
    rerender(<PlaygroundView keys={[key, { ...key, id: 8 }]} models={['chat-model']} />);
    expect(screen.getByLabelText('密钥上下文（租户与模型范围）')).toHaveValue(UNAUTHENTICATED_KEY);
    expect(screen.queryByLabelText('会话 API Key 明文')).not.toBeInTheDocument();
  });
});
