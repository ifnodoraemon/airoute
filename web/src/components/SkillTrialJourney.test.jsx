import { useState } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import SkillHubView from './SkillHubView';
import PlaygroundView from './PlaygroundView';

const skill = { id: 'writing', name: 'Writing SOP', description: 'Write clearly', category: 'dev', enabled: true, tools: [], manifest: '# SOP\nUse plain language.' };
const response = data => ({ ok: true, status: 200, json: async () => ({ code: 0, data }) });
afterEach(() => { vi.unstubAllGlobals(); });

describe('skill trial journey', () => {
  it('sends the saved SOP and task through the real playground request path', async () => {
    const adminFetch = vi.fn(async url => response(String(url).endsWith('/skills') ? [skill] : {}));
    const fetch = vi.fn(async () => ({ ok: true, json: async () => ({ choices: [{ message: { content: 'Actual model response' } }] }) }));
    vi.stubGlobal('fetch', fetch);
    function Journey() {
      const [scenario, setScenario] = useState(null);
      return scenario ? <PlaygroundView models={['chat-trial']} initialScenario={scenario} /> : <SkillHubView adminFetch={adminFetch} onTrial={setScenario} />;
    }
    render(<Journey />);
    fireEvent.click(await screen.findByRole('button', { name: '模型实测' }));
    expect(fetch).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText('测试任务'), { target: { value: 'Write an example.' } });
    fireEvent.click(screen.getByRole('button', { name: '前往 Playground 实测' }));
    fireEvent.click(screen.getByRole('button', { name: '发送请求' }));
    await waitFor(() => expect(fetch).toHaveBeenCalledOnce());
    const body = JSON.parse(fetch.mock.calls[0][1].body);
    expect(body.messages).toEqual([{ role: 'system', content: skill.manifest }, { role: 'user', content: 'Write an example.' }]);
    expect(await screen.findByText('Actual model response')).toBeInTheDocument();
    expect(screen.queryByText(/L3_EXECUTION_COMPLETED|成功模拟执行/)).not.toBeInTheDocument();
  });
});
