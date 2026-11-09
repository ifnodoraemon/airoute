import useUrlFilters from './useUrlFilters';
import { LOG_PAGE_SIZE } from './useLogsData';

const DEFAULT_FILTERS = {
  q: '', range: 'all', start: '', end: '', session: '', correlation_type: 'session_id',
  key_id: '', model: 'all', channel: 'all', status: 'all', cache: 'all', latency: 'all', page: '1',
};

export default function useConsoleLogFilters() {
  const [logFilters, setLogFilters] = useUrlFilters('logs', DEFAULT_FILTERS);
  return {
    logFilters, setLogFilters,
    logPageNumber: Math.max(1, parseInt(logFilters.page, 10) || 1),
    logPageSize: LOG_PAGE_SIZE,
    logFilter: logFilters.q,
    setLogFilter: (value) => setLogFilters({ q: value }),
    timeRange: logFilters.range,
    setTimeRange: (value) => setLogFilters({ range: value }),
    customStartTime: logFilters.start,
    setCustomStartTime: (value) => setLogFilters({ start: value }),
    customEndTime: logFilters.end,
    setCustomEndTime: (value) => setLogFilters({ end: value }),
    sessionFilter: logFilters.session,
    setSessionFilter: (value, type = 'session_id') => setLogFilters({ session: value, correlation_type: type }),
  };
}
