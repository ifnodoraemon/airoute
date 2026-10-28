// Regression test for the markdown code-block chrome language.
//
// Started as the BUG-WEB-06 probe; now asserts the CORRECT behaviour (it fails on
// the pre-fix code, which hard-coded Chinese labels and had no `lang` prop).
//
// Runner:
//   cd web && npx vitest run --configLoader runner src/components/MarkdownRenderer.i18n.test.jsx

import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import MarkdownRenderer from './MarkdownRenderer';

const FENCED_CODE = ['```js', 'const answer = 42;', 'const other = 1;', '```'].join('\n');

describe('MarkdownRenderer code block language', () => {
  it('uses English chrome for an English console', () => {
    render(<MarkdownRenderer content={FENCED_CODE} lang="en" />);

    expect(screen.getByTitle('Copy code')).toBeInTheDocument();
    expect(screen.getByText('Copy')).toBeInTheDocument();
    expect(screen.getByTitle('Toggle line wrap')).toBeInTheDocument();
    expect(screen.getByText('Wrap')).toBeInTheDocument();
    expect(screen.getByText('(2 lines)')).toBeInTheDocument();

    // No Chinese may leak into the English console.
    expect(screen.queryByText('复制')).toBeNull();
    expect(screen.queryByTitle('复制代码')).toBeNull();
    expect(screen.queryByTitle('切换自动换行')).toBeNull();
  });

  it('uses Chinese chrome for a Chinese console', () => {
    render(<MarkdownRenderer content={FENCED_CODE} lang="zh" />);

    expect(screen.getByTitle('复制代码')).toBeInTheDocument();
    expect(screen.getByText('复制')).toBeInTheDocument();
    expect(screen.getByTitle('切换自动换行')).toBeInTheDocument();
    expect(screen.getByText('(2 行)')).toBeInTheDocument();

    expect(screen.queryByText('Copy')).toBeNull();
  });

  it('defaults to Chinese when no language is provided', () => {
    render(<MarkdownRenderer content={FENCED_CODE} />);
    expect(screen.getByText('复制')).toBeInTheDocument();
  });
});
