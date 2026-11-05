import { useState } from 'react';
import { expect, it } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import ModelThinkingCapabilities from './ModelThinkingCapabilities';

const profiles = [
  {
    id: 'gpu',
    label: 'GPU',
    on: true,
    off: true,
    efforts: ['low', 'high', 'xhigh', 'max'],
  },
  {
    id: 'api',
    label: 'API',
    on: true,
    off: false,
    efforts: ['low', 'medium', 'high'],
  },
];
function Harness({ initial = {} }) {
  const [channel, setChannel] = useState({
    models_str: 'open-model,closed-model',
    model_capabilities: initial,
  });
  return (
    <>
      <ModelThinkingCapabilities
        channel={channel}
        profiles={profiles}
        resolvedCapabilities={{
          'open-model': {
            source: 'catalog',
            profile: 'gpu',
            automatic: {
              profile: 'gpu',
              off: true,
              efforts: profiles[0].efforts,
            },
          },
          'closed-model': {
            source: 'catalog',
            profile: 'api',
            automatic: {
              profile: 'api',
              off: false,
              efforts: profiles[1].efforts,
            },
          },
        }}
        onChange={setChannel}
        isZh
      />
      <output data-testid="config">{JSON.stringify(channel.model_capabilities)}</output>
    </>
  );
}

it('edits only two facts for the selected model while keeping automatic transport', () => {
  render(<Harness />);
  expect(screen.getByLabelText('思考能力模型')).toBeVisible();
  expect(screen.getByLabelText('思考参数适配')).not.toBeVisible();
  expect(screen.getByLabelText('支持关闭思考')).toBeChecked();
  fireEvent.click(screen.getByLabelText('支持关闭思考'));
  fireEvent.click(screen.getByLabelText('支持档位 high'));
  expect(JSON.parse(screen.getByTestId('config').textContent)).toEqual({
    'open-model': {
      thinking: { can_disable: false, efforts: ['low', 'xhigh', 'max'] },
    },
  });
  fireEvent.change(screen.getByLabelText('思考能力模型'), {
    target: { value: 'closed-model' },
  });
  expect(screen.getByLabelText('支持关闭思考')).toBeDisabled();
  expect(screen.getByLabelText('支持档位 medium')).toBeChecked();
  expect(screen.queryByLabelText('支持档位 max')).not.toBeInTheDocument();
});

it('allows a fixed model default and preserves the empty set explicitly', () => {
  render(<Harness />);
  for (const effort of profiles[0].efforts)
    fireEvent.click(screen.getByLabelText(`支持档位 ${effort}`));
  expect(
    JSON.parse(screen.getByTestId('config').textContent)['open-model'].thinking.efforts
  ).toEqual([]);
});

it('restores the actual automatic facts rather than a previous manual adapter', () => {
  render(
    <Harness
      initial={{
        'closed-model': {
          thinking: { profile: 'gpu', can_disable: true, efforts: ['max'] },
        },
      }}
    />
  );
  fireEvent.change(screen.getByLabelText('思考能力模型'), {
    target: { value: 'closed-model' },
  });
  expect(screen.getByLabelText('支持关闭思考')).toBeChecked();
  fireEvent.click(screen.getByText('高级'));
  fireEvent.click(screen.getByRole('button', { name: '恢复自动识别' }));
  expect(screen.getByLabelText('支持关闭思考')).toBeDisabled();
  expect(screen.getByLabelText('支持档位 medium')).toBeChecked();
  expect(screen.queryByLabelText('支持档位 max')).not.toBeInTheDocument();
});
