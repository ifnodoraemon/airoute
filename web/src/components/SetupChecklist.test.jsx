import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import SetupChecklist from './SetupChecklist';

const ready = {
  channels: [{ status: 'active', breaker_status: 'CLOSED' }],
  keys: [{ status: 'active', budget: 10, used_cost: 1 }],
  models: ['test-model'],
};

describe('SetupChecklist', () => {
  it('links each unfinished setup step to its console workflow', () => {
    const setCurrentTab = vi.fn();
    const setShowChannelModal = vi.fn();
    const onOpenOnboarding = vi.fn();
    render(<SetupChecklist isZh setCurrentTab={setCurrentTab} setShowChannelModal={setShowChannelModal} onOpenOnboarding={onOpenOnboarding} />);
    expect(screen.getByText('0/4 项已完成')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: '接入服务商' }));
    expect(setShowChannelModal).toHaveBeenCalledWith(true);
    fireEvent.click(screen.getByRole('button', { name: '查看模型路由' }));
    expect(setCurrentTab).toHaveBeenCalledWith('models');
    fireEvent.click(screen.getByRole('button', { name: '管理密钥' }));
    expect(setCurrentTab).toHaveBeenCalledWith('keys');
    fireEvent.click(screen.getByRole('button', { name: '测试握手' }));
    expect(onOpenOnboarding).toHaveBeenCalled();
  });

  it('does not count failed requests as a completed handshake and links error logs', () => {
    const onNavigateToErrors = vi.fn();
    render(<SetupChecklist {...ready} isZh stats={{ total_requests: 8, success_requests: 0 }} attention={{ failed: 8 }} onNavigateToErrors={onNavigateToErrors} />);
    expect(screen.getByText('3/4 项已完成')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: '查看异常日志' }));
    expect(onNavigateToErrors).toHaveBeenCalled();
    expect(screen.getByRole('button', { name: '测试握手' })).toBeInTheDocument();
  });

  it('requires a routable channel and a key with remaining budget', () => {
    render(<SetupChecklist {...ready} channels={[{ status: 'active', breaker_status: 'OPEN' }]} keys={[{ status: 'active', budget: 10, used_cost: 10 }]} attention={{ tripped: 1 }} isZh />);
    expect(screen.getByText('0/4 项已完成')).toBeInTheDocument();
    expect(screen.getByText('渠道熔断中，请先检查渠道健康状态。')).toBeInTheDocument();
  });

  it('disappears after all steps have evidence of completion', () => {
    render(<SetupChecklist {...ready} stats={{ success_requests: 1 }} isZh />);
    expect(screen.queryByRole('region', { name: '开通检查清单' })).not.toBeInTheDocument();
  });
});
