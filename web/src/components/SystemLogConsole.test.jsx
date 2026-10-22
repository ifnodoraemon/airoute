import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import SystemLogConsole from './SystemLogConsole';

function entry(i) {
  return {
    id: i + 1,
    timestamp: new Date(2026, 9, 22, 16, 0, 0).toISOString(),
    level: 'INFO',
    message: `log ${i + 1}`,
    category: 'system',
    node_id: 'node-1',
    attrs: {},
  };
}

function adminFetchFor(rows) {
  return vi.fn().mockResolvedValue({
    ok: true,
    status: 200,
    json: async () => ({ code: 0, data: rows, total: rows.length, node_id: 'node-1' }),
  });
}

describe('SystemLogConsole', () => {
  it('renders only the newest window of a large buffer', async () => {
    // 900 buffered lines used to mean 900 mounted rows on every SSE record.
    const rows = Array.from({ length: 900 }, (_, i) => entry(i));
    render(<SystemLogConsole adminFetch={adminFetchFor(rows)} lang="zh" />);

    expect(await screen.findByText(/仅渲染最近 300 条/)).toBeInTheDocument();
    expect(screen.getAllByText(/^log \d+$/)).toHaveLength(300);
    expect(screen.getByText(/内存缓冲池: 900 \/ 2000/)).toBeInTheDocument();
  });

  it('surfaces the live-stream state when EventSource is unavailable', async () => {
    // jsdom has no EventSource: the console must fall back to state it can show
    // instead of silently pretending the stream is fine.
    render(<SystemLogConsole adminFetch={adminFetchFor([entry(0)])} lang="zh" />);

    expect(await screen.findByText(/正在连接实时流/)).toBeInTheDocument();
  });
});
