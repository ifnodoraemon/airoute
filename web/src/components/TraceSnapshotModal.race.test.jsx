// Regression test for the trace-snapshot search race.
//
// Started as the BUG-WEB-04 probe; now asserts the CORRECT behaviour (it fails on
// the pre-fix code, which had no request-sequence guard).
//
// Runner:
//   cd web && npx vitest run --configLoader runner src/components/TraceSnapshotModal.race.test.jsx

import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import TraceSnapshotModal from './TraceSnapshotModal';

function deferred() {
  let resolve;
  const promise = new Promise((res) => { resolve = res; });
  return { promise, resolve };
}

function jsonResponse(body, status = 200) {
  return { ok: status < 400, status, json: async () => body };
}

describe('TraceSnapshotModal search race', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('ignores a late response for a query that has been superseded', async () => {
    const first = deferred(); // mount query (trace A) — slow
    const second = deferred(); // second query (chat B) — fast

    let call = 0;
    const adminFetch = vi.fn(() => {
      call += 1;
      return call === 1 ? first.promise : second.promise;
    });

    render(
      <TraceSnapshotModal
        isOpen
        traceId="tr-slow-A"
        initialQueryField="trace_id"
        onClose={() => {}}
        lang="zh"
        adminFetch={adminFetch}
      />,
    );

    // Request #1 (the modal's own trace) is still in the air.
    await waitFor(() => expect(adminFetch).toHaveBeenCalledTimes(1));
    expect(String(adminFetch.mock.calls[0][0])).toContain('trace_id=tr-slow-A');

    // The operator searches a second correlation id and its answer arrives first.
    fireEvent.change(screen.getByPlaceholderText('输入关联 ID 后回车查询'), { target: { value: 'chat-B' } });
    fireEvent.click(screen.getByRole('button', { name: '查询' }));

    await waitFor(() => expect(adminFetch).toHaveBeenCalledTimes(2));
    expect(String(adminFetch.mock.calls[1][0])).toContain('trace_id=chat-B');

    second.resolve(jsonResponse({ code: 0, data: [{ id: 2, model: 'model-from-B', status_code: 200, duration_ms: 10 }] }));
    await waitFor(() => expect(screen.getByText('model-from-B')).toBeInTheDocument());

    // The stale response for trace A lands afterwards and must be discarded.
    first.resolve(jsonResponse({ code: 0, data: [{ id: 1, model: 'model-from-A', status_code: 500, duration_ms: 900 }] }));
    await waitFor(() => expect(adminFetch).toHaveBeenCalledTimes(2));
    // Give the discarded promise a turn to settle, then assert the newer rows won.
    await Promise.resolve();

    expect(screen.getByText('model-from-B')).toBeInTheDocument();
    expect(screen.queryByText('model-from-A')).toBeNull();
    expect(screen.getByDisplayValue('chat-B')).toBeInTheDocument();
  });

  it('aborts the in-flight request when the modal closes', async () => {
    const signals = [];
    const pending = deferred();
    const adminFetch = vi.fn((url, options) => {
      signals.push(options?.signal);
      return pending.promise;
    });

    const { rerender } = render(
      <TraceSnapshotModal isOpen traceId="tr-close" onClose={() => {}} lang="zh" adminFetch={adminFetch} />,
    );
    await waitFor(() => expect(adminFetch).toHaveBeenCalledTimes(1));
    expect(signals[0]).toBeDefined();
    expect(signals[0].aborted).toBe(false);

    rerender(
      <TraceSnapshotModal isOpen={false} traceId="tr-close" onClose={() => {}} lang="zh" adminFetch={adminFetch} />,
    );

    expect(signals[0].aborted).toBe(true);
  });
});
