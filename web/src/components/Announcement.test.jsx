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
});
