import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react';
import TraceSnapshotModal from './TraceSnapshotModal';
import LogDetailModal from './LogDetailModal';
import OnboardingModal from './OnboardingModal';
import WalletManagementView from './WalletManagementView';
import CommandPalette from './CommandPalette';
import { availableKeyModels } from '../lib/keyModels';

const response = data => ({ ok: true, status: 200, json: async () => ({ code: 0, data }) });
const user = { username: 'alice', role: 'user', balance: 10 };
const key = { id: 1, key_id: 'ak_1f0013dea24034ac0001', status: 'active', allowed_models: ['chat-b'] };

beforeEach(() => { window.history.replaceState(null, '', '/'); });
afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers(); });

describe('functional journey regressions', () => {
  it('preserves the correlation field when opening and refreshing a trace', async () => {
    const adminFetch = vi.fn(async () => response([]));
    render(<TraceSnapshotModal isOpen traceId="chat-123" initialQueryField="chat_id" adminFetch={adminFetch} onClose={vi.fn()} />);
    await waitFor(() => expect(adminFetch).toHaveBeenCalledTimes(1));
    expect(adminFetch.mock.calls[0][0]).toContain('chat_id=chat-123');
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'session_id' } });
    fireEvent.change(screen.getByRole('textbox'), { target: { value: 'session-456' } });
    fireEvent.click(screen.getByTitle('刷新'));
    await waitFor(() => expect(adminFetch).toHaveBeenCalledTimes(2));
    expect(adminFetch.mock.calls[1][0]).toContain('session_id=session-456');
    expect(adminFetch.mock.calls[1][0]).not.toContain('object');
  });

  it('explains historical charges with the saved rate and all billable units', () => {
    const onCopy = vi.fn();
    render(<LogDetailModal log={{ model: 'images', api_key: 'sk-***', cost: 16, status_code: 200, pricing_snapshot: { fixed_price: 10, group_multiplier: 0.8, billable_units: 2 } }} prices={[{model:'images',fixed_price:99}]} onClose={vi.fn()} onCopy={onCopy} />);
    expect(screen.getByText(/固定 ¥10 × 2/)).toBeInTheDocument();
    expect(screen.queryByText(/¥99/)).not.toBeInTheDocument();
    fireEvent.click(screen.getByText('复制聊天 cURL 示例'));
    expect(onCopy.mock.calls[0][0]).toContain('Bearer YOUR_API_KEY');
    expect(onCopy.mock.calls[0][0]).not.toContain('sk-***');
    expect(screen.queryByText(/删除日志/)).not.toBeInTheDocument();
  });

  it('does not fabricate a historical formula for older records', () => {
    const fetchMock = vi.fn(); vi.stubGlobal('fetch', fetchMock);
    render(<LogDetailModal log={{ model:'chat-b', cost:1, status_code:200 }} onClose={vi.fn()} onCopy={vi.fn()} />);
    expect(screen.getByText(/没有历史费率快照/)).toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('blocks the first call until an active key exists', () => {
    const onCreateKey=vi.fn();
    render(<OnboardingModal isOpen keys={[{...key,status:'disabled'}]} models={['chat-b']} onClose={vi.fn()} onCreateKey={onCreateKey} />);
    expect(screen.getByRole('button',{name:/下一步/})).toBeDisabled();
    expect(screen.queryByText(/sk-airoute-demo-key/)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button',{name:'创建 API 密钥'}));
    expect(onCreateKey).toHaveBeenCalled();
  });

  it.each([
    ['Claude Messages','/v1/messages','x-api-key',{ content:[{text:'hello'}] }],
    ['Google Gemini','/v1beta/models/chat-b:generateContent','x-goog-api-key',{ candidates:[{content:{parts:[{text:'hello'}]}}] }],
  ])('tests the selected %s protocol using an allowed model', async (label,path,header,body) => {
    // The list only carries a key ID now, so the probe can only work with the
    // plaintext the operator pastes for the session.
    const sessionKey = 'sk-airoute-onboarding-0123456789';
    const fetchMock=vi.fn(async()=>({ok:true,json:async()=>body}));vi.stubGlobal('fetch',fetchMock);
    render(<OnboardingModal isOpen keys={[key]} models={['chat-a','chat-b']} onClose={vi.fn()} showToast={vi.fn()} />);
    fireEvent.change(screen.getByLabelText('会话 API Key 明文'), { target: { value: sessionKey } });
    fireEvent.click(screen.getByRole('button',{name:/下一步：选择/}));
    expect(screen.getByRole('combobox')).toHaveValue('chat-b');
    expect(screen.queryByRole('option',{name:'chat-a'})).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button',{name:new RegExp(label)}));
    fireEvent.click(screen.getByRole('button',{name:/下一步：代码/}));
    fireEvent.click(screen.getByRole('button',{name:'立即测试握手'}));
    await waitFor(()=>expect(fetchMock).toHaveBeenCalledOnce());
    expect(fetchMock.mock.calls[0][0]).toContain(path);
    // The probe authenticates with the pasted plaintext, never the listed key ID.
    expect(fetchMock.mock.calls[0][1].headers[header]).toBe(sessionKey);
    expect(fetchMock.mock.calls[0][1].headers[header]).not.toBe(key.key_id);
    expect(JSON.stringify(fetchMock.mock.calls[0][1].headers)).not.toContain('***');
    expect(await screen.findByText('hello')).toBeInTheDocument();
  });

  it('disables checkout when the payment channel is not ready', async () => {
    const adminFetch=vi.fn(async()=>response({...user,stripe_enabled:false,orders:[]}));
    render(<WalletManagementView adminUser={user} adminFetch={adminFetch} showToast={vi.fn()} />);
    await screen.findByText(/在线支付尚未开通/);
    expect(screen.getByRole('button',{name:/立即支付/})).toBeDisabled();
    expect(adminFetch.mock.calls.some(([url])=>url.includes('recharge/stripe'))).toBe(false);
  });

  it('waits for the server to confirm payment before refreshing the balance', async () => {
    vi.useFakeTimers();
    window.history.replaceState(null,'','/?tab=wallet&recharge=success&order_no=REC-test');
    let polls=0;
    const adminFetch=vi.fn(async url=> url.includes('/orders/') ? response({status:++polls===1?'pending':'paid'}) : response({...user,balance:polls>1?29.99:10,stripe_enabled:true,orders:[]}));
    const onBalanceUpdate=vi.fn();
    render(<WalletManagementView adminUser={user} adminFetch={adminFetch} showToast={vi.fn()} onBalanceUpdate={onBalanceUpdate} />);
    await act(async()=>{await vi.advanceTimersByTimeAsync(0);});
    expect(screen.getByText(/正在确认支付结果/)).toBeInTheDocument();
    expect(onBalanceUpdate).not.toHaveBeenCalledWith(29.99);
    await act(async()=>{await vi.advanceTimersByTimeAsync(2000);});
    expect(screen.getByText(/充值已到账/)).toBeInTheDocument();
    expect(onBalanceUpdate).toHaveBeenCalledWith(29.99);
    expect(window.location.search).not.toContain('recharge');
    await act(async()=>{await vi.advanceTimersByTimeAsync(10000);});
    expect(polls).toBe(2);
  });

  it('finds models beyond the first 15 and passes the selected model to the playground', () => {
    const onSelectModel=vi.fn();
    render(<CommandPalette isOpen models={Array.from({length:25},(_,i)=>`chat-${i}`)} onSelectModel={onSelectModel} onClose={vi.fn()} setCurrentTab={vi.fn()} setViewMode={vi.fn()} lang="zh" />);
    fireEvent.change(screen.getByRole('textbox'),{target:{value:'chat-24'}});
    fireEvent.click(screen.getByText('chat-24'));
    expect(onSelectModel).toHaveBeenCalledWith('chat-24');
  });

  it('matches key access without adding unavailable or disabled models', () => {
    expect(availableKeyModels({...key,allowed_models:['namespace/*']},['namespace/chat','other/chat'])).toEqual(['namespace/chat']);
    expect(availableKeyModels({...key,status:'disabled'},['chat-b'])).toEqual([]);
    expect(availableKeyModels({...key,allowed_models:['missing']},['chat-b'])).toEqual([]);
  });
});
