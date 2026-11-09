import React from 'react';

export default function usePricingModels({
  formData, groupFilter, isZh, modelRoutes, platformModels, prices
}) {
  const filteredPrices = React.useMemo(() => {
    if (groupFilter === 'all') return prices;
    return prices.filter(p => (p.group_name || 'default') === groupFilter);
  }, [prices, groupFilter]);

  const mountedRouteModels = React.useMemo(() => {
    return new Set((modelRoutes || []).map(r => r.model));
  }, [modelRoutes]);

  const unroutedPrices = React.useMemo(() => {
    return prices.filter(p => p.model !== '*' && !mountedRouteModels.has(p.model));
  }, [prices, mountedRouteModels]);

  const unpricedRoutes = React.useMemo(() => {
    const pricedModels = new Set(prices.map(p => p.model));
    return (modelRoutes || []).filter(r => !pricedModels.has(r.model) && !pricedModels.has('*'));
  }, [modelRoutes, prices]);

  const unpricedModels = React.useMemo(() => {
    const curGroup = formData.group_name || 'default';
    const configured = new Set(prices.filter(p => (p.group_name || 'default') === curGroup).map(p => p.model));
    return (platformModels || []).filter(m => !configured.has(m));
  }, [platformModels, prices, formData.group_name]);

  const candidateModels = React.useMemo(() => {
    const curGroup = formData.group_name || 'default';
    const configuredInGroup = new Set(prices.filter(p => (p.group_name || 'default') === curGroup).map(p => p.model));
    const list = [];
    const seen = new Set();

    // 1. Unpriced platform models in this group first
    unpricedModels.forEach(m => {
      if (!seen.has(m) && m !== '*') {
        seen.add(m);
        list.push({ model: m, isUnpriced: true });
      }
    });

    // 2. Models from mounted routes
    (modelRoutes || []).forEach(r => {
      if (r?.model && !seen.has(r.model) && r.model !== '*') {
        seen.add(r.model);
        list.push({ model: r.model, isUnpriced: !configuredInGroup.has(r.model) });
      }
    });

    // 3. Platform models
    (platformModels || []).forEach(m => {
      if (!seen.has(m) && m !== '*') {
        seen.add(m);
        list.push({ model: m, isUnpriced: !configuredInGroup.has(m) });
      }
    });

    // 4. Popular preset models if candidate list is short
    const popularPresets = [
      'deepseek-chat', 'deepseek-reasoner', 'deepseek-v4.1-flash',
      'gpt-4o', 'gpt-4o-mini', 'claude-3-7-sonnet', 'qwen-max', 'qwen-plus'
    ];
    popularPresets.forEach(m => {
      if (!seen.has(m) && list.length < 12) {
        seen.add(m);
        list.push({ model: m, isPreset: true, isUnpriced: !configuredInGroup.has(m) });
      }
    });

    return list;
  }, [unpricedModels, modelRoutes, platformModels, prices, formData.group_name]);

  const getGroupBadge = (group) => {
    const g = (group || 'default').toLowerCase();
    if (g === 'vip') {
      return {
        label: isZh ? 'VIP 用户组' : 'VIP Tier',
        className: 'bg-amber-50 dark:bg-amber-950/40 text-amber-700 dark:text-amber-300 border-amber-200 dark:border-amber-800'
      };
    }
    if (g === 'enterprise') {
      return {
        label: isZh ? '企业大客户' : 'Enterprise Tier',
        className: 'bg-purple-50 dark:bg-purple-950/40 text-purple-700 dark:text-purple-300 border-purple-200 dark:border-purple-800'
      };
    }
    if (g === 'default') {
      return {
        label: isZh ? '默认基础组' : 'Default Tier',
        className: 'bg-indigo-50 dark:bg-indigo-950/40 text-indigo-700 dark:text-indigo-300 border-indigo-200 dark:border-indigo-800'
      };
    }
    return {
      label: isZh ? `${group} 组` : `${group} Tier`,
      className: 'bg-sky-50 dark:bg-sky-950/40 text-sky-700 dark:text-sky-300 border-sky-200 dark:border-sky-800'
    };
  };
  return {
    candidateModels, filteredPrices, getGroupBadge, mountedRouteModels, unpricedModels, unpricedRoutes,
    unroutedPrices
  };
}
