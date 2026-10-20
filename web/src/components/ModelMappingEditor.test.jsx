import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import ModelMappingEditor from './ModelMappingEditor';

// The mapping is the "serve model X from upstream model Y" table. It round-trips
// through a "from:to, from:to" string, which is what the channel form stores, so a
// parse or serialise bug silently rewrites an operator's routing.

function setup({ mappingStr = '', ...props } = {}) {
  const onChange = vi.fn();
  const utils = render(<ModelMappingEditor mappingStr={mappingStr} onChange={onChange} lang="zh" {...props} />);
  return { ...utils, onChange };
}

function openEditor() {
  fireEvent.click(screen.getByRole('button', { name: /模型别名映射|Model Alias Mapping/ }));
}

describe('ModelMappingEditor', () => {
  it('starts collapsed when there is no mapping', () => {
    setup();

    expect(screen.queryByPlaceholderText(/原模型|Source Model/)).not.toBeInTheDocument();
  });

  it('starts expanded when a mapping was passed', () => {
    setup({ mappingStr: 'gpt-4o:deepseek-chat' });

    expect(screen.getByDisplayValue('gpt-4o')).toBeInTheDocument();
    expect(screen.getByDisplayValue('deepseek-chat')).toBeInTheDocument();
  });

  it('shows an empty state when opened with no rules', () => {
    // Expanding does not conjure an input row: with nothing configured there is
    // nothing to edit, and the panel says so.
    setup();

    openEditor();

    expect(screen.getByText(/暂无映射规则/)).toBeInTheDocument();
    expect(screen.queryByPlaceholderText(/原模型|Source Model/)).not.toBeInTheDocument();
  });

  it('shows an input row once a rule is added', () => {
    setup();
    openEditor();

    fireEvent.click(screen.getByRole('button', { name: /添加映射规则|Add Mapping Rule/ }));

    expect(screen.getByPlaceholderText(/原模型|Source Model/)).toBeInTheDocument();
  });

  describe('parsing', () => {
    it('splits on the first colon only, so a value may contain one', () => {
      // A model id can carry a colon (a tag or a namespace); only the first one
      // separates the two sides.
      setup({ mappingStr: 'gpt-4o:openai/gpt-4o:latest' });

      expect(screen.getByDisplayValue('gpt-4o')).toBeInTheDocument();
      expect(screen.getByDisplayValue('openai/gpt-4o:latest')).toBeInTheDocument();
    });

    it('trims both sides', () => {
      setup({ mappingStr: '  gpt-4o  :  deepseek-chat  ' });

      expect(screen.getByDisplayValue('gpt-4o')).toBeInTheDocument();
      expect(screen.getByDisplayValue('deepseek-chat')).toBeInTheDocument();
    });

    it('ignores an entry with no colon', () => {
      setup({ mappingStr: 'gpt-4o:deepseek-chat, nonsense' });

      expect(screen.getAllByPlaceholderText(/原模型|Source Model/)).toHaveLength(1);
    });

    it('ignores an entry that is only a colon', () => {
      setup({ mappingStr: 'gpt-4o:deepseek-chat, :' });

      expect(screen.getAllByPlaceholderText(/原模型|Source Model/)).toHaveLength(1);
    });
  });

  describe('editing', () => {
    it('adds an empty row', () => {
      setup();
      openEditor();

      fireEvent.click(screen.getByRole('button', { name: /添加映射规则|Add Mapping Rule/ }));

      expect(screen.getAllByPlaceholderText(/原模型|Source Model/)).toHaveLength(1);
    });

    it('removes a row', () => {
      setup({ mappingStr: 'a:b, c:d' });
      expect(screen.getAllByPlaceholderText(/原模型|Source Model/)).toHaveLength(2);

      fireEvent.click(screen.getAllByRole('button', { name: /删除|Remove/ })[0]);

      expect(screen.getAllByPlaceholderText(/原模型|Source Model/)).toHaveLength(1);
      expect(screen.getByDisplayValue('c')).toBeInTheDocument();
    });

    it('reports the mapping when a row is completed', () => {
      const { onChange } = setup();
      openEditor();
      fireEvent.click(screen.getByRole('button', { name: /添加映射规则|Add Mapping Rule/ }));

      fireEvent.change(screen.getByPlaceholderText(/原模型|Source Model/), { target: { value: 'gpt-4o' } });
      fireEvent.change(screen.getByPlaceholderText(/映射模型|Target Model/), { target: { value: 'deepseek-chat' } });

      expect(onChange).toHaveBeenLastCalledWith('gpt-4o:deepseek-chat');
    });

    it('does not report a half-filled row', () => {
      // A row with only a source would serialise to "gpt-4o:" and be rejected by the
      // gateway, so it is held back until it is complete.
      const { onChange } = setup();
      openEditor();
      fireEvent.click(screen.getByRole('button', { name: /添加映射规则|Add Mapping Rule/ }));

      fireEvent.change(screen.getByPlaceholderText(/原模型|Source Model/), { target: { value: 'gpt-4o' } });

      expect(onChange).toHaveBeenLastCalledWith('');
    });

    it('drops a row that was cleared back to empty', () => {
      const { onChange } = setup({ mappingStr: 'a:b' });

      fireEvent.change(screen.getByDisplayValue('b'), { target: { value: '' } });

      expect(onChange).toHaveBeenLastCalledWith('');
    });

    it('joins several rows with a comma and a space', () => {
      const { onChange } = setup({ mappingStr: 'a:b' });
      fireEvent.click(screen.getByRole('button', { name: /添加映射规则|Add Mapping Rule/ }));

      const sources = screen.getAllByPlaceholderText(/原模型|Source Model/);
      const targets = screen.getAllByPlaceholderText(/映射模型|Target Model/);
      fireEvent.change(sources[1], { target: { value: 'c' } });
      fireEvent.change(targets[1], { target: { value: 'd' } });

      expect(onChange).toHaveBeenLastCalledWith('a:b, c:d');
    });
  });

  it('renders in English when asked', () => {
    setup({ lang: 'en' });

    expect(screen.getByText(/Model Alias Mapping/)).toBeInTheDocument();
  });
});
