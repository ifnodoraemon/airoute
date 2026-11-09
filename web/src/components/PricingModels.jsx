import { RefreshCw, AlertCircle, Clock, Info, ArrowRight } from 'lucide-react';
import PricingRateTable from './PricingRateTable';

export default function PricingModels({
  availableGroups, filteredPrices, formatDaysLabel, getGroupBadge, groupFilter, handleDelete, handleOpenEdit,
  isAdmin, isWeekend, isZh, loading, localGroupMultipliers, mountedRouteModels, platformModels, prices,
  selectedKeys, serverTime, serverWeekday, setCurrentTab, setEditingPrice, setFormData, setGroupFilter,
  setPricingSubTab, setSelectedKeys, setShowModal, setTierPreviewGroup, tierPreviewGroup, unpricedRoutes,
  unroutedPrices
}) {
  return (
    <div className="space-y-4">
      {/* Architecture Guidance Banner */}
      <div className="p-3.5 rounded-xl border border-[#355CFF]/20 bg-[#EEF2FF]/60 dark:bg-[#242D48]/40 flex items-start sm:items-center justify-between gap-3 text-xs">
        <div className="flex items-center space-x-2.5 text-[#202124] dark:text-[#F1F2F4] min-w-0">
          <Info className="w-4 h-4 text-[#355CFF] dark:text-[#91A7FF] shrink-0 mt-0.5 sm:mt-0" />
          <span>
            {isZh
              ? '这里配置客户调用费率。默认定价可叠加分组倍率；命中分组专属定价时直接使用该定价，不重复应用分组倍率。闲时优惠在此基础上计算。'
              : 'These are customer rates. Group multipliers apply to inherited default rates; group-specific prices override them without a second group discount. Off-peak discounts apply afterward.'}
          </span>
        </div>
        <button
          type="button"
          onClick={() => setPricingSubTab('groups')}
          className="shrink-0 text-[#355CFF] dark:text-[#91A7FF] font-semibold hover:underline inline-flex items-center space-x-1 cursor-pointer"
        >
          <span>{isZh ? '配置分组倍率' : 'Configure Multipliers'}</span>
          <ArrowRight className="w-3 h-3" />
        </button>
      </div>

      <div className="ui-card overflow-hidden">
        {/* Group Filter Tabs & Tier Preview */}
        <div className="flex flex-wrap items-center justify-between gap-2 px-6 pt-5 pb-3 border-b border-[#E4E6E9] dark:border-[#2C3038]">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-xs text-[#656B75] dark:text-[#A1A7B0] font-medium mr-1">{isZh ? '价格分组:' : 'Price Group:'}</span>
            {[
              { id: 'all', label: isZh ? '全部规则' : 'All Rules' },
              ...availableGroups.map(g => ({
                id: g,
                label: g === 'default'
                  ? (isZh ? '默认组 (default)' : 'Default (default)')
                  : g === 'vip'
                    ? (isZh ? 'VIP组 (vip)' : 'VIP (vip)')
                    : g === 'enterprise'
                      ? (isZh ? '企业组 (enterprise)' : 'Enterprise (enterprise)')
                      : `${g} ${isZh ? '组' : 'Tier'}`
              }))
            ].map(tab => {
              const count = tab.id === 'all' ? prices.length : prices.filter(p => (p.group_name || 'default') === tab.id).length;
              const isActive = groupFilter === tab.id;
              return (
                <button
                  key={tab.id}
                  onClick={() => { setGroupFilter(tab.id); setSelectedKeys([]); }}
                  className={`px-3 py-1.5 rounded-lg text-xs font-medium transition cursor-pointer flex items-center space-x-1.5 ${
                    isActive
                      ? 'bg-[#355CFF] text-white shadow-xs'
                      : 'bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'
                  }`}
                >
                  <span>{tab.label}</span>
                  <span className={`text-[10px] px-1.5 py-0.2 rounded-full ${isActive ? 'bg-white/20 text-white' : 'bg-[#E4E6E9] dark:bg-[#2C3038] text-[#656B75] dark:text-[#A1A7B0]'}`}>
                    {count}
                  </span>
                </button>
              );
            })}
          </div>

          {/* Rate Preview Tier Selector */}
          <div className="flex items-center space-x-2">
            <span className="text-xs text-[#656B75] dark:text-[#A1A7B0] font-medium">{isZh ? '折算生效预览:' : 'Preview Tier:'}</span>
            <select
              value={tierPreviewGroup}
              onChange={(e) => setTierPreviewGroup(e.target.value)}
              className="ui-field text-xs font-semibold py-1 px-2.5 rounded-lg cursor-pointer"
            >
              <option value="default">{isZh ? '默认客户定价 (1.0x)' : 'Default Customer Rate (1.0x)'}</option>
              {Object.entries(localGroupMultipliers).filter(([g]) => g !== 'default').map(([g, mult]) => (
                <option key={g} value={g}>
                  {g === 'vip' ? (isZh ? `VIP组 (${mult}x · ${Math.round(mult * 10)}折)` : `VIP (${mult}x)`)
                    : g === 'enterprise' ? (isZh ? `企业组 (${mult}x)` : `Enterprise (${mult}x)`)
                    : `${g} (${mult}x)`}
                </option>
              ))}
            </select>
          </div>
        </div>

    {/* Route <-> Pricing Matching Banners */}
    {unpricedRoutes.length > 0 && (
      <div className="mb-4 p-3 rounded-xl border border-amber-200/80 dark:border-amber-800/60 bg-amber-50/70 dark:bg-amber-950/30 flex items-center justify-between text-xs animate-in fade-in">
        <div className="flex items-center space-x-2 text-amber-900 dark:text-amber-200 min-w-0">
          <AlertCircle className="w-4 h-4 text-amber-600 shrink-0" />
          <span className="truncate">
            {isZh
              ? `检测到有 ${unpricedRoutes.length} 个模型路由（如 ${unpricedRoutes.slice(0, 2).map(r => r.model).join('、')}${unpricedRoutes.length > 2 ? ' 等' : ''}）未配置专属定价规则，当前按系统基准价计费。`
              : `${unpricedRoutes.length} active model routes lack custom pricing rules and fallback to baseline rates.`}
          </span>
        </div>
        <button
          type="button"
          onClick={() => {
            setFormData(prev => ({ ...prev, model: unpricedRoutes[0].model }));
            setEditingPrice(null);
            setShowModal(true);
          }}
          className="px-2.5 py-1 text-xs font-semibold bg-amber-600 hover:bg-amber-500 text-white rounded-lg shadow-xs cursor-pointer transition shrink-0 ml-3"
        >
          {isZh ? `立即配置 (${unpricedRoutes[0].model})` : `Configure (${unpricedRoutes[0].model})`}
        </button>
      </div>
    )}

    {(serverTime || serverWeekday) && (
      <div className="mb-4 flex items-center space-x-2 text-xs text-slate-500 dark:text-slate-400">
        <Clock className="w-3.5 h-3.5" />
        <span>
          {isZh ? '计费服务器时间' : 'Billing server time'}: {serverWeekday} {serverTime}
          {isWeekend ? (isZh ? '（周末）' : ' (weekend)') : ''}
        </span>
        <span className="text-slate-400 dark:text-slate-600">
          {isZh ? '离峰判定以服务器时区为准' : 'Off-peak is judged in the server time zone'}
        </span>
      </div>
    )}

    {unroutedPrices.length > 0 && (
      <div className="mb-4 p-3 rounded-xl border border-indigo-200/80 dark:border-indigo-800/60 bg-indigo-50/70 dark:bg-indigo-950/30 flex items-center justify-between text-xs animate-in fade-in">
        <div className="flex items-center space-x-2 text-indigo-950 dark:text-indigo-200 min-w-0">
          <Info className="w-4 h-4 text-indigo-600 shrink-0" />
          <span className="truncate">
            {isZh
              ? `提示：有 ${unroutedPrices.length} 个模型（如 ${unroutedPrices.slice(0, 2).map(p => p.model).join('、')}）配置了费率但尚未挂载任何上游路由渠道。`
              : `Notice: ${unroutedPrices.length} models have pricing configured but no upstream channel routes mounted.`}
          </span>
        </div>
        <button
          type="button"
          onClick={() => setCurrentTab && setCurrentTab('models')}
          className="px-2.5 py-1 text-xs font-semibold bg-indigo-600 hover:bg-indigo-500 text-white rounded-lg shadow-xs cursor-pointer transition shrink-0 ml-3"
        >
          {isZh ? '前往模型路由挂载' : 'Go to Model Routes'}
        </button>
      </div>
    )}

    {loading && prices.length === 0 ? (
      <div className="p-12 text-center text-slate-400 text-xs flex items-center justify-center space-x-2">
        <RefreshCw className="w-4 h-4 animate-spin text-indigo-600" />
        <span>{isZh ? '加载费率规则中...' : 'Loading pricing rules...'}</span>
      </div>
    ) : filteredPrices.length === 0 ? (
      <div className="p-12 text-center text-slate-400 text-xs">
        {prices.length === 0
          ? (isZh ? '暂无配置的定价规则，点击上方「新建定价规则」进行添加' : 'No pricing rules configured. Click "New Pricing Rule" above to create one.')
          : (isZh ? '当前分组下暂无定价规则，点击上方「新建定价规则」添加' : 'No pricing rules in this group. Click "New Pricing Rule" to add.')}
      </div>
    ) : (
      <PricingRateTable {...{
        filteredPrices, formatDaysLabel, getGroupBadge, handleDelete, handleOpenEdit, isAdmin, isZh,
        localGroupMultipliers, mountedRouteModels, platformModels, prices, selectedKeys, setCurrentTab,
        setSelectedKeys, tierPreviewGroup
      }} />
    )}
    </div>
    </div>
  );
}
