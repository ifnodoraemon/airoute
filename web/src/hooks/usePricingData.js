import React, { useEffect, useCallback } from 'react';

export default function usePricingData({
  adminFetch, groupMultipliers, isZh, localGroupMultipliers, prices, setIsWeekend, setLoading,
  setLocalGroupMultipliers, setPlatformModels, setPrices, setServerTime, setServerWeekday, showToast
}) {
  const fetchPrices = useCallback(async () => {
    setLoading(true);
    try {
      const [res, modelsRes] = await Promise.all([
        adminFetch('/api/v1/admin/pricing'),
        adminFetch('/api/v1/admin/models').catch(() => null)
      ]);
      const data = await res.json();
      if (res.ok && data.code === 0) {
        setPrices(data.data || []);
        if (data.group_multipliers && typeof data.group_multipliers === 'object') {
          setLocalGroupMultipliers(data.group_multipliers);
        }
        if (data.server_time) setServerTime(data.server_time);
        if (data.server_weekday) setServerWeekday(data.server_weekday);
        if (data.is_weekend !== undefined) setIsWeekend(data.is_weekend);
      } else {
        showToast(data.error || '获取模型费率失败', 'warning');
      }

      if (modelsRes && modelsRes.ok) {
        const mData = await modelsRes.json();
        if (mData.code === 0 && Array.isArray(mData.data)) {
          setPlatformModels(mData.data);
        }
      }
    } catch (err) {
      showToast((isZh ? '网络请求异常: ' : 'Network request error: ') + err.message, 'error');
    } finally {
      setLoading(false);
    }
  }, [setLoading, adminFetch, setPrices, setServerTime, setServerWeekday, setIsWeekend, setLocalGroupMultipliers, showToast, setPlatformModels, isZh]);

  useEffect(() => {
    if (groupMultipliers && typeof groupMultipliers === 'object') {
      setLocalGroupMultipliers(groupMultipliers);
    }
  }, [groupMultipliers, setLocalGroupMultipliers]);

  const availableGroups = React.useMemo(() => {
    const set = new Set(['default']);
    if (localGroupMultipliers && typeof localGroupMultipliers === 'object') {
      Object.keys(localGroupMultipliers).forEach(g => {
        if (g) set.add(g.trim().toLowerCase());
      });
    }
    prices.forEach(p => {
      if (p.group_name) set.add(p.group_name.trim().toLowerCase());
    });
    return Array.from(set);
  }, [prices, localGroupMultipliers]);
  return {
    availableGroups, fetchPrices
  };
}
