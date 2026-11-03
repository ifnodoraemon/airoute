import { describe, expect, it } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import PlaygroundView from './PlaygroundView';

const key = { id: 7, tenant_id: 'new-customer', key_id: 'ak_12345678901234567890', allowed_models: ['*'], status: 'active' };
describe('asynchronously loaded playground keys', () => {
  it('selects a key that arrives after the initial empty list', () => {
    const { rerender, container } = render(<PlaygroundView keys={[]} models={['chat-model']} />);
    expect(screen.getByLabelText('选择 API 访问密钥')).toBeDisabled();
    rerender(<PlaygroundView keys={[key]} models={['chat-model']} />);
    expect(screen.getByLabelText('选择 API 访问密钥')).toHaveValue('7');
    expect(container.querySelector('input[type="password"]')).toBeNull();
  });
  it('keeps a selected key across refresh and replaces it when revoked', () => {
    const second = { ...key, id: 8 };
    const { rerender } = render(<PlaygroundView keys={[key, second]} models={['chat-model']} />);
    fireEvent.change(screen.getByLabelText('选择 API 访问密钥'), { target: { value: '8' } });
    rerender(<PlaygroundView keys={[{ ...key }, { ...second }]} models={['chat-model']} />);
    expect(screen.getByLabelText('选择 API 访问密钥')).toHaveValue('8');
    rerender(<PlaygroundView keys={[key, { ...second, status: 'disabled' }]} models={['chat-model']} />);
    expect(screen.getByLabelText('选择 API 访问密钥')).toHaveValue('7');
  });
});
