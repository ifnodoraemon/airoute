import { describe, it, expect, vi, beforeEach } from 'vitest';
import { act, render, screen, fireEvent, waitFor } from '@testing-library/react';
import AnnouncementBanner from './AnnouncementBanner';
import AnnouncementModal from './AnnouncementModal';
import AnnouncementActions from './AnnouncementActions';

const announcement = { id: 'notice_123', announcement_id: 'ann_post', kind: 'announcement', title: '集群升级通知', content: '今晚进行升级', level: 'warning' };
const response = (data, status = 200) => ({ ok: status < 400, status, json: async () => data });

describe('announcement records', () => {
  beforeEach(() => { localStorage.clear(); vi.clearAllMocks(); });

  it('loads the banner from the announcement collection and links to the timeline', async () => {
    const fetcher = vi.fn().mockResolvedValue(response({ code: 0, data: announcement }));
    const open = vi.fn();
    render(<AnnouncementBanner adminFetch={fetcher} onOpen={open} />);
    await screen.findByText('集群升级通知:');
    expect(fetcher).toHaveBeenCalledWith('/api/v1/announcements/banner', expect.objectContaining({ signal: expect.any(AbortSignal) }));
    fireEvent.click(screen.getByRole('button', { name: '查看全部公告' }));
    expect(open).toHaveBeenCalledOnce();
    fireEvent.click(screen.getByRole('button', { name: '关闭公告（本条不再提示）' }));
    expect(screen.queryByText('集群升级通知:')).not.toBeInTheDocument();
    expect(localStorage.getItem('airoute_dismissed_announcement')).toBe('notice_123');
  });

  it('keeps a dismissed delivery hidden', async () => {
    localStorage.setItem('airoute_dismissed_announcement', announcement.id);
    const fetcher = vi.fn().mockResolvedValue(response({ code: 0, data: announcement }));
    render(<AnnouncementBanner adminFetch={fetcher} />);
    await waitFor(() => expect(fetcher).toHaveBeenCalledOnce());
    expect(screen.queryByText('集群升级通知:')).not.toBeInTheDocument();
  });

  it('hides a withdrawn banner after a failed refresh', async () => {
    const fetcher = vi.fn().mockResolvedValueOnce(response({ code: 0, data: announcement }))
      .mockResolvedValue(response({ error: 'unavailable' }, 503));
    const { rerender } = render(<AnnouncementBanner adminFetch={fetcher} refreshKey={0} />);
    await screen.findByText('集群升级通知:');
    rerender(<AnnouncementBanner adminFetch={fetcher} refreshKey={1} />);
    await waitFor(() => expect(screen.queryByText('集群升级通知:')).not.toBeInTheDocument());
  });

  it('ignores a stale response after the announcement changes', async () => {
    let finishOld;
    const fetcher = vi.fn().mockImplementationOnce(() => new Promise(resolve => { finishOld = resolve; }))
      .mockResolvedValue(response({ code: 0, data: { ...announcement, id: 'notice_new', title: '最新公告' } }));
    const { rerender } = render(<AnnouncementBanner adminFetch={fetcher} refreshKey={0} />);
    rerender(<AnnouncementBanner adminFetch={fetcher} refreshKey={1} />);
    await screen.findByText('最新公告:');
    await act(async () => finishOld(response({ code: 0, data: announcement })));
    expect(screen.queryByText('集群升级通知:')).not.toBeInTheDocument();
  });

  it('creates an independent draft without fetching a singleton configuration', async () => {
    const fetcher = vi.fn().mockResolvedValue(response({ code: 0 }));
    const published = vi.fn(), close = vi.fn();
    render(<AnnouncementModal isOpen adminFetch={fetcher} onPublished={published} onClose={close} />);
    expect(fetcher).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText('公告标题'), { target: { value: '独立草稿' } });
    fireEvent.click(screen.getByRole('button', { name: '保存草稿' }));
    await waitFor(() => expect(close).toHaveBeenCalledOnce());
    const [url, options] = fetcher.mock.calls[0];
    expect(url).toBe('/api/v1/admin/announcements');
    expect(options.method).toBe('POST');
    expect(JSON.parse(options.body)).toMatchObject({ title: '独立草稿', status: 'draft', show_banner: false, request_id: expect.any(String) });
    expect(JSON.parse(options.body)).not.toHaveProperty('enabled');
    expect(published).toHaveBeenCalledOnce();
  });

  it('edits and withdraws the selected published record', async () => {
    const fetcher = vi.fn().mockImplementation((url, options) => Promise.resolve(response(options?.method
      ? { code: 0 } : { code: 0, data: { title: '已发布公告', content: '正文', level: 'info', status: 'published', show_banner: false } })));
    const close = vi.fn();
    render(<AnnouncementModal isOpen announcementId="ann_post" adminFetch={fetcher} onClose={close} />);
    await screen.findByDisplayValue('已发布公告');
    fireEvent.click(screen.getByRole('button', { name: '撤回为草稿' }));
    await waitFor(() => expect(close).toHaveBeenCalledOnce());
    const [url, options] = fetcher.mock.calls.at(-1);
    expect(url).toBe('/api/v1/admin/announcements/ann_post');
    expect(options.method).toBe('PUT');
    expect(JSON.parse(options.body).status).toBe('draft');
  });

  it('deletes only the selected announcement after the in-page confirmation', async () => {
    const fetcher = vi.fn().mockResolvedValue(response({ code: 0 }));
    const changed = vi.fn();
    render(<AnnouncementActions id="ann_post" lang="zh" fetcher={fetcher} onChanged={changed} onEdit={vi.fn()} />);
    fireEvent.click(screen.getByRole('button', { name: '删除公告' }));
    expect(fetcher).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: '确认删除' }));
    await waitFor(() => expect(changed).toHaveBeenCalledOnce());
    expect(fetcher).toHaveBeenCalledWith('/api/v1/admin/announcements/ann_post', { method: 'DELETE' });
  });
});
