import { useState } from 'react';
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import ModelMappingEditor from './ModelMappingEditor';

function setup(initial = [], lang = 'zh') {
  const onChange = vi.fn();
  function Form() {
    const [rows, setRows] = useState(initial);
    return <ModelMappingEditor rows={rows} lang={lang} onChange={next => { onChange(next); setRows(next); }} />;
  }
  render(<Form />);
  return onChange;
}

describe('structured model mapping editor', () => {
  it('starts collapsed and adds an editable draft row', () => {
    setup();
    expect(screen.queryByPlaceholderText(/原模型/)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /模型别名映射/ }));
    fireEvent.click(screen.getByRole('button', { name: /添加映射规则/ }));
    expect(screen.getByPlaceholderText(/原模型/)).toBeInTheDocument();
  });
  it('preserves colons and commas on both sides throughout editing', () => {
    const change = setup([{from:'source:tag,variant',to:'target:32b,latest'}]);
    fireEvent.change(screen.getByDisplayValue('target:32b,latest'), {target:{value:'target:70b,latest'}});
    expect(change).toHaveBeenLastCalledWith([{from:'source:tag,variant',to:'target:70b,latest'}]);
  });
  it('keeps incomplete rows in the parent draft so saving can reject them', () => {
    const change = setup([{from:'source',to:'target'}]);
    fireEvent.change(screen.getByDisplayValue('target'),{target:{value:''}});
    expect(change).toHaveBeenLastCalledWith([{from:'source',to:''}]);
    expect(screen.getByText(/1 条待补全/)).toBeInTheDocument();
  });
  it('removes exactly the selected row', () => {
    const change = setup([{from:'a',to:'b'},{from:'c',to:'d'}]);
    fireEvent.click(screen.getAllByRole('button',{name:/删除该映射/})[0]);
    expect(change).toHaveBeenLastCalledWith([{from:'c',to:'d'}]);
    expect(screen.getByDisplayValue('c')).toBeInTheDocument();
  });
  it('renders in English', () => {
    setup([], 'en');
    expect(screen.getByText('Model Alias Mapping')).toBeInTheDocument();
  });
});
