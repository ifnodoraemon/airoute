import { expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import ThinkingStatus from './ThinkingStatus';

it('explains native as upstream parameters without making it an effort level', () => {
  render(<ThinkingStatus observation={{ mode: 'native' }} />);
  expect(screen.getByLabelText('思考状态')).toHaveTextContent('上游原生参数');
  expect(screen.getByText(/上游原生参数/)).toHaveAttribute('title', '客户端直接传入上游原生思考参数；具体设置由上游解释。');
});
