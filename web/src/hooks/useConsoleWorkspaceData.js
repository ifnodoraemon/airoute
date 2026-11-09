import { useConsoleData } from './useConsoleData';
import { useLogsData } from './useLogsData';
import { useChannelActions } from './useChannelActions';
import { useKeyActions } from './useKeyActions';
import useConsoleLogFilters from './useConsoleLogFilters';

export default function useConsoleWorkspaceData({ adminUser, isZh, showToast }) {
  const { refresh: fetchData, reset: resetConsoleData, ...data } = useConsoleData({ adminUser });
  const filters = useConsoleLogFilters();
  const { reset: resetLogsData, ...logs } = useLogsData({ filters: filters.logFilters, showToast, isZh });
  const { reset: resetChannelActions, ...channels } = useChannelActions({
    channels: data.channels, showToast, isZh, refresh: fetchData,
  });
  const { reset: resetKeyActions, ...keys } = useKeyActions({
    keys: data.keys, adminUser, showToast, isZh, refresh: fetchData,
  });
  return {
    ...data, ...filters, ...logs, ...channels, ...keys,
    fetchData, resetConsoleData, resetLogsData, resetChannelActions, resetKeyActions,
  };
}
