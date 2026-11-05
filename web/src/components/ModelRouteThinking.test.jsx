import { expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import ModelRouteModal from './ModelRouteModal';

const capability = (efforts, extra = {}) => ({
  known: true,
  on: true,
  off: false,
  efforts,
  source: 'configured',
  ...extra,
});
const gpu = capability(['low', 'high', 'xhigh', 'max'], { off: true });
const proxy = capability([]);
const google = capability(['low', 'medium', 'high']);

function setup(caps, { policy, status = [], models, mapped = [] } = {}) {
  const channels = caps.map((thinking, index) => ({
    id: index + 1,
    name: `channel-${index + 1}`,
    type: 'custom',
    status: status[index] || 'active',
    models: models?.[index] || [`actual-${index + 1}`],
    thinking_capabilities: { [`actual-${index + 1}`]: thinking },
  }));
  const providers = caps.map((thinking, index) => ({
    channel_id: index + 1,
    channel_name: channels[index].name,
    mapped_model: mapped[index] || `actual-${index + 1}`,
    thinking,
    priority: 1,
    weight: 1,
  }));
  const adminFetch = vi.fn(async () => ({
    ok: true,
    json: async () => ({ code: 0 }),
  }));
  render(
    <ModelRouteModal
      isOpen
      modelRoute={{ model: 'public-alias', providers, thinking_policy: policy }}
      allChannels={channels}
      allModels={['public-alias']}
      adminFetch={adminFetch}
      showToast={vi.fn()}
      onClose={vi.fn()}
      onSaveSuccess={vi.fn()}
    />
  );
  return { adminFetch, channels };
}
const choose = (value) =>
  fireEvent.change(screen.getByLabelText('默认思考'), { target: { value } });
const options = () => [...screen.getByLabelText('默认思考').options].map((item) => item.value);
const saveButton = () => screen.getByRole('button', { name: /保存/ });

it('shows only the four real GPU levels in a mixed route and explains partial coverage', () => {
  setup([gpu, proxy]);
  expect(options()).toEqual(['auto', 'off', 'low', 'high', 'xhigh', 'max']);
  expect(screen.getByRole('option', { name: '最高' })).toHaveAttribute('title', 'channel-1');
  expect(screen.queryByRole('option', { name: '中' })).not.toBeInTheDocument();
});

it('shows only three Gemini levels and never offers unsupported off', () => {
  setup([google]);
  expect(options()).toEqual(['auto', 'low', 'medium', 'high']);
});

it('keeps a single upstream level and does not manufacture controls for toggle-only models', () => {
  setup([capability(['high'], { off: true })]);
  expect(options()).toEqual(['auto', 'off', 'high']);
});

it('uses model default when no adjustable upstream effort is confirmed', () => {
  setup([proxy]);
  expect(options()).toEqual(['auto', 'on']);
  choose('on');
  expect(
    screen
      .getAllByRole('combobox')
      .filter((node) => node.getAttribute('aria-label')?.includes('思考'))
  ).toHaveLength(1);
  expect(saveButton()).toBeEnabled();
});

it('uses the union across providers rather than requiring all providers to share a level', () => {
  setup([google, capability(['low', 'high', 'max'])]);
  expect(options()).toEqual(['auto', 'low', 'medium', 'high', 'max']);
  expect(screen.getByRole('option', { name: '中' })).toHaveAttribute('title', 'channel-1');
  expect(screen.getByRole('option', { name: '高' })).toHaveAttribute(
    'title',
    'channel-1 / channel-2'
  );
});

it('does not include disabled channel capabilities or unconfirmed models', () => {
  setup([gpu, { known: false }], { status: ['disabled', 'active'] });
  expect(options()).toEqual(['auto']);
});

it('enables restrictions with only supported levels and posts that same subset', async () => {
  const { adminFetch } = setup([google]);
  choose('high');
  fireEvent.click(screen.getByText('高级'));
  fireEvent.click(screen.getByLabelText('限制可选思考强度'));
  expect(screen.queryByLabelText('允许强度 max')).not.toBeInTheDocument();
  fireEvent.click(saveButton());
  await waitFor(() => expect(adminFetch).toHaveBeenCalled());
  expect(JSON.parse(adminFetch.mock.calls[0][1].body).thinking_policy).toMatchObject({
    default_mode: 'on',
    default_effort: 'high',
    allowed_efforts: ['low', 'medium', 'high'],
  });
});

it('preserves and flags a stale default instead of silently lowering it', () => {
  setup([google], {
    policy: {
      default_mode: 'on',
      default_effort: 'max',
      allowed_modes: null,
      allowed_efforts: null,
    },
  });
  expect(screen.getByLabelText('默认思考')).toHaveValue('max');
  expect(screen.getByRole('option', { name: '最高 · 不可用' })).toBeDisabled();
  expect(saveButton()).toBeDisabled();
  choose('high');
  expect(saveButton()).toBeEnabled();
});

it('requires unsupported legacy restrictions to be explicitly removed', () => {
  setup([google], {
    policy: {
      default_mode: 'on',
      default_effort: 'high',
      allowed_modes: null,
      allowed_efforts: ['low', 'medium', 'high', 'max'],
    },
  });
  expect(saveButton()).toBeDisabled();
  fireEvent.click(screen.getByText('高级'));
  expect(screen.getByLabelText('允许强度 max')).toBeChecked();
  fireEvent.click(screen.getByLabelText('允许强度 max'));
  expect(screen.queryByLabelText('允许强度 max')).not.toBeInTheDocument();
  expect(saveButton()).toBeEnabled();
});
