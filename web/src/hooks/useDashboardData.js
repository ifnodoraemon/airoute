import { useState, useEffect, useCallback, useRef } from 'react';
import { apiFetch } from '../api/client';

export default function useDashboardData({
  DASHBOARD_REFRESH_MS, LIVE_POLL_MS, adminFetch, onSelectKey, propSelectedKey
}) {
  const [chartMetric, setChartMetric] = useState('requests'); // 'requests' | 'tokens' | 'latency' | 'errors' | 'rpm' | 'tpm'
  const [timeframe, setTimeframe] = useState('24h'); // '1h' | '24h' | '7d'
  const [hoveredBucket, setHoveredBucket] = useState(null);
  const [refreshing, setRefreshing] = useState(false);
  const [lastRefreshed, setLastRefreshed] = useState(null);
  const [timeseriesData, setTimeseriesData] = useState(null);
  const [selectedKey, setSelectedKey] = useState(propSelectedKey || '');
  const [keyStats, setKeyStats] = useState(null);
  const [dismissedAttentions, setDismissedAttentions] = useState(() => {
    try {
      return JSON.parse(localStorage.getItem('airoute_dismissed_attention') || '[]');
    } catch {
      return [];
    }
  });

  const handleDismissAttention = (itemKey) => {
    setDismissedAttentions(prev => {
      const next = [...new Set([...prev, itemKey])];
      try { localStorage.setItem('airoute_dismissed_attention', JSON.stringify(next)); } catch {}
      return next;
    });
  };

  const [recentLogs, setRecentLogs] = useState([]);
  const [recentLogsLoading, setRecentLogsLoading] = useState(false);
  const [liveStats, setLiveStats] = useState(null);

  const dashboardRequest = useRef(0);
  const [statsKey, setStatsKey] = useState(null);
  const fetchDashboard = useCallback(async () => {
    const request = ++dashboardRequest.current;
    const fetcher = adminFetch || apiFetch;
    // `selectedKey` holds the numeric key id (string form). The public key ID
    // (`ak_…`) the list renders is not a filter key: logs and stats resolve the
    // row id server-side through this `key_id` query parameter.
    const suffix = selectedKey ? `&key_id=${encodeURIComponent(selectedKey)}` : '';
    const read = async (url) => {
      const res = await fetcher(url);
      const json = await res.json();
      if (!res.ok || json.code !== 0) throw new Error(json.error || 'Dashboard request failed');
      return json.data;
    };
    setRecentLogsLoading(true);
    const results = await Promise.allSettled([
      read(`/api/v1/admin/logs?limit=10${suffix}`),
      read(`/api/v1/admin/stats/timeseries?timeframe=${timeframe}${suffix}`),
      selectedKey ? read(`/api/v1/admin/stats/overview?key_id=${encodeURIComponent(selectedKey)}`) : Promise.resolve(null),
    ]);
    if (request !== dashboardRequest.current) return false;
    setRecentLogs(results[0].status === 'fulfilled' && Array.isArray(results[0].value) ? results[0].value : []);
    setTimeseriesData(results[1].status === 'fulfilled' ? results[1].value : null);
    setKeyStats(results[2].status === 'fulfilled' ? results[2].value : null);
    setStatsKey(selectedKey);
    setRecentLogsLoading(false);
    return results.every(r => r.status === 'fulfilled');
  }, [selectedKey, timeframe, adminFetch]);

  const handleSelectKeyDrilldown = (key) => {
    setSelectedKey(key);
    onSelectKey?.(key);
  };
  useEffect(() => {
    if (propSelectedKey !== undefined) setSelectedKey(propSelectedKey || '');
  }, [propSelectedKey]);
  useEffect(() => {
    const requestRef = dashboardRequest;
    setKeyStats(null);
    setTimeseriesData(null);
    setRecentLogs([]);
    fetchDashboard();
    const timer = setInterval(() => {
      if (!document.hidden) fetchDashboard();
    }, DASHBOARD_REFRESH_MS);
    return () => { ++requestRef.current; clearInterval(timer); };
  }, [fetchDashboard, DASHBOARD_REFRESH_MS]);

  // Real-time gateway telemetry (in-flight requests and streams).
  //
  // The 60s window only sees completed requests, so during a long stream the
  // speedometers read zero while the gateway is in fact busy. The live endpoint
  // is purely in-memory and reports what is happening right now. It is optional:
  // when it is missing (older gateway) or fails, the dashboard silently keeps
  // its previous 60s-window behaviour — no toast, no error state, no blocking of
  // the first paint.
  useEffect(() => {
    let mounted = true;
    let timer = null;
    let inFlight = false;
    const fetcher = adminFetch || apiFetch;

    const load = async () => {
      // Never stack requests: on a slow gateway the 2s tick would otherwise pile
      // up and resolve out of order.
      if (inFlight) return;
      inFlight = true;
      try {
        const res = await fetcher('/api/v1/admin/stats/live');
        const json = res && res.ok ? await res.json() : null;
        if (!mounted) return;
        const data = json && json.code === 0 ? json.data : null;
        setLiveStats(data && typeof data === 'object' && !Array.isArray(data) ? data : null);
      } catch {
        // Endpoint absent or gateway unreachable: degrade to the 60s window.
        if (mounted) setLiveStats(null);
      } finally {
        inFlight = false;
      }
    };

    const stop = () => {
      if (timer !== null) {
        clearInterval(timer);
        timer = null;
      }
    };
    const start = () => {
      if (timer === null) timer = setInterval(load, LIVE_POLL_MS);
    };
    // A hidden tab has nobody watching it; polling there only burns requests.
    // Coming back visible catches up immediately instead of waiting a tick.
    const onVisibilityChange = () => {
      if (document.hidden) {
        stop();
        return;
      }
      start();
      load();
    };

    load();
    if (!document.hidden) start();
    document.addEventListener('visibilitychange', onVisibilityChange);

    return () => {
      mounted = false;
      stop();
      document.removeEventListener('visibilitychange', onVisibilityChange);
    };
  }, [adminFetch, LIVE_POLL_MS]);
  return {
    chartMetric, dismissedAttentions, fetchDashboard, handleDismissAttention, handleSelectKeyDrilldown,
    hoveredBucket, keyStats, lastRefreshed, liveStats, recentLogs, recentLogsLoading, refreshing, selectedKey,
    setChartMetric, setHoveredBucket, setLastRefreshed, setRefreshing, setTimeframe, statsKey, timeframe,
    timeseriesData
  };
}
