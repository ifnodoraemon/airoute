import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import AnnouncementBanner from './AnnouncementBanner';
import AnnouncementModal from './AnnouncementModal';

describe('Announcement System', () => {
  beforeEach(() => {
    localStorage.clear();
    vi.clearAllMocks();
  });

  it('renders announcement banner when enabled', async () => {
    const mockAdminFetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        code: 0,
        data: {
          id: 'ann_123',
          title: '集群升级通知',
          content: '今晚 24:00 进行网络升级',
          level: 'warning',
          enabled: true,
        }
      })
    });

    render(<AnnouncementBanner adminFetch={mockAdminFetch} lang="zh" />);

    await waitFor(() => {
      expect(screen.getByText('重要提醒')).toBeInTheDocument();
      expect(screen.getByText('集群升级通知:')).toBeInTheDocument();
      expect(screen.getByText('今晚 24:00 进行网络升级')).toBeInTheDocument();
    });
  });

  it('allows dismissing the banner and remembers in localStorage', async () => {
    const mockAdminFetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        code: 0,
        data: {
          id: 'ann_123',
          title: '集群升级通知',
          content: '今晚 24:00 进行网络升级',
          level: 'warning',
          enabled: true,
        }
      })
    });

    render(<AnnouncementBanner adminFetch={mockAdminFetch} lang="zh" />);

    await waitFor(() => {
      expect(screen.getByText('集群升级通知:')).toBeInTheDocument();
    });

    const closeBtn = screen.getByTitle('关闭公告（本条不再提示）');
    fireEvent.click(closeBtn);

    expect(screen.queryByText('集群升级通知:')).not.toBeInTheDocument();
    expect(localStorage.getItem('airoute_dismissed_announcement')).toBe('ann_123');
  });

  it('does not render banner if previously dismissed', async () => {
    localStorage.setItem('airoute_dismissed_announcement', 'ann_123');

    const mockAdminFetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        code: 0,
        data: {
          id: 'ann_123',
          title: '集群升级通知',
          content: '今晚 24:00 进行网络升级',
          level: 'warning',
          enabled: true,
        }
      })
    });

    render(<AnnouncementBanner adminFetch={mockAdminFetch} lang="zh" />);

    // Should not display
    expect(screen.queryByText('集群升级通知:')).not.toBeInTheDocument();
  });

  it('submits announcement from modal', async () => {
    const mockAdminFetch = vi.fn().mockImplementation((url, opts) => {
      if (url.includes('/config')) {
        return Promise.resolve({
          ok: true,
          json: async () => ({
            code: 0,
            data: {
              id: 'ann_1',
              title: '原有公告',
              content: '原有内容',
              level: 'info',
              enabled: false
            }
          })
        });
      }
      if (opts && opts.method === 'POST') {
        return Promise.resolve({
          ok: true,
          json: async () => ({ code: 0, message: 'ok' })
        });
      }
      return Promise.resolve({ ok: true, json: async () => ({ code: 0 }) });
    });

    const onPublished = vi.fn();
    const onClose = vi.fn();

    render(
      <AnnouncementModal
        isOpen={true}
        onClose={onClose}
        adminFetch={mockAdminFetch}
        onPublished={onPublished}
        lang="zh"
      />
    );

    await waitFor(() => {
      expect(screen.getByText('系统级全员公告发布')).toBeInTheDocument();
    });

    const submitBtn = screen.getByText('保存草稿并下线');
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(mockAdminFetch).toHaveBeenCalledWith('/api/v1/admin/announcement', expect.objectContaining({
        method: 'POST'
      }));
      expect(onPublished).toHaveBeenCalled();
      expect(onClose).toHaveBeenCalled();
    });
  });

  it('hides a withdrawn notice when the refresh fails', async () => {
    // A 401/500 used to leave the previous announcement rendered, so an operator
    // could act on a notice that had already been taken down.
    const mockAdminFetch = vi.fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ code: 0, data: { id: 'ann_1', title: '旧公告', content: '已下线', level: 'info', enabled: true } }),
      })
      .mockResolvedValue({ ok: false, status: 500, json: async () => ({ error: 'boom' }) });

    const { rerender } = render(<AnnouncementBanner adminFetch={mockAdminFetch} lang="zh" refreshKey={0} />);
    await waitFor(() => expect(screen.getByText('旧公告:')).toBeInTheDocument());

    rerender(<AnnouncementBanner adminFetch={mockAdminFetch} lang="zh" refreshKey={1} />);

    await waitFor(() => expect(screen.queryByText('旧公告:')).not.toBeInTheDocument());
  });

  it('refreshes immediately when the console publishes an announcement', async () => {
    const mockAdminFetch = vi.fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ code: 0, data: null }) })
      .mockResolvedValue({
        ok: true,
        json: async () => ({ code: 0, data: { id: 'ann_2', title: '新公告', content: '刚发布', level: 'critical', enabled: true } }),
      });

    const { rerender } = render(<AnnouncementBanner adminFetch={mockAdminFetch} lang="zh" refreshKey={0} />);
    await waitFor(() => expect(screen.queryByText('新公告:')).not.toBeInTheDocument());

    // Publishing bumps the key: the banner must not wait for its 60s poll.
    rerender(<AnnouncementBanner adminFetch={mockAdminFetch} lang="zh" refreshKey={1} />);

    await waitFor(() => expect(screen.getByText('新公告:')).toBeInTheDocument());
    expect(mockAdminFetch.mock.calls.length).toBeGreaterThanOrEqual(2);
  });
});
