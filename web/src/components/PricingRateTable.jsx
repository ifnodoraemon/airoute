import { AlertCircle, Moon, CheckCircle2, Calendar, ArrowRight } from 'lucide-react';

export default function PricingRateTable({
  filteredPrices, formatDaysLabel, getGroupBadge, handleDelete, handleOpenEdit, isAdmin, isZh,
  localGroupMultipliers, mountedRouteModels, platformModels, prices, selectedKeys, setCurrentTab,
  setSelectedKeys, tierPreviewGroup
}) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-left text-xs border-collapse">
        <thead>
          <tr className="border-b border-[#E4E6E9] dark:border-[#2C3038] text-[#656B75] dark:text-[#A1A7B0] font-medium bg-[#F0F1F2]/50 dark:bg-[#22252B]/50">
            {isAdmin && (
              <th className="ui-th w-10 text-center">
                <input
                  type="checkbox"
                  className="rounded border-[#E4E6E9] dark:border-[#2C3038] text-[#355CFF] focus:ring-[#355CFF]/30 cursor-pointer"
                  checked={filteredPrices.length > 0 && selectedKeys.length === filteredPrices.length}
                  onChange={(e) => {
                    if (e.target.checked) {
                      setSelectedKeys(filteredPrices.map(p => `${p.group_name || 'default'}::${p.model}`));
                    } else {
                      setSelectedKeys([]);
                    }
                  }}
                />
              </th>
            )}
            <th className="ui-th">{isZh ? '模型标识' : 'Model'}</th>
            <th className="ui-th">{isZh ? '适用用户组' : 'Group Tier'}</th>
            <th className="ui-th">{isZh ? '基准输入 (1M)' : 'Prompt (1M)'}</th>
            <th className="ui-th">{isZh ? '基准输出 (1M)' : 'Completion (1M)'}</th>
            <th className="ui-th">{isZh ? '缓存命中 (1M)' : 'Cache Hit (1M)'}</th>
            <th className="ui-th">{isZh ? '分时优惠时段' : 'Off-Peak Windows'}</th>
            <th className="ui-th">{isZh ? '当前生效费率' : 'Live Rate'}</th>
            {isAdmin && <th className="ui-th text-right">{isZh ? '操作' : 'Actions'}</th>}
          </tr>
        </thead>
        <tbody className="divide-y divide-[#E4E6E9]/60 dark:divide-[#2C3038]/60 text-[#202124] dark:text-[#F1F2F4]">
          {filteredPrices.map((p) => {
            const itemKey = `${p.group_name || 'default'}::${p.model}`;
            const isOff = p.is_currently_off_peak;
            const discount = p.current_discount || 1.0;
            const effPrompt = p.effective_prompt_price !== undefined ? p.effective_prompt_price : p.prompt_price;
            const effComp = p.effective_completion_price !== undefined ? p.effective_completion_price : p.completion_price;
            const currencySym = p.currency === 'USD' ? '$' : '¥';
            const grpBadge = getGroupBadge(p.group_name);

            // Parse multi-slots for clean display
            let displaySlots = [];
            if (p.off_peak_slots) {
              try {
                const arr = JSON.parse(p.off_peak_slots);
                if (Array.isArray(arr) && arr.length > 0) displaySlots = arr;
              } catch (e) {}
            }
            if (displaySlots.length === 0 && (p.off_peak_start || p.off_peak_end)) {
              displaySlots = [{
                name: isZh ? '优惠时段' : 'Off-Peak',
                start: p.off_peak_start || '00:00',
                end: p.off_peak_end || '08:30',
                discount: p.off_peak_discount ?? 0.5
              }];
            }

            const isRouteMounted = p.model === '*' || mountedRouteModels.has(p.model) || (platformModels || []).includes(p.model);

            return (
              <tr key={itemKey} className="ui-tr-hover">
                {isAdmin && (
                  <td className="ui-td text-center" onClick={(e) => e.stopPropagation()}>
                    <input
                      type="checkbox"
                      className="rounded border-[#E4E6E9] dark:border-[#2C3038] text-[#355CFF] focus:ring-[#355CFF]/30 cursor-pointer"
                      checked={selectedKeys.includes(itemKey)}
                      onChange={(e) => {
                        e.stopPropagation();
                        if (e.target.checked) {
                          setSelectedKeys(prev => [...prev, itemKey]);
                        } else {
                          setSelectedKeys(prev => prev.filter(k => k !== itemKey));
                        }
                      }}
                    />
                  </td>
                )}
                <td className="ui-td font-mono font-bold text-[#202124] dark:text-[#F1F2F4]">
                  <div className="flex items-center space-x-2">
                    <span>{p.model}</span>
                    {p.model !== '*' && (
                      isRouteMounted ? (
                        <span
                          className="inline-flex items-center space-x-1 px-1.5 py-0.5 rounded text-[10px] font-sans font-medium bg-emerald-50 dark:bg-emerald-950/40 text-emerald-700 dark:text-emerald-300 border border-emerald-200/80 dark:border-emerald-800"
                          title={isZh ? '已挂载上游路由渠道，可正常调度调用' : 'Healthy upstream route active'}
                        >
                          <CheckCircle2 className="w-2.5 h-2.5 text-emerald-500" />
                          <span>{isZh ? '路由就绪' : 'Routed'}</span>
                        </span>
                      ) : (
                        <button
                          type="button"
                          onClick={(e) => {
                            e.stopPropagation();
                            if (setCurrentTab) setCurrentTab('models');
                          }}
                          className="inline-flex items-center space-x-1 px-1.5 py-0.5 rounded text-[10px] font-sans font-medium bg-amber-50 dark:bg-amber-950/40 hover:bg-amber-100 dark:hover:bg-amber-900/60 text-amber-700 dark:text-amber-300 border border-amber-200/80 dark:border-amber-800 cursor-pointer transition text-left"
                          title={isZh ? '当前无可用上游渠道挂载此模型，客户端调用将返回 404！点击前往【模型路由】挂载服务商' : 'No upstream route configured. Click to configure route.'}
                        >
                          <AlertCircle className="w-2.5 h-2.5 text-amber-500 shrink-0" />
                          <span>{isZh ? '未配路由' : 'Unrouted'}</span>
                          <ArrowRight className="w-2 h-2 text-amber-600 ml-0.5" />
                        </button>
                      )
                    )}
                  </div>
                </td>

                <td className="ui-td">
                  <span className={`inline-flex items-center px-2 py-0.5 rounded-lg text-xs font-semibold border ${grpBadge.className}`}>
                    {grpBadge.label}
                  </span>
                </td>

                <td className="ui-td font-mono font-medium">
                  {currencySym}{Number(p.prompt_price || 0).toFixed(2)}
                </td>

                <td className="ui-td font-mono font-medium">
                  {currencySym}{Number(p.completion_price || 0).toFixed(2)}
                </td>

                <td className="ui-td font-mono text-emerald-700 dark:text-emerald-400">
                  <div>
                    {currencySym}{Number(p.cache_read_price || 0).toFixed(2)}
                    {p.prompt_price > 0 && (
                      <span className="text-xs text-[#656B75] dark:text-[#A1A7B0] ml-1">
                        ({Math.round(((p.prompt_price - p.cache_read_price) / p.prompt_price) * 100)}% {isZh ? '节省' : 'Saved'})
                      </span>
                    )}
                  </div>
                </td>

                {/* Multi-slot Off-peak policy column */}
                <td className="ui-td font-sans">
                  {p.off_peak_enabled ? (
                    <div className="space-y-1.5">
                      <div className="flex flex-wrap gap-1.5">
                        {displaySlots.map((slot, sIdx) => {
                          const daysStr = formatDaysLabel(slot.days, isZh);
                          return (
                            <span
                              key={sIdx}
                              className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-lg bg-[#EEF2FF] dark:bg-[#242D48] border border-[#355CFF]/20 text-[#355CFF] dark:text-[#91A7FF] text-xs font-semibold"
                            >
                              <Moon className="w-3 h-3 text-[#355CFF] dark:text-[#91A7FF]" />
                              <span>
                                {slot.name ? `${slot.name}: ` : ''}
                                {daysStr ? `${daysStr} ` : ''}
                                {slot.start}-{slot.end} ({isZh ? `${Math.round((slot.discount ?? 0.5) * 10)}折` : `${Math.round((1 - (slot.discount ?? 0.5)) * 100)}% Off`})
                              </span>
                            </span>
                          );
                        })}
                      </div>
                      {p.weekend_all_day && (
                        <div className="ui-meta flex items-center space-x-1">
                          <Calendar className="w-3 h-3 text-[#656B75] dark:text-[#A1A7B0]" />
                          <span>{isZh ? '周六日全天享受优惠' : 'Weekend all-day discount active'}</span>
                        </div>
                      )}
                    </div>
                  ) : (
                    <span className="text-[#656B75] dark:text-[#A1A7B0] text-xs">{isZh ? '全天统一定价' : 'Uniform 24/7 Pricing'}</span>
                  )}
                </td>

                {/* Live effective rate */}
                <td className="ui-td font-mono">
                  {(() => {
                    const priceGroup = p.group_name || 'default';
                    const previewMult = tierPreviewGroup !== 'default' && priceGroup !== tierPreviewGroup ? (localGroupMultipliers[tierPreviewGroup] ?? 1.0) : 1.0;
                    if (priceGroup !== 'default' && priceGroup !== tierPreviewGroup) {
                      return <span className="ui-meta">{isZh ? '请选择对应用户组预览' : 'Select the matching group to preview'}</span>;
                    }
                    if (priceGroup === 'default' && tierPreviewGroup !== 'default' && prices.some(rate => rate.model === p.model && rate.group_name === tierPreviewGroup)) {
                      return <span className="ui-meta">{isZh ? '使用该用户组专属定价' : 'Uses the group-specific price'}</span>;
                    }
                    const showTierTag = tierPreviewGroup !== 'default' && previewMult !== 1.0;
                    const calcPrompt = Math.round(effPrompt * previewMult * 1e6) / 1e6;
                    const calcComp = Math.round(effComp * previewMult * 1e6) / 1e6;

                    return isOff ? (
                      <div className="space-y-0.5">
                        <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-md bg-emerald-50 dark:bg-emerald-950/40 border border-emerald-200 dark:border-emerald-800 text-emerald-700 dark:text-emerald-300 text-xs font-bold">
                          <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse"></span>
                          <span>{isZh ? `优惠 ${Math.round(discount * 10)}折 生效中` : `${Math.round((1 - discount) * 100)}% Off Active`}</span>
                          {showTierTag && <span className="text-[10px] ml-1 font-sans opacity-85">({tierPreviewGroup} {previewMult}x)</span>}
                        </span>
                        <div className="text-xs text-emerald-700 dark:text-emerald-400 font-bold">
                          {isZh ? '输入' : 'Prompt'} {currencySym}{calcPrompt.toFixed(2)} / {isZh ? '输出' : 'Output'} {currencySym}{calcComp.toFixed(2)}
                        </div>
                      </div>
                    ) : (
                      <div className="space-y-0.5">
                        <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-md bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0] text-xs font-medium">
                          <span>{isZh ? '基准单价' : 'Standard Rate'}</span>
                          {showTierTag && <span className="text-[10px] ml-1 font-sans text-[#355CFF] dark:text-[#91A7FF] font-bold">({tierPreviewGroup} {previewMult}x)</span>}
                        </span>
                        <div className="text-xs text-[#202124] dark:text-[#F1F2F4] font-medium">
                          {isZh ? '输入' : 'Prompt'} {currencySym}{calcPrompt.toFixed(2)} / {isZh ? '输出' : 'Output'} {currencySym}{calcComp.toFixed(2)}
                        </div>
                      </div>
                    );
                  })()}
                </td>

                {/* Actions */}
                {isAdmin && (
                  <td className="ui-td text-right space-x-2 font-sans">
                    <button
                      onClick={() => handleOpenEdit(p)}
                      className="ui-btn-secondary ui-btn-sm"
                    >
                      {isZh ? '编辑' : 'Edit'}
                    </button>
                    <button
                      onClick={() => handleDelete(p.model, p.group_name || 'default')}
                      className="text-xs px-2 py-1 text-[#656B75] dark:text-[#A1A7B0] hover:text-rose-600 dark:hover:text-rose-400 transition font-medium cursor-pointer"
                    >
                      {isZh ? '删除' : 'Delete'}
                    </button>
                  </td>
                )}
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
