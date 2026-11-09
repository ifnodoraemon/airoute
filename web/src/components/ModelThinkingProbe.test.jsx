import { useState } from 'react';
import { expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { apiJSON } from '../api/client';
import ModelThinkingCapabilities from './ModelThinkingCapabilities';

vi.mock('../api/client', () => ({ apiJSON: vi.fn() }));

function Harness({ initialThinking } = {}) {
  const [channel, setChannel] = useState({ type: 'openai', base_url: 'https://relay.example/v1', api_key: '', models_str: 'physical', model_capabilities: { physical: { thinking: initialThinking || { can_disable: true, efforts: ['low', 'max', 'extended'] } } } });
  return <>
    <ModelThinkingCapabilities channel={channel} channelId={7} onChange={setChannel} isZh />
    <output data-testid="config">{JSON.stringify(channel.model_capabilities.physical.thinking)}</output>
  </>;
}

it('probes the actual model and merges confirmed findings while retaining unknown settings', async () => {
  apiJSON.mockResolvedValueOnce({ data: { thinking: { can_disable: false, efforts: ['low', 'ultra'] }, checks: { off: { status: 'unsupported' }, low: { status: 'supported' }, max: { status: 'unsupported' }, ultra: { status: 'supported' }, extended: { status: 'unknown' } } } });
  render(<Harness />);
  fireEvent.click(screen.getByRole('button', { name: '自动探测' }));
  await waitFor(() => expect(screen.getByLabelText('支持档位 ultra')).toBeChecked());
  expect(JSON.parse(screen.getByTestId('config').textContent)).toEqual({ can_disable: false, efforts: ['low', 'extended', 'ultra'] });
  expect(JSON.parse(apiJSON.mock.calls[0][1].body)).toMatchObject({ channel_id: 7, model: 'physical', api_key: '' });
  expect(screen.getByText('探测结果').parentElement).not.toHaveAttribute('open');
});

it('keeps manual configuration when no new facts are confirmed', async () => {
  apiJSON.mockResolvedValueOnce({ data: { checks: { ultra: { status: 'unknown' } } } });
  render(<Harness />);
  fireEvent.click(screen.getByRole('button', { name: '自动探测' }));
  await screen.findByText('探测结果');
  expect(JSON.parse(screen.getByTestId('config').textContent)).toEqual({ can_disable: true, efforts: ['low', 'max', 'extended'] });
});

it('explains when actual requests cannot confirm adjustable levels', async () => {
  apiJSON.mockResolvedValueOnce({ data: { message: '已实际探测，未确认可调档位', checks: {} } });
  render(<Harness />);
  fireEvent.click(screen.getByRole('button', { name: '自动探测' }));
  expect(await screen.findByText('已实际探测，未确认可调档位')).toBeInTheDocument();
  expect(screen.queryByText(/待确认/)).not.toBeInTheDocument();
  expect(JSON.parse(screen.getByTestId('config').textContent).efforts).toEqual(['low', 'max', 'extended']);
});

it('replaces a fixed preset with the verified parameter dialect and capabilities', async () => {
  apiJSON.mockResolvedValueOnce({ data: { thinking: { profile: 'deepseek', can_disable: true, efforts: ['low', 'high', 'ultra'] }, checks: { off: { status: 'supported' }, low: { status: 'supported' }, high: { status: 'supported' }, ultra: { status: 'supported' } } } });
  render(<Harness initialThinking={{ profile: 'reasoning-only', efforts: [] }} />);
  fireEvent.click(screen.getByRole('button', { name: '自动探测' }));
  await waitFor(() => expect(screen.getByLabelText('支持关闭思考')).toBeChecked());
  expect(JSON.parse(screen.getByTestId('config').textContent)).toEqual({ profile: 'deepseek', can_disable: true, efforts: ['low', 'high', 'ultra'] });
});

it('allows another named level without offering switch values as efforts', () => {
  render(<Harness />);
  const input = screen.getByLabelText('自定义思考档位');
  fireEvent.change(input, { target: { value: 'none' } });
  expect(screen.getByRole('button', { name: '添加' })).toBeDisabled();
  fireEvent.change(input, { target: { value: 'intensive' } });
  fireEvent.click(screen.getByRole('button', { name: '添加' }));
  expect(screen.getByLabelText('支持档位 intensive')).toBeChecked();
});
