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
});
