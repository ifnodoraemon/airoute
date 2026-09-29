import React from 'react';
import { Key, Plus, Trash2, Copy, Check, Sparkles, DollarSign } from 'lucide-react';

export default function KeysView({
  keys,
  handleOpenCreateKey,
  selectedKeyIds,
  setSelectedKeyIds,
  handleBatchStatusKeys,
  handleBatchDeleteKeys,
  copyToClipboard,
  copiedKey,
  handleToggleKeyStatus,
  setActiveQuickKey,
  handleDeleteKey,
  handleUpdateKeyGroup,
  adminUser,
  pricingGroups = [],
  onNavigateToPricing,
}) {
  const isAdmin = adminUser?.role === 'admin';
  const userGuaranteedGroup = adminUser?.group_name && adminUser.group_name !== 'default' ? adminUser.group_name : null;
  const canUserSwitch = isAdmin || !!userGuaranteedGroup;

  const getGroupBadge = (group) => {
    const g = (group || 'default').toLowerCase();
    if (g === 'vip') {
      return {
        label: '⭐ VIP 组',
        className: 'bg-amber-50 dark:bg-amber-950/40 text-amber-800 dark:text-amber-300 border-amber-200 dark:border-amber-800/60'
      };
    }
    if (g === 'enterprise') {
      return {
        label: '👑 企业组',
        className: 'bg-purple-50 dark:bg-purple-950/40 text-purple-800 dark:text-purple-300 border-purple-200 dark:border-purple-800/60'
      };
    }
    if (g === 'default') {
      return {
        label: '默认组',
        className: 'bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300 border-slate-200 dark:border-slate-700'
      };
    }
    return {
      label: `${group} 组`,
      className: 'bg-sky-50 dark:bg-sky-950/40 text-sky-800 dark:text-sky-300 border-sky-200 dark:border-sky-800/60'
    };
  };

  const getGroupOptions = (currentKeyGroup) => {
    if (isAdmin) {
      const set = new Set(['default']);
      (pricingGroups || []).forEach(g => set.add(g));
      if (currentKeyGroup) set.add(currentKeyGroup);
      return Array.from(set).map(g => ({
        id: g,
        label: g === 'default' ? '默认组 (default)' : g === 'vip' ? 'VIP 组 (vip)' : g === 'enterprise' ? '企业组 (enterprise)' : `${g} 组`
      }));
    }
    if (userGuaranteedGroup) {
      const set = new Set(['default', userGuaranteedGroup]);
      if (currentKeyGroup) set.add(currentKeyGroup);
      return Array.from(set).map(g => ({
        id: g,
        label: g === 'default' ? '默认标准组 (default)' : `${g} 组 (账号专属)`
      }));
    }
    return [
      { id: currentKeyGroup || 'default', label: currentKeyGroup && currentKeyGroup !== 'default' ? `${currentKeyGroup} 组` : '默认组' }
    ];
  };
  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center bg-white dark:bg-[#111726] p-5 rounded-3xl border border-slate-200/80 dark:border-slate-800/80 shadow-xs gap-3">
        <div className="flex items-center space-x-2">
          <Key className="w-4 h-4 text-amber-500" />
          <h3 className="font-semibold text-slate-900 dark:text-slate-100 text-sm">
            API 访问密钥
          </h3>
        </div>
        <div className="flex items-center space-x-2">
          {onNavigateToPricing && (
            <button
              onClick={onNavigateToPricing}
              className="px-3.5 py-2 bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-300 rounded-xl text-xs font-semibold shadow-2xs flex items-center space-x-1.5 transition cursor-pointer"
              title="查看各计费分组的模型定价明细"
            >
              <DollarSign className="w-3.5 h-3.5 text-emerald-500" />
              <span>查看组内费率</span>
            </button>
          )}
          <button
            onClick={handleOpenCreateKey}
            className="px-4 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-semibold shadow-sm flex items-center space-x-1.5 transition cursor-pointer"
          >
            <Plus className="w-4 h-4" />
            <span>新建密钥</span>
          </button>
        </div>
      </div>

      {keys.length === 0 ? (
        <div className="bg-white dark:bg-[#111726] border border-dashed border-slate-300 dark:border-slate-800 rounded-3xl p-10 text-center flex flex-col items-center justify-center space-y-3">
          <div className="w-12 h-12 rounded-2xl bg-amber-50 dark:bg-amber-950/60 border border-amber-200 dark:border-amber-800/80 flex items-center justify-center text-amber-600 dark:text-amber-400 shadow-sm">
            <Key className="w-6 h-6" />
          </div>
          <h4 className="font-bold text-sm text-slate-900 dark:text-slate-100">暂无 API 密钥</h4>
          <button
            onClick={handleOpenCreateKey}
            className="px-4 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-semibold shadow-sm flex items-center space-x-1.5 transition cursor-pointer"
          >
            <Plus className="w-4 h-4" />
            <span>新建密钥</span>
          </button>
        </div>
      ) : (
        <div className="space-y-3">
          {selectedKeyIds.length > 0 && (
            <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between bg-indigo-50 dark:bg-indigo-950/40 border border-indigo-200 dark:border-indigo-800/60 px-5 py-3 rounded-2xl animate-in fade-in gap-3">
              <div className="flex items-center space-x-2 text-xs font-semibold text-indigo-900 dark:text-indigo-200">
                <span>已选中 {selectedKeyIds.length} 个密钥</span>
              </div>
              <div className="flex items-center space-x-2">
                <button
                  onClick={() => handleBatchStatusKeys('active')}
                  className="px-3 py-1.5 bg-emerald-600 hover:bg-emerald-700 text-white text-xs font-semibold rounded-xl shadow-xs transition cursor-pointer"
                >
                  批量启用
                </button>
                <button
                  onClick={() => handleBatchStatusKeys('disabled')}
                  className="px-3 py-1.5 bg-amber-600 hover:bg-amber-700 text-white text-xs font-semibold rounded-xl shadow-xs transition cursor-pointer"
                >
                  批量停用
                </button>
                <button
                  onClick={handleBatchDeleteKeys}
                  className="px-3 py-1.5 bg-rose-600 hover:bg-rose-700 text-white text-xs font-semibold rounded-xl shadow-xs transition flex items-center space-x-1 cursor-pointer"
                >
                  <Trash2 className="w-3.5 h-3.5" />
                  <span>批量注销</span>
                </button>
                <button
                  onClick={() => setSelectedKeyIds([])}
                  className="px-3 py-1.5 bg-white dark:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-200 dark:border-slate-700 text-xs font-medium rounded-xl hover:bg-slate-50 transition cursor-pointer"
                >
                  取消选择
                </button>
              </div>
            </div>
          )}

          <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl overflow-hidden shadow-xs">
            <table className="w-full text-left border-collapse">
              <thead>
                <tr className="border-b border-slate-200 dark:border-slate-800 text-slate-500 dark:text-slate-400 text-xs uppercase bg-slate-50/80 dark:bg-slate-900/80">
                  <th className="py-3.5 px-4 w-10 text-center">
                    <input
                      type="checkbox"
                      className="rounded border-slate-300 text-indigo-600 focus:ring-indigo-500 cursor-pointer"
                      checked={keys.length > 0 && selectedKeyIds.length === keys.length}
                      onChange={(e) => {
                        if (e.target.checked) {
                          setSelectedKeyIds(keys.map((k) => k.id));
                        } else {
                          setSelectedKeyIds([]);
                        }
                      }}
                    />
                  </th>
                  <th className="py-3.5 px-6 font-semibold">密钥名称</th>
                  <th className="py-3.5 px-6 font-semibold">密钥 (API Key)</th>
                  <th className="py-3.5 px-6 font-semibold">
                    <div className="flex items-center space-x-1">
                      <span>计费分组</span>
                      {onNavigateToPricing && (
                        <button
                          type="button"
                          onClick={onNavigateToPricing}
                          className="text-[10px] text-indigo-600 dark:text-indigo-400 hover:underline font-normal cursor-pointer ml-0.5"
                          title="查看各分组费率详情"
                        >
                          (费率明细)
                        </button>
                      )}
                    </div>
                  </th>
                  <th className="py-3.5 px-6 font-semibold">已用 / 额度</th>
                  <th className="py-3.5 px-6 font-semibold">授权模型</th>
                  <th className="py-3.5 px-6 font-semibold">速率 (RPM)</th>
                  <th className="py-3.5 px-6 font-semibold">状态</th>
                  <th className="py-3.5 px-6 text-right font-semibold">操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800/60 text-sm">
                {keys.map((k) => (
                  <tr key={k.id} className="hover:bg-slate-50/60 dark:hover:bg-slate-800/40 transition">
                    <td className="py-4 px-4 text-center">
                      <input
                        type="checkbox"
                        className="rounded border-slate-300 text-indigo-600 focus:ring-indigo-500 cursor-pointer"
                        checked={selectedKeyIds.includes(k.id)}
                        onChange={(e) => {
                          e.stopPropagation();
                          if (e.target.checked) {
                            setSelectedKeyIds((prev) => [...prev, k.id]);
                          } else {
                            setSelectedKeyIds((prev) => prev.filter((x) => x !== k.id));
                          }
                        }}
                      />
                    </td>
                    <td className="py-4 px-6 text-slate-800 dark:text-slate-100 font-semibold">{k.tenant_id}</td>
                    <td className="py-4 px-6 font-mono text-xs text-indigo-700 dark:text-indigo-400 font-semibold flex items-center space-x-2">
                      <span>{k.key}</span>
                      <button
                        onClick={() => copyToClipboard(k.key)}
                        className="p-1 hover:bg-indigo-50 dark:hover:bg-indigo-950/60 rounded-lg text-slate-400 hover:text-indigo-600 dark:hover:text-indigo-300 transition flex items-center space-x-1 cursor-pointer"
                        title="复制 Key"
                      >
                        {copiedKey === k.key ? (
                          <Check className="w-3.5 h-3.5 text-emerald-500" />
                        ) : (
                          <Copy className="w-3.5 h-3.5" />
                        )}
                      </button>
                    </td>
                    <td className="py-4 px-6 text-xs">
                      {canUserSwitch ? (
                        <select
                          value={k.group_name || 'default'}
                          onChange={(e) => handleUpdateKeyGroup && handleUpdateKeyGroup(k.id, e.target.value)}
                          className="bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded-lg px-2.5 py-1 text-[11px] font-semibold text-slate-800 dark:text-slate-200 focus:outline-none focus:border-indigo-500 cursor-pointer shadow-2xs hover:border-slate-300 dark:hover:border-slate-600 transition"
                          title="切换该密钥的扣费分组"
                        >
                          {getGroupOptions(k.group_name).map(opt => (
                            <option key={opt.id} value={opt.id}>
                              {opt.label}
                            </option>
                          ))}
                        </select>
                      ) : (
                        <span
                          className={`inline-flex items-center px-2.5 py-0.5 rounded-lg text-[11px] font-semibold border ${getGroupBadge(k.group_name).className}`}
                        >
                          {getGroupBadge(k.group_name).label}
                        </span>
                      )}
                    </td>
                    <td className="py-4 px-6 text-xs font-mono">
                      <span className="text-slate-900 dark:text-slate-100 font-bold">
                        ¥{Number(k.used_cost || 0).toFixed(4)}
                      </span>
                      <span className="text-slate-400 mx-1">/</span>
                      <span className={k.budget > 0 ? 'text-indigo-600 dark:text-indigo-400 font-medium' : 'text-slate-400'}>
                        {k.budget > 0 ? `¥${Number(k.budget).toFixed(2)}` : '不限'}
                      </span>
                    </td>
                    <td className="py-4 px-6 text-xs text-slate-600 dark:text-slate-300 font-medium">
                      {!k.allowed_models || k.allowed_models.length === 0 ? (
                        <span className="inline-flex items-center px-2 py-0.5 rounded-full text-[11px] bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800/60 font-semibold">
                          全部允许
                        </span>
                      ) : (
                        <span className="font-mono">{k.allowed_models.join(', ')}</span>
                      )}
                    </td>
                    <td className="py-4 px-6 text-slate-600 dark:text-slate-400 font-mono text-xs">
                      {k.rpm ? `${k.rpm} 次/分` : '不限'}
                    </td>
                    <td className="py-4 px-6">
                      {k.status === 'disabled' ? (
                        <span className="px-2.5 py-0.5 rounded-full text-xs bg-rose-50 dark:bg-rose-950/50 text-rose-700 dark:text-rose-300 border border-rose-200 dark:border-rose-800/60 font-medium inline-flex items-center space-x-1.5">
                          <span className="w-1.5 h-1.5 rounded-full bg-rose-500 animate-pulse"></span>
                          <span>已停用</span>
                        </span>
                      ) : (
                        <span className="px-2.5 py-0.5 rounded-full text-xs bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800/60 font-medium inline-flex items-center space-x-1.5">
                          <span className="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>
                          <span>正常</span>
                        </span>
                      )}
                    </td>
                    <td className="py-4 px-6 text-right space-x-2">
                      <button
                        onClick={() => handleToggleKeyStatus(k)}
                        className={`text-xs px-2.5 py-1.5 rounded-xl border font-medium transition cursor-pointer ${
                          k.status === 'disabled'
                            ? 'bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 hover:bg-emerald-100 border-emerald-200 dark:border-emerald-800/60'
                            : 'bg-amber-50 dark:bg-amber-950/50 text-amber-700 dark:text-amber-300 hover:bg-amber-100 border-amber-200 dark:border-amber-800/60'
                        }`}
                        title={k.status === 'disabled' ? '恢复启用' : '暂停使用'}
                      >
                        {k.status === 'disabled' ? '启用' : '停用'}
                      </button>
                      <button
                        onClick={() => setActiveQuickKey(k)}
                        className="text-xs px-3 py-1.5 rounded-xl bg-indigo-50 dark:bg-indigo-950/50 text-indigo-700 dark:text-indigo-300 hover:bg-indigo-100 dark:hover:bg-indigo-900/60 border border-indigo-200 dark:border-indigo-800/60 font-medium transition inline-flex items-center space-x-1 cursor-pointer"
                      >
                        <Sparkles className="w-3.5 h-3.5 text-indigo-500" />
                        <span>快速接入</span>
                      </button>
                      <button
                        onClick={() => handleDeleteKey(k.id)}
                        className="text-xs px-2.5 py-1.5 text-rose-600 dark:text-rose-400 hover:text-rose-800 dark:hover:text-rose-300 transition font-medium cursor-pointer"
                      >
                        删除
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}
