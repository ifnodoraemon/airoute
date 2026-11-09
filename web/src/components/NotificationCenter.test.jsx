import { describe, it, expect, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import NotificationCenterView from './NotificationCenterView';
import NotificationFeed from './NotificationFeed';
import NotificationComposeModal from './NotificationComposeModal';
import NotificationBell from './NotificationBell';

const announcement = { id: 'ann_first', kind: 'announcement', title: '十月平台更新', content: '第一段\n\n第二段 <img src=x onerror=alert(1)>', level: 'info', created_at: '2026-10-28T08:00:00Z', read_at: null };
const message = { id: 'msg_first', kind: 'message', title: '账单更新', content: '你的账单已经更新。\n\n请查看最新明细。', level: 'warning', created_at: '2026-11-01T08:00:00Z', read_at: null };
const counts = { unread_count: 2, announcement_unread_count: 1, message_unread_count: 1 };
const response = (data, status = 200) => ({ ok: status < 400, status, json: async () => data });
const page = items => ({ items, total: items.length, page: 1, page_size: 20, latest_sequence: 18, ...counts });

function fetchFeed(items, mutation = { code: 0 }) {
  return vi.fn(async (url, options) => options?.method === 'POST' ? response(mutation) : response({ code: 0, data: page(items) }));
}

describe('announcement timeline and personal inbox', () => {
  it('keeps the two destinations distinct and does not expose publishing controls to a user', async () => {
    const change = vi.fn();
    render(<NotificationCenterView kind="announcement" onKindChange={change} counts={counts} fetcher={fetchFeed([announcement])} />);
    expect(await screen.findByRole('list', { name: '全站公告时间线' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: /全站公告/ })).toHaveAttribute('aria-selected', 'true');
    fireEvent.click(screen.getByRole('tab', { name: /我的消息/ }));
    expect(change).toHaveBeenCalledWith('message');
    expect(screen.queryByRole('button', { name: '发送个人消息' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: '发布全站公告' })).not.toBeInTheDocument();
  });

  it('opens full announcement text and only marks it read after an explicit read action', async () => {
    const fetcher = fetchFeed([announcement]);
    const changed = vi.fn();
    render(<NotificationFeed kind="announcement" lang="zh" fetcher={fetcher} onChanged={changed} />);
    const open = await screen.findByRole('button', { name: '阅读全文' });
    expect(fetcher.mock.calls.filter(([, options]) => options?.method === 'POST')).toHaveLength(0);
    fireEvent.click(open);
    await waitFor(() => expect(changed).toHaveBeenCalledTimes(1));
    expect(fetcher).toHaveBeenCalledWith('/api/v1/user/notifications/ann_first/read', expect.objectContaining({ method: 'POST' }));
    expect(screen.getByRole('button', { name: '收起正文' })).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByText('已读')).toBeInTheDocument();
    expect(screen.getByText(/第二段 <img/)).toHaveClass('whitespace-pre-wrap');
    expect(document.querySelector('img')).toBeNull();
  });

  it('opens a personal message in a reading pane and persists its read state', async () => {
    const fetcher = fetchFeed([message]);
    render(<NotificationFeed kind="message" lang="zh" fetcher={fetcher} />);
    fireEvent.click(await screen.findByRole('button', { name: /账单更新/ }));
    const detail = await screen.findByRole('article', { name: '消息正文' });
    expect(within(detail).getByRole('heading', { name: '账单更新' })).toBeInTheDocument();
    expect(within(detail).getByText(/请查看最新明细/)).toHaveClass('whitespace-pre-wrap');
    await waitFor(() => expect(within(detail).getByText('已读')).toBeInTheDocument());
    expect(fetcher).toHaveBeenCalledWith('/api/v1/user/notifications/msg_first/read', expect.objectContaining({ method: 'POST' }));
  });

  it('preserves unread state when the server rejects a read at application level', async () => {
    const changed = vi.fn();
    render(<NotificationFeed kind="message" lang="zh" fetcher={fetchFeed([message], { code: 500, error: '写入失败' })} onChanged={changed} />);
    fireEvent.click(await screen.findByRole('button', { name: /账单更新/ }));
    expect(await screen.findByRole('alert')).toHaveTextContent('写入失败');
    expect(changed).not.toHaveBeenCalled();
    expect(within(screen.getByRole('article', { name: '消息正文' })).getByRole('button', { name: '标为已读' })).toBeEnabled();
  });

  it('marks only the current category through the received snapshot and supports unread filtering', async () => {
    const fetcher = fetchFeed([message]);
    render(<NotificationFeed kind="message" lang="zh" fetcher={fetcher} />);
    const markAll = await screen.findByRole('button', { name: '本栏全部已读' });
    await waitFor(() => expect(markAll).toBeEnabled());
    fireEvent.click(markAll);
    await waitFor(() => expect(fetcher.mock.calls.some(([url]) => url.endsWith('/read-all'))).toBe(true));
    const [, options] = fetcher.mock.calls.find(([url]) => url.endsWith('/read-all'));
    expect(JSON.parse(options.body)).toEqual({ kind: 'message', through_sequence: 18 });
    await waitFor(() => expect(screen.getByRole('checkbox', { name: '只看未读' })).toBeEnabled());
    fireEvent.click(screen.getByRole('checkbox', { name: '只看未读' }));
    await waitFor(() => expect(fetcher.mock.calls.some(([url]) => url.includes('kind=message') && url.includes('unread=true'))).toBe(true));
  });

  it('drops an earlier account response after switching accounts', async () => {
    let resolveAlice;
    const alice = vi.fn(() => new Promise(resolve => { resolveAlice = resolve; }));
    const { rerender } = render(<NotificationFeed key="alice" kind="message" lang="zh" fetcher={alice} />);
    rerender(<NotificationFeed key="bob" kind="message" lang="zh" fetcher={fetchFeed([])} />);
    await screen.findByText('暂无个人消息');
    await act(async () => resolveAlice(response({ code: 0, data: page([message]) })));
    expect(screen.queryByText('账单更新')).not.toBeInTheDocument();
  });

  it('shows an error and offers retry instead of pretending a failed initial load is empty', async () => {
    const fetcher = vi.fn().mockResolvedValueOnce(response({ error: '服务暂不可用' }, 503)).mockResolvedValue(response({ code: 0, data: page([]) }));
    render(<NotificationFeed kind="announcement" lang="zh" fetcher={fetcher} />);
    expect(await screen.findByRole('alert')).toHaveTextContent('服务暂不可用');
    expect(screen.queryByText('暂无全站公告')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: '重试' }));
    expect(await screen.findByText('暂无全站公告')).toBeInTheDocument();
  });

  it('shows the exact unread total in the bell accessible name and caps the visual badge', () => {
    const onClick = vi.fn();
    render(<NotificationBell counts={{ unread_count: 120 }} lang="zh" onClick={onClick} />);
    fireEvent.click(screen.getByRole('button', { name: '消息中心，120 条未读' }));
    expect(screen.getByText('99+')).toBeInTheDocument();
    expect(onClick).toHaveBeenCalledOnce();
  });
});

describe('private message composer', () => {
  it('reuses a delivery ID after a failed response and reports success only after acknowledgment', async () => {
    const fetcher = vi.fn().mockResolvedValueOnce(response({ code: 500, error: '请重试' })).mockResolvedValue(response({ code: 0 }));
    const sent = vi.fn();
    const close = vi.fn();
    render(<NotificationComposeModal onClose={close} onSent={sent} fetcher={fetcher} />);
    fireEvent.change(screen.getByLabelText('收件账号'), { target: { value: ' alice ' } });
    fireEvent.change(screen.getByLabelText('消息标题'), { target: { value: '账单说明' } });
    fireEvent.change(screen.getByLabelText('消息内容'), { target: { value: '仅本人可见' } });
    fireEvent.click(screen.getByRole('button', { name: '发送消息' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('请重试');
    expect(sent).not.toHaveBeenCalled();
    expect(close).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: '发送消息' }));
    await waitFor(() => expect(sent).toHaveBeenCalledOnce());
    expect(close).toHaveBeenCalledOnce();
    const bodies = fetcher.mock.calls.map(([, options]) => JSON.parse(options.body));
    expect(bodies[0].request_id).toBe(bodies[1].request_id);
    expect(bodies[0].recipient_username).toBe('alice');
  });

  it('guards typed content on Escape', () => {
    const close = vi.fn();
    render(<NotificationComposeModal onClose={close} fetcher={vi.fn()} />);
    fireEvent.change(screen.getByLabelText('消息标题'), { target: { value: '尚未发送' } });
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(screen.getByRole('alertdialog')).toBeInTheDocument();
    expect(close).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: '放弃并关闭' }));
    expect(close).toHaveBeenCalledOnce();
  });
});
