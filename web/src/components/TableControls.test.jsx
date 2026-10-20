import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { SortableTh, ColumnToggle } from './TableControls';

// The first component tests in the suite. Everything before this exercised hooks
// and the request layer; nothing rendered a component, so the markup, the
// accessibility attributes and the click wiring were unverified.

function renderTh(props) {
  return render(
    <table>
      <thead>
        <tr>
          <SortableTh {...props} />
        </tr>
      </thead>
    </table>
  );
}

describe('SortableTh', () => {
  it('renders a plain cell for a non-sortable column', () => {
    renderTh({ column: { key: 'id', label: 'ID', sortable: false }, sort: null, onSort: vi.fn() });

    expect(screen.getByText('ID')).toBeInTheDocument();
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  it('reports no sort direction when the column is inactive', () => {
    renderTh({
      column: { key: 'name', label: 'Name' },
      sort: { key: 'other', dir: 'asc' },
      onSort: vi.fn(),
    });

    expect(screen.getByRole('columnheader')).toHaveAttribute('aria-sort', 'none');
  });

  it('announces an ascending sort', () => {
    renderTh({ column: { key: 'name', label: 'Name' }, sort: { key: 'name', dir: 'asc' }, onSort: vi.fn() });

    expect(screen.getByRole('columnheader')).toHaveAttribute('aria-sort', 'ascending');
  });

  it('announces a descending sort', () => {
    renderTh({ column: { key: 'name', label: 'Name' }, sort: { key: 'name', dir: 'desc' }, onSort: vi.fn() });

    expect(screen.getByRole('columnheader')).toHaveAttribute('aria-sort', 'descending');
  });

  it('asks the parent to sort by the column key', () => {
    const onSort = vi.fn();
    renderTh({ column: { key: 'latency', label: 'Latency' }, sort: null, onSort });

    fireEvent.click(screen.getByRole('button'));

    expect(onSort).toHaveBeenCalledWith('latency');
  });
});

function columnControls({ columns, visible, ...overrides }) {
  return {
    columns,
    isVisible: (key) => visible.includes(key),
    toggleColumn: vi.fn(),
    resetColumns: vi.fn(),
    ...overrides,
  };
}

describe('ColumnToggle', () => {
  const columns = [
    { key: 'model', label: 'Model' },
    { key: 'channel', label: 'Channel' },
    { key: 'latency', label: 'Latency' },
  ];

  it('shows no count while every column is visible', () => {
    render(<ColumnToggle controls={columnControls({ columns, visible: ['model', 'channel', 'latency'] })} />);

    expect(screen.getByText('列')).toBeInTheDocument();
  });

  it('shows how many columns are hidden', () => {
    render(<ColumnToggle controls={columnControls({ columns, visible: ['model'] })} />);

    expect(screen.getByText('列 1/3')).toBeInTheDocument();
  });

  it('renders the English label when asked', () => {
    render(<ColumnToggle lang="en" controls={columnControls({ columns, visible: ['model', 'channel', 'latency'] })} />);

    expect(screen.getByText('Columns')).toBeInTheDocument();
    expect(screen.getByText('Visible columns')).toBeInTheDocument();
    expect(screen.getByText('Reset')).toBeInTheDocument();
  });

  it('checks exactly the visible columns', () => {
    render(<ColumnToggle controls={columnControls({ columns, visible: ['model', 'latency'] })} />);

    const boxes = screen.getAllByRole('checkbox');
    expect(boxes).toHaveLength(3);
    expect(boxes[0]).toBeChecked(); // model
    expect(boxes[1]).not.toBeChecked(); // channel
    expect(boxes[2]).toBeChecked(); // latency
  });

  it('toggles the column that was clicked', () => {
    const controls = columnControls({ columns, visible: ['model'] });
    render(<ColumnToggle controls={controls} />);

    fireEvent.click(screen.getAllByRole('checkbox')[1]);

    expect(controls.toggleColumn).toHaveBeenCalledWith('channel');
  });

  it('resets the columns', () => {
    const controls = columnControls({ columns, visible: ['model'] });
    render(<ColumnToggle controls={controls} />);

    fireEvent.click(screen.getByRole('button', { name: /重置/ }));

    expect(controls.resetColumns).toHaveBeenCalledTimes(1);
  });

  it('closes the dropdown when the user clicks away', () => {
    render(<ColumnToggle controls={columnControls({ columns, visible: ['model'] })} />);

    const details = document.querySelector('details');
    details.open = true;
    expect(details.open).toBe(true);

    fireEvent.mouseDown(document.body);

    expect(details.open).toBe(false);
  });

  it('stays open when the click lands inside it', () => {
    render(<ColumnToggle controls={columnControls({ columns, visible: ['model'] })} />);

    const details = document.querySelector('details');
    details.open = true;

    fireEvent.mouseDown(screen.getAllByRole('checkbox')[0]);

    expect(details.open).toBe(true);
  });
});
