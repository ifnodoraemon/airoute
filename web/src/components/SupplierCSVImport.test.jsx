import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import SupplierCSVImport from './SupplierCSVImport';

const response = (body, status = 200) => ({ ok: status === 200, status, json: async () => body });
const file = { text: async () => 'channel,model,input_cost,output_cost\na,m,1,2' };

describe('supplier CSV import journey', () => {
  it('previews the exact file, writes only after confirmation and refreshes the list', async () => {
    const adminFetch = vi.fn(async url => response({ code: 0, data: url.includes('dry_run')
      ? { valid: 1, items: [{ channel: 'a', model: 'm', input_cost: 1, output_cost: 2, status: 'active' }] }
      : { imported: 1 } }));
    const onImported = vi.fn();
    render(<SupplierCSVImport adminFetch={adminFetch} onImported={onImported} />);
    fireEvent.change(screen.getByLabelText('导入 CSV'), { target: { files: [file] } });
    await screen.findByRole('dialog');
    expect(adminFetch).toHaveBeenCalledOnce();
    expect(adminFetch.mock.calls[0][0]).toContain('dry_run=1');
    fireEvent.click(screen.getByRole('button', { name: '确认导入' }));
    await waitFor(() => expect(onImported).toHaveBeenCalledOnce());
    expect(adminFetch.mock.calls[1][1].body).toBe(adminFetch.mock.calls[0][1].body);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('shows invalid row details and prevents a rejected file from being committed', async () => {
    const adminFetch = vi.fn(async () => response({ code: 400, error: 'CSV 存在无效行', data: { errors: [{ row: 3, error: 'input_cost 不是有效数字' }] } }, 400));
    render(<SupplierCSVImport adminFetch={adminFetch} />);
    fireEvent.change(screen.getByLabelText('导入 CSV'), { target: { files: [file] } });
    expect(await screen.findByRole('alert')).toHaveTextContent('第 3 行：input_cost 不是有效数字');
    expect(screen.getByRole('button', { name: '确认导入' })).toBeDisabled();
    expect(adminFetch).toHaveBeenCalledOnce();
  });
});
