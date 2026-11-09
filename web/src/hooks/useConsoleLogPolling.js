import { useEffect, useRef } from 'react';

export default function useConsoleLogPolling({
  adminToken, currentTab, fetchData, fetchLogs, fetchRecentLogs, goToOffset, logFilters, logPageNumber,
  logPageSize, logs, restoringSession, setLogFilters, viewMode
}) {
  useEffect(() => {
    if (restoringSession || !adminToken || viewMode !== 'console') return;
    fetchData();
    fetchRecentLogs();
    const interval = setInterval(() => {
      fetchData();
      // The dashboard shows raw recent streaming logs, completely isolated from
      // any search or facet filters applied on the logs tab.
      if (currentTab === 'dashboard') {
        fetchRecentLogs();
      }
    }, 8000);
    return () => clearInterval(interval);
  }, [currentTab, adminToken, viewMode, fetchData, fetchRecentLogs, restoringSession]);

  // One owner for filter-driven loads on the logs tab. Debouncing also prevents
  // a click and several effects from issuing competing copies of the same query.
  //
  // The page cursor is URL state (`logs.page`). A filter change resets to page 1,
  // paging itself is handled by handleLogPageChange (which fetches directly), and
  // returning to the tab keeps the page the link points at — so the effect stays
  // silent when only the cursor moved.
  const logFilterSignatureRef = useRef('');
  const logsTabActiveRef = useRef(false);
  useEffect(() => {
    if (currentTab !== 'logs' || viewMode !== 'console') {
      logsTabActiveRef.current = false;
      return undefined;
    }
    if (restoringSession || !adminToken) return undefined;

    const signature = JSON.stringify([
      logFilters.q, logFilters.range, logFilters.start, logFilters.end,
      logFilters.session, logFilters.correlation_type, logFilters.key_id,
      logFilters.model, logFilters.channel, logFilters.status, logFilters.cache, logFilters.latency,
    ]);
    const enteringTab = !logsTabActiveRef.current;
    logsTabActiveRef.current = true;
    const firstRun = logFilterSignatureRef.current === '';
    const signatureChanged = firstRun || logFilterSignatureRef.current !== signature;
    logFilterSignatureRef.current = signature;
    if (!enteringTab && !signatureChanged) return undefined;

    let targetPage;
    if (firstRun) {
      // Deep link / restored URL: honour the page it names.
      targetPage = logPageNumber;
    } else if (signatureChanged) {
      // A different result set makes the old offset meaningless.
      targetPage = 1;
    } else {
      // Returning to the tab: keep the page currently in the URL.
      targetPage = logPageNumber;
    }
    const targetOffset = (targetPage - 1) * logPageSize;
    const timer = setTimeout(() => {
      if (signatureChanged && logPageNumber !== 1) setLogFilters({ page: '1' });
      goToOffset(targetOffset);
      fetchLogs({ offset: targetOffset });
    }, 350);
    return () => clearTimeout(timer);
  }, [logFilters, currentTab, viewMode, fetchLogs, goToOffset, restoringSession, adminToken, logPageNumber, logPageSize, setLogFilters]);

  // Server-side pagination for audit logs (offset-based, page size above).
  const handleLogPageChange = (newOffset) => {
    if (newOffset < 0) return;
    const nextPage = Math.floor(newOffset / logPageSize) + 1;
    goToOffset(newOffset);
    fetchLogs({ offset: newOffset });
    if (String(nextPage) !== logFilters.page) setLogFilters({ page: String(nextPage) });
  };

  // Batch Log Operations

  // Batch Key Operations

  // Batch Channel Operations

  // `overrideIds` lets a single row act on itself (ChannelsView passes
  // [ch.id]); without it the batch handler only looked at the checkbox
  // selection, so a single-row toggle either did nothing or changed a
  // different, unrelated set of channels.

  // Handle Key Modal Opening. Credentials are always generated server-side
  // with crypto/rand; the browser never invents an authentication secret.

  // Handle Key Creation

  // The server already applied keyword/facet filters, so the page is exactly the
  // result set. Re-filtering here would be wrong (it would hide rows the server
  // matched on fields the client does not check, e.g. error_message).
  const filteredLogs = logs;
  return {
    filteredLogs, handleLogPageChange
  };
}
