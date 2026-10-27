import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import LogDetailModal from './LogDetailModal';

function log(over = {}) {
  return {
    id: 1,
    trace_id: 'tr-1',
    chat_id: '',
    session_id: '',
    model: 'deepseek-v4.1-flash',
    channel: '罗总→gpustack-cluster',
    status_code: 503,
    prompt_tokens: 10,
    completion_tokens: 5,
    cached_tokens: 0,
    total_tokens: 15,
    cost: 0.01,
    duration_ms: 1200,
    ttft_ms: 300,
    created_at: '2026-10-22T08:00:00Z',
    error_message: 'upstream 罗总 returned status 503',
    usage_source: 'upstream',
    ...over,
  };
}

describe('LogDetailModal', () => {
  it('renders a failover channel chain as an ordered sequence', () => {
    render(<LogDetailModal log={log()} onClose={vi.fn()} onCopy={vi.fn()} lang="zh" />);

    // Both hops are shown separately instead of one opaque "A→B" string.
    expect(screen.getAllByText('罗总').length).toBeGreaterThan(0);
    expect(screen.getByText('gpustack-cluster')).toBeInTheDocument();
  });

  it('copies the raw upstream error text', () => {
    const onCopy = vi.fn();
    render(<LogDetailModal log={log()} onClose={vi.fn()} onCopy={onCopy} lang="zh" />);

    fireEvent.click(screen.getByRole('button', { name: /复制错误/ }));

    expect(onCopy).toHaveBeenCalledWith('upstream 罗总 returned status 503');
  });

  // 499 is a gateway-synthesized "client closed request" status: it must never
  // be attributed to the upstream provider, and must never suggest shifting
  // channel weights.
  it('attributes a 499 record to the client abort, not to the upstream', () => {
    render(
      <LogDetailModal
        log={log({
          status_code: 499,
          error_message: 'client closed request: downstream connection closed before the response completed',
        })}
        onClose={vi.fn()}
        onCopy={vi.fn()}
        lang="zh"
      />
    );

    expect(screen.getByText('499 客户端断开')).toBeInTheDocument();
    expect(
      screen.getByText('本次请求由客户端提前断开（网关合成状态码，非上游返回）')
    ).toBeInTheDocument();
    // The captured message is a gateway-side disconnect reason, so the detail
    // block must not be labelled as a raw upstream response.
    expect(screen.getByText('网关记录的断开原因（非上游回包）')).toBeInTheDocument();
    expect(screen.getByText(/客户端超时/)).toBeInTheDocument();
    // No upstream misattribution and no harmful "shift channel weights" advice.
    expect(screen.queryByText(/上游返回异常状态/)).not.toBeInTheDocument();
    expect(screen.queryByText(/上游服务商返回非预期状态码/)).not.toBeInTheDocument();
    expect(screen.queryByText(/权重调度/)).not.toBeInTheDocument();
  });

  it('explains that a 499 with output tokens was generated and billed by the upstream', () => {
    render(
      <LogDetailModal
        log={log({
          status_code: 499,
          completion_tokens: 24000,
          error_message: 'client closed request: downstream connection closed',
        })}
        onClose={vi.fn()}
        onCopy={vi.fn()}
        lang="zh"
      />
    );

    // The 24k-token incident shape: the provider had finished generating and
    // the usage was billed even though the caller had already timed out.
    const summary = screen.getByText(/24000 个输出 Token/);
    expect(summary.textContent).toContain('计费');
    expect(summary.textContent).toContain('并非上游故障');
    expect(summary.textContent).toContain('实际消耗');
  });

  it('marks a 499 whose token counts were estimated as an estimate, not as actual usage', () => {
    render(
      <LogDetailModal
        log={log({
          status_code: 499,
          completion_tokens: 24000,
          usage_source: 'estimated',
          error_message: 'client closed request: downstream connection closed',
        })}
        onClose={vi.fn()}
        onCopy={vi.fn()}
        lang="zh"
      />
    );

    // The provider never reported usage, so the panel must say "estimated"
    // instead of claiming the recorded tokens were actually consumed.
    const summary = screen.getByText(/24000 个输出 Token/);
    expect(summary.textContent).toContain('估算');
    expect(summary.textContent).not.toContain('实际消耗');
  });

  it('does not render the status-code speculation disclaimer for a 499 without an error message', () => {
    render(
      <LogDetailModal
        log={log({ status_code: 499, error_message: '' })}
        onClose={vi.fn()}
        onCopy={vi.fn()}
        lang="zh"
      />
    );

    expect(screen.queryByText(/基于状态码的推测/)).not.toBeInTheDocument();
    expect(
      screen.getByText(/该状态码由网关在下游连接断开时生成，不是上游服务商返回的状态码/)
    ).toBeInTheDocument();
    expect(screen.getByText('499 客户端断开')).toBeInTheDocument();
    expect(screen.queryByText(/权重调度/)).not.toBeInTheDocument();
  });

  // Regression guard: real upstream statuses keep the upstream-oriented
  // diagnosis and the raw-upstream wording.
  it('keeps the upstream-oriented diagnosis for a non-499 error status', () => {
    render(<LogDetailModal log={log()} onClose={vi.fn()} onCopy={vi.fn()} lang="zh" />);

    expect(screen.getByText('上游异常原始回包')).toBeInTheDocument();
    expect(screen.getByText(/可能原因: 上游超时或网络不可达/)).toBeInTheDocument();
    expect(screen.queryByText('499 客户端断开')).not.toBeInTheDocument();
  });

  it('renders the English client-abort copy for a 499 record', () => {
    render(
      <LogDetailModal
        log={log({ status_code: 499, completion_tokens: 24000, error_message: 'client closed request' })}
        onClose={vi.fn()}
        onCopy={vi.fn()}
        lang="en"
      />
    );

    expect(screen.getByText('499 Client Closed')).toBeInTheDocument();
    expect(
      screen.getByText(
        'Request aborted by the client (gateway-synthesized status, not an upstream response)'
      )
    ).toBeInTheDocument();
    expect(
      screen.getByText('Gateway-recorded disconnect reason (not a raw upstream response)')
    ).toBeInTheDocument();
    expect(screen.queryByText(/Shift traffic weight/)).not.toBeInTheDocument();
  });
});
