import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import LogsView from './LogsView';

// LogsView is the audit table. Two things here are worth pinning: the filters are
// applied by the server, facets cover the whole history, and the pagination guard
// has to work whether or not the gateway reported a total.

function log(over = {}) {
  return { id: 1, model: 'gpt-6', channel: 'c1', status_code: 200, cached_tokens: 0, ttft_ms: 100, ...over };
}

function setup(props = {}) {
  const handlers = {
    setSessionFilter: vi.fn(),
    setLogFilter: vi.fn(),
    setLogFilters: vi.fn(),
    fetchLogs: vi.fn(),
    handleClearLogs: vi.fn(),
    setTimeRange: vi.fn(),
    setCustomStartTime: vi.fn(),
    setCustomEndTime: vi.fn(),
    setSelectedLogIds: vi.fn(),
    handleBatchDeleteLogs: vi.fn(),
    onExportAll: vi.fn(),
    onLogPageChange: vi.fn(),
    setActiveLogDetail: vi.fn(),
    showToast: vi.fn(),
    handleDeleteSingleLog: vi.fn(),
  };
  const utils = render(
    <LogsView
      sessionFilter=""
      logFilter=""
      logFilters={{ q: '', range: 'all', start: '', end: '', session: '', model: 'all', channel: 'all', status: 'all', cache: 'all', latency: 'all' }}
      logLoading={false}
      selectedLogIds={[]}
      filteredLogs={[]}
      logsLength={0}
      isAdmin
      lang="zh"
      {...handlers}
      {...props}
    />
  );
  return { ...utils, ...handlers };
}

describe('LogsView', () => {
  it('renders an empty table without crashing', () => {
    setup();

    expect(screen.getByText(/本页 0 条/)).toBeInTheDocument();
  });

  it('counts the rows on the page', () => {
    setup({ filteredLogs: [log({ id: 1 }), log({ id: 2 })], logsLength: 2 });

    expect(screen.getByText(/本页 2 条/)).toBeInTheDocument();
  });

  it('mentions the server-side total when the gateway reported one', () => {
    // The filters run on the server, so "2 on this page" alone would hide that
    // 500 rows matched. The count sits in the collapsible filter panel.
    setup({ filteredLogs: [log({ id: 1 }), log({ id: 2 })], logsLength: 2, logTotal: 500 });
    fireEvent.click(screen.getByRole('button', { name: /更多筛选/ }));

    expect(screen.getByText(/全库匹配 500 条/)).toBeInTheDocument();
  });

  describe('pagination', () => {
    it('reports the current page number from the offset', () => {
      setup({ logOffset: 100, logPageSize: 50, logTotal: 500 });

      expect(screen.getByText(/第 3 页/)).toBeInTheDocument();
    });

    it('disables the previous button on the first page', () => {
      setup({ logOffset: 0, logPageSize: 50, logTotal: 500 });

      expect(screen.getByRole('button', { name: /上一页|Prev/ })).toBeDisabled();
    });

    it('enables the previous button past the first page', () => {
      setup({ logOffset: 50, logPageSize: 50, logTotal: 500 });

      expect(screen.getByRole('button', { name: /上一页|Prev/ })).not.toBeDisabled();
    });

    it('steps back by exactly one page', () => {
      const { onLogPageChange } = setup({ logOffset: 100, logPageSize: 50, logTotal: 500 });

      fireEvent.click(screen.getByRole('button', { name: /上一页|Prev/ }));

      expect(onLogPageChange).toHaveBeenCalledWith(50);
    });

    it('steps forward by exactly one page', () => {
      const { onLogPageChange } = setup({ logOffset: 0, logPageSize: 50, logTotal: 500 });

      fireEvent.click(screen.getByRole('button', { name: /下一页|Next/ }));

      expect(onLogPageChange).toHaveBeenCalledWith(50);
    });

    it('disables the next button on the last page', () => {
      // offset + pageSize >= total, so there is nothing left to fetch.
      setup({ logOffset: 450, logPageSize: 50, logTotal: 500 });

      expect(screen.getByRole('button', { name: /下一页|Next/ })).toBeDisabled();
    });

    it('falls back to the page length when the gateway reported no total', () => {
      // A short page means the end of the result set; a full one does not.
      setup({ logOffset: 0, logPageSize: 50, logTotal: 0, logsLength: 10 });

      expect(screen.getByRole('button', { name: /下一页|Next/ })).toBeDisabled();
    });

    it('keeps the next button enabled on a full page with no total', () => {
      setup({ logOffset: 0, logPageSize: 50, logTotal: 0, logsLength: 50 });

      expect(screen.getByRole('button', { name: /下一页|Next/ })).not.toBeDisabled();
    });

    it('disables pagination while a load is in flight', () => {
      setup({ logOffset: 50, logPageSize: 50, logTotal: 500, logLoading: true });

      expect(screen.getByRole('button', { name: /下一页|Next/ })).toBeDisabled();
    });
  });

  it('renders the server page without applying a conflicting latency filter', () => {
    setup({
      filteredLogs: [log({ duration_ms: 800 })], logsLength: 1,
      logFilters: { model: 'all', channel: 'all', status: 'all', cache: 'all', latency: 'fast' },
    });
    expect(screen.getByText(/本页 1 条/)).toBeInTheDocument();
  });

  it('offers global models and channels even when the current page is empty', () => {
    setup({ filteredLogs: [], logFacets: { models: ['gpt-6', 'claude-5'], channels: ['old-gpustack', 'new-channel'] } });
    fireEvent.click(screen.getByRole('button', { name: /更多筛选/ }));
    const options = screen.getAllByRole('option').map((o) => o.textContent);
    expect(options).toEqual(expect.arrayContaining(['gpt-6', 'claude-5', 'old-gpustack', 'new-channel']));
  });

  it('keeps a selected channel visible when no logs match', () => {
    setup({ logFilters: { model: 'all', channel: 'deleted-provider', status: 'error', cache: 'all', latency: 'all' } });
    fireEvent.click(screen.getByRole('button', { name: /更多筛选/ }));
    expect(screen.getByRole('option', { name: 'deleted-provider' }).selected).toBe(true);
  });

  it('keeps the filter panel collapsed until it is asked for', () => {
    setup({ filteredLogs: [log({ id: 1, model: 'gpt-6' })], logsLength: 1 });

    // The facets are behind a disclosure; they are not rendered up front.
    expect(screen.queryByRole('option')).not.toBeInTheDocument();
  });

  it('renders in English when asked', () => {
    setup({ lang: 'en', filteredLogs: [log({ id: 1 })], logsLength: 1, logTotal: 500 });
    fireEvent.click(screen.getByRole('button', { name: /More filters/ }));

    expect(screen.getByText(/1 on this page/)).toBeInTheDocument();
    expect(screen.getByText(/500 matching in total/)).toBeInTheDocument();
  });
});
