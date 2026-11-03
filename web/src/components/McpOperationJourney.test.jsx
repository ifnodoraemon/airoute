import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import McpIntegrationView from './McpIntegrationView';

const server = { id: 'python-service', name: 'Python service', category: 'dev', transport: 'stdio', endpoint: 'uvx service', env_vars: '{"TOKEN":"configured"}', enabled: false, tools: ['echo'], prompts: [], resources: [] };
const response = data => ({ ok: true, status: 200, json: async () => ({ code: 0, data }) });
const adminFetch = () => vi.fn(async (url, opts) => response(opts?.method === 'POST' ? server : url.endsWith('/settings') ? { mcp_enabled: true } : [server]));
afterEach(() => { vi.unstubAllGlobals(); });

describe('MCP operation journeys', () => {
  it('edits a service with its original ID and disabled state intact', async () => {
    const fetcher = adminFetch();
    render(<McpIntegrationView adminFetch={fetcher} />);
    fireEvent.click(await screen.findByTitle('编辑服务'));
    const endpoint = screen.getByDisplayValue('uvx service');
    fireEvent.change(endpoint, { target: { value: 'python "folder with spaces/service.py"' } });
    fireEvent.click(screen.getByRole('button', { name: '保存服务' }));
    await waitFor(() => expect(fetcher.mock.calls.some(([, options]) => options?.method === 'POST')).toBe(true));
    const body = JSON.parse(fetcher.mock.calls.find(([, opts]) => opts?.method === 'POST')[1].body);
    expect(body).toMatchObject({ id: server.id, enabled: false, tools: ['echo'], env_vars: server.env_vars, endpoint: 'python "folder with spaces/service.py"' });
  });

  it.each([
    [401, { error: { message: 'invalid key' } }],
    [200, { jsonrpc: '2.0', id: 1, result: { isError: true, content: [{ type: 'text', text: 'tool failed' }] } }],
  ])('reports HTTP %i / tool failures without claiming success', async (status, body) => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: status === 200, status, json: async () => body })));
    render(<McpIntegrationView adminFetch={adminFetch()} />);
    fireEvent.click(await screen.findByRole('button', { name: '调试探针' }));
    fireEvent.click(screen.getByRole('button', { name: '发送 JSON-RPC 2.0 报文' }));
    expect(await screen.findByText('状态: 调用失败')).toBeInTheDocument();
    expect(screen.getByText(`接收响应 (HTTP ${status})`)).toBeInTheDocument();
    expect(screen.queryByText('状态: 成功返回')).not.toBeInTheDocument();
  });
});
