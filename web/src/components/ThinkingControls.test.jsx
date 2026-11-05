import { useState } from 'react';
import { expect, it } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import ThinkingControls from './ThinkingControls';
import { thinkingRequest } from '../lib/thinking';

function Harness({ capability, initial }) {
  const [value, setValue] = useState(initial);
  return (
    <>
      <ThinkingControls capability={capability} value={value} onChange={setValue} />
      <output data-testid="request">{JSON.stringify(thinkingRequest(capability, value))}</output>
    </>
  );
}

it('uses one selection for closed API levels and requests summaries without an extra step', () => {
  render(
    <Harness
      capability={{
        known: true,
        on: true,
        off: false,
        efforts: ['low', 'medium', 'high'],
        summary: true,
      }}
    />
  );
  expect(screen.getAllByRole('combobox')).toHaveLength(1);
  expect([...screen.getByLabelText('思考').options].map((option) => option.value)).toEqual([
    'auto',
    'low',
    'medium',
    'high',
  ]);
  fireEvent.change(screen.getByLabelText('思考'), {
    target: { value: 'high' },
  });
  expect(JSON.parse(screen.getByTestId('request').textContent)).toEqual({
    thinking_control: { mode: 'on', effort: 'high', include_summary: true },
  });
});

it('supports an open deployment toggle without manufacturing effort levels', () => {
  render(<Harness capability={{ known: true, on: true, off: true, efforts: [] }} />);
  expect([...screen.getByLabelText('思考').options].map((option) => option.value)).toEqual([
    'auto',
    'off',
    'on',
  ]);
  fireEvent.change(screen.getByLabelText('思考'), { target: { value: 'off' } });
  expect(JSON.parse(screen.getByTestId('request').textContent)).toEqual({
    thinking_control: { mode: 'off' },
  });
  fireEvent.change(screen.getByLabelText('思考'), { target: { value: 'on' } });
  expect(JSON.parse(screen.getByTestId('request').textContent)).toEqual({
    thinking_control: { mode: 'on' },
  });
});

it('keeps an invalid saved effort visible until the user chooses another', () => {
  render(
    <Harness
      capability={{
        known: true,
        on: true,
        off: false,
        efforts: ['low', 'high'],
      }}
      initial={{ mode: 'on', effort: 'max' }}
    />
  );
  expect(screen.getByLabelText('思考')).toHaveValue('max');
  expect(screen.getByRole('option', { name: '最高 · 不可用' })).toBeDisabled();
  expect(screen.getByRole('alert')).toBeInTheDocument();
  fireEvent.change(screen.getByLabelText('思考'), { target: { value: 'low' } });
  expect(JSON.parse(screen.getByTestId('request').textContent).thinking_control).toEqual({
    mode: 'on',
    effort: 'low',
  });
});
