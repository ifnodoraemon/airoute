import { useState } from 'react';
import { expect, it } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import ModelThinkingCapabilities from './ModelThinkingCapabilities';

const openLevels = ['low', 'high', 'xhigh', 'max'];
function Harness({ initial = {} }) {
  const [channel, setChannel] = useState({
    models_str: 'open-model,closed-model',
    model_capabilities: initial,
  });
  return (
    <>
      <ModelThinkingCapabilities
        channel={channel}
        resolvedCapabilities={{
          'open-model': {
            known: true,
            off: true,
            supports_disable: true,
            efforts: openLevels,
            supported_efforts: openLevels,
          },
          'closed-model': {
            known: true,
            off: false,
            supports_disable: false,
            efforts: ['low', 'medium', 'high'],
            supported_efforts: ['low', 'medium', 'high'],
          },
        }}
        onChange={setChannel}
        isZh
      />
      <output data-testid="config">{JSON.stringify(channel.model_capabilities)}</output>
    </>
  );
}

it('edits two facts without any adapter catalog, picker or advanced entry', () => {
  render(<Harness />);
  expect(screen.getAllByRole('combobox')).toHaveLength(1);
  expect(screen.queryByText(/参数适配|高级/)).not.toBeInTheDocument();
  fireEvent.click(screen.getByLabelText('支持关闭思考'));
  fireEvent.click(screen.getByLabelText('支持档位 high'));
  expect(JSON.parse(screen.getByTestId('config').textContent)).toEqual({
    'open-model': { thinking: { can_disable: false, efforts: ['low', 'xhigh', 'max'] } },
  });
  fireEvent.change(screen.getByLabelText('思考能力模型'), { target: { value: 'closed-model' } });
  expect(screen.getByLabelText('支持关闭思考')).toBeEnabled();
  expect(screen.getByLabelText('支持档位 medium')).toBeChecked();
  expect(screen.getByLabelText('支持档位 max')).not.toBeChecked();
  fireEvent.click(screen.getByLabelText('支持档位 ultra'));
  fireEvent.click(screen.getByLabelText('支持关闭思考'));
  expect(JSON.parse(screen.getByTestId('config').textContent)['closed-model'].thinking).toEqual({
    can_disable: true, efforts: ['low', 'medium', 'high', 'ultra'],
  });
});

it('preserves an empty level set explicitly', () => {
  render(<Harness />);
  for (const effort of openLevels) fireEvent.click(screen.getByLabelText(`支持档位 ${effort}`));
  expect(
    JSON.parse(screen.getByTestId('config').textContent)['open-model'].thinking.efforts
  ).toEqual([]);
});

it('removes minimal from model settings and prevents adding it as a custom level', () => {
  render(<Harness initial={{ 'open-model': { thinking: { efforts: ['minimal', 'high'] } } }} />);
  expect(screen.queryByLabelText('支持档位 minimal')).not.toBeInTheDocument();
  fireEvent.change(screen.getByLabelText('自定义思考档位'), { target: { value: 'minimal' } });
  expect(screen.getByRole('button', { name: '添加' })).toBeDisabled();
  fireEvent.click(screen.getByLabelText('支持档位 ultra'));
  expect(JSON.parse(screen.getByTestId('config').textContent)['open-model'].thinking.efforts).toEqual(['high', 'ultra']);
});

it('uses backend available controls without modifying a managed legacy adapter', () => {
  render(
    <Harness
      initial={{
        'open-model': {
          thinking: { profile: 'backend-managed', can_disable: false, efforts: ['high'] },
        },
      }}
    />
  );
  expect(screen.getByLabelText('支持关闭思考')).toBeEnabled();
  expect(screen.getByLabelText('支持档位 low')).not.toBeChecked();
  fireEvent.click(screen.getByLabelText('支持关闭思考'));
  fireEvent.click(screen.getByLabelText('支持档位 max'));
  expect(JSON.parse(screen.getByTestId('config').textContent)['open-model'].thinking).toEqual({
    profile: 'backend-managed',
    can_disable: true,
    efforts: ['high', 'max'],
  });
});
