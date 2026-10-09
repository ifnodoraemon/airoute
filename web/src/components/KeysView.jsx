import React, { useState } from 'react';
import {
  Key,
  Plus,
  Trash2,
  Copy,
  Check,
  Sparkles,
  DollarSign,
  Search,
  Eye,
  EyeOff,
  FileSpreadsheet,
  Terminal,
  ShieldAlert,
  Zap
} from 'lucide-react';
import { getGatewayOrigin } from '../config';

export default function KeysView({
  keys = [],
  handleOpenCreateKey,
  selectedKeyIds = [],
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
  showToast,
  onNavigateToPlayground
}) {
  const [searchQuery, setSearchQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState('all'); // 'all' | 'active' | 'disabled'
  const [isMasked, setIsMasked] = useState(true);

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

  const maskKeyString = (kStr) => {
    if (!kStr) return '-';
    if (!isMasked) return kStr;
    if (kStr.length <= 12) return 'sk-••••••••';
    return `${kStr.slice(0, 6)}••••••••${kStr.slice(-4)}`;
  };

  // Filter keys
  const filteredKeys = keys.filter(k => {
    if (statusFilter !== 'all' && k.status !== statusFilter) return false;
    if (searchQuery.trim()) {
      const q = searchQuery.trim().toLowerCase();
      const matchTenant = (k.tenant_id || '').toLowerCase().includes(q);
      const matchKey = (k.key || '').toLowerCase().includes(q);
      const matchGroup = (k.group_name || '').toLowerCase().includes(q);
      if (!matchTenant && !matchKey && !matchGroup) return false;
    }
    return true;
  });

  // Export keys to CSV
  const handleExportKeysCSV = () => {
    if (filteredKeys.length === 0) {
      if (showToast) showToast('当前没有可导出的密钥', 'warning');
      return;
    }
    const headers = ['ID', '密钥别名', 'API Key', '计费分组', '已用额度(元)', '预算上限(元)', '授权模型', '速率限制(RPM)', '状态', '创建时间'];
    const rows = filteredKeys.map(k => [
      k.id,
      `"${k.tenant_id || ''}"`,
      `"${k.key || ''}"`,
      `"${k.group_name || 'default'}"`,
      (k.used_cost || 0).toFixed(4),
      k.budget > 0 ? k.budget.toFixed(2) : '不限',
      `"${(k.allowed_models || []).join(';') || '*'}"`,
      k.rpm || '不限',
      k.status === 'disabled' ? '已停用' : '正常',
      `"${k.created_at || ''}"`
    ]);
    const csvContent = '\uFEFF' + [headers.join(','), ...rows.map(r => r.join(','))].join('\r\n');
    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `airoute-api-keys-${new Date().toISOString().slice(0, 10)}.csv`;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
    if (showToast) showToast(`成功导出 ${filteredKeys.length} 个密钥数据`, 'success');
  };

  // Copy cURL sample snippet
  const copyCurlSnippet = (keyStr) => {
    const origin = getGatewayOrigin();
    const curl = `curl ${origin}/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer ${keyStr}" \\
  -d '{
    "model": "deepseek-v4-flash",
    "messages": [{"role": "user", "content": "Hello Airoute!"}]
  }'`;
    navigator.clipboard.writeText(curl);
    if (showToast) showToast('cURL 调用示例已复制到剪贴板', 'success');
  };

  const activeCount = keys.filter(k => k.status !== 'disabled').length;
  const totalCost = keys.reduce((acc, k) => acc + Number(k.used_cost || 0), 0);

  return (
    <div className="space-y-6">
      {/* Top Header Card */}
      <div className="bg-white dark:bg-[#111726] p-6 rounded-3xl border border-slate-200/80 dark:border-slate-800/80 shadow-xs space-y-4">
        <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4">
          <div className="flex items-center space-x-3">
            <div className="w-10 h-10 rounded-2xl bg-amber-50 dark:bg-amber-950/60 text-amber-600 dark:text-amber-400 flex items-center justify-center border border-amber-200/60 dark:border-amber-800/60 shadow-xs">
              <Key className="w-5 h-5" />
            </div>
            <div>
              <div className="flex items-center space-x-2">
                <h3 className="font-bold text-slate-900 dark:text-slate-100 text-sm">
                  API 访问密钥管理
                </h3>
                <span className="text-[11px] font-semibold px-2 py-0.5 rounded-full bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-300">
                  {keys.length} 个密钥 · {activeCount} 个已生效
                </span>
              </div>
              <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
                用于各业务客户端与系统调用接入鉴权、独立计费及模型级权限隔离
              </p>
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-2.5">
            {onNavigateToPricing && (
              <button
                onClick={onNavigateToPricing}
                className="px-3.5 py-2 bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-300 rounded-xl text-xs font-semibold shadow-2xs flex items-center space-x-1.5 transition cursor-pointer"
                title="查看各计费分组的模型定价明细"
              >
                <DollarSign className="w-3.5 h-3.5 text-emerald-500" />
                <span>组内费率明细</span>
              </button>
            )}
            <button
              onClick={() => setIsMasked(!isMasked)}
              className="px-3 py-2 bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-300 rounded-xl text-xs font-semibold shadow-2xs flex items-center space-x-1.5 transition cursor-pointer"
              title={isMasked ? '显示明文密钥' : '脱敏隐藏密钥'}
            >
              {isMasked ? <Eye className="w-3.5 h-3.5" /> : <EyeOff className="w-3.5 h-3.5" />}
              <span>{isMasked ? '显示明文' : '脱敏遮蔽'}</span>
            </button>
            <button
              onClick={handleExportKeysCSV}
              className="px-3 py-2 bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-300 rounded-xl text-xs font-semibold shadow-2xs flex items-center space-x-1.5 transition cursor-pointer"
              title="导出当前密钥列表为 CSV"
            >
              <FileSpreadsheet className="w-3.5 h-3.5 text-emerald-600" />
              <span>导出清单</span>
            </button>
            <button
              onClick={handleOpenCreateKey}
              className="px-4 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-semibold shadow-sm flex items-center space-x-1.5 transition cursor-pointer"
            >
              <Plus className="w-4 h-4" />
              <span>新建密钥</span>
            </button>
          </div>
        </div>

        {/* Filter Bar */}
        <div className="flex flex-wrap items-center justify-between pt-3 border-t border-slate-100 dark:border-slate-800/80 gap-3 text-xs">
          <div className="relative flex-1 max-w-sm">
            <Search className="w-3.5 h-3.5 text-slate-400 absolute left-3 top-2.5" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="搜索密钥别名 / 密钥 token / 计费分组..."
              className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl pl-8 pr-3 py-1.5 text-xs text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500"
            />
          </div>

          <div className="flex items-center space-x-2">
            <span className="text-slate-500 dark:text-slate-400 font-medium">状态:</span>
            <div className="inline-flex bg-slate-100 dark:bg-slate-900 p-0.5 rounded-xl border border-slate-200/80 dark:border-slate-800">
              <button
                onClick={() => setStatusFilter('all')}
                className={`px-2.5 py-1 rounded-lg text-xs font-medium transition cursor-pointer ${
                  statusFilter === 'all'
                    ? 'bg-white dark:bg-slate-800 text-indigo-600 dark:text-indigo-400 shadow-2xs font-semibold'
                    : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200'
                }`}
              >
                全部 ({keys.length})
              </button>
              <button
                onClick={() => setStatusFilter('active')}
                className={`px-2.5 py-1 rounded-lg text-xs font-medium transition cursor-pointer ${
                  statusFilter === 'active'
                    ? 'bg-white dark:bg-slate-800 text-emerald-600 dark:text-emerald-400 shadow-2xs font-semibold'
                    : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200'
                }`}
              >
                正常 ({activeCount})
              </button>
              <button
                onClick={() => setStatusFilter('disabled')}
                className={`px-2.5 py-1 rounded-lg text-xs font-medium transition cursor-pointer ${
                  statusFilter === 'disabled'
                    ? 'bg-white dark:bg-slate-800 text-amber-600 dark:text-amber-400 shadow-2xs font-semibold'
                    : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200'
                }`}
              >
                已停用 ({keys.length - activeCount})
              </button>
            </div>
          </div>
        </div>
      </div>

      {/* Keys Content */}
      {keys.length === 0 ? (
        <div className="bg-white dark:bg-[#111726] border border-dashed border-slate-300 dark:border-slate-800 rounded-3xl p-12 text-center flex flex-col items-center justify-center space-y-3">
          <div className="w-12 h-12 rounded-2xl bg-amber-50 dark:bg-amber-950/60 border border-amber-200 dark:border-amber-800/80 flex items-center justify-center text-amber-600 dark:text-amber-400 shadow-sm">
            <Key className="w-6 h-6" />
          </div>
          <h4 className="font-bold text-sm text-slate-900 dark:text-slate-100">暂无 API 密钥</h4>
          <p className="text-xs text-slate-500 max-w-sm">
            API 密钥是业务客户端调用大模型网关的身份凭证，创建后即可分配至对应系统使用。
          </p>
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
                      checked={filteredKeys.length > 0 && selectedKeyIds.length === filteredKeys.length}
                      onChange={(e) => {
                        if (e.target.checked) {
                          setSelectedKeyIds(filteredKeys.map((k) => k.id));
                        } else {
                          setSelectedKeyIds([]);
                        }
                      }}
                    />
                  </th>
                  <th className="py-3.5 px-5 font-semibold">密钥别名</th>
                  <th className="py-3.5 px-5 font-semibold">API Key 凭据</th>
                  <th className="py-3.5 px-5 font-semibold">
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
                  <th className="py-3.5 px-5 font-semibold">已用 / 额度上限</th>
                  <th className="py-3.5 px-5 font-semibold">授权模型范围</th>
                  <th className="py-3.5 px-5 font-semibold">速率限流</th>
                  <th className="py-3.5 px-5 font-semibold">状态</th>
                  <th className="py-3.5 px-5 text-right font-semibold">操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800/60 text-sm">
                {filteredKeys.map((k) => (
                  <tr key={k.id} className="hover:bg-slate-50/60 dark:hover:bg-slate-800/40 transition">
                    <td className="py-3.5 px-4 text-center">
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
                    <td className="py-3.5 px-5 text-slate-800 dark:text-slate-100 font-semibold text-xs">
                      {k.tenant_id}
                    </td>
                    <td className="py-3.5 px-5 font-mono text-xs text-indigo-700 dark:text-indigo-400 font-semibold">
                      <div className="flex items-center space-x-1.5">
                        <span className="select-all">{maskKeyString(k.key)}</span>
                        <button
                          onClick={() => copyToClipboard(k.key)}
                          className="p-1 hover:bg-indigo-50 dark:hover:bg-indigo-950/60 rounded-lg text-slate-400 hover:text-indigo-600 dark:hover:text-indigo-300 transition cursor-pointer"
                          title="复制完整 API Key"
                        >
                          {copiedKey === k.key ? (
                            <Check className="w-3.5 h-3.5 text-emerald-500" />
                          ) : (
                            <Copy className="w-3.5 h-3.5" />
                          )}
                        </button>
                      </div>
                    </td>
                    <td className="py-3.5 px-5 text-xs">
                      {canUserSwitch ? (
                        <select
                          value={k.group_name || 'default'}
                          onChange={(e) => handleUpdateKeyGroup && handleUpdateKeyGroup(k.id, e.target.value)}
                          className="bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded-lg px-2 py-1 text-[11px] font-semibold text-slate-800 dark:text-slate-200 focus:outline-none focus:border-indigo-500 cursor-pointer shadow-2xs hover:border-slate-300 dark:hover:border-slate-600 transition"
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
                          className={`inline-flex items-center px-2 py-0.5 rounded-lg text-[11px] font-semibold border ${getGroupBadge(k.group_name).className}`}
                        >
                          {getGroupBadge(k.group_name).label}
                        </span>
                      )}
                    </td>
                    <td className="py-3.5 px-5 text-xs font-mono">
                      <span className="text-slate-900 dark:text-slate-100 font-bold">
                        ¥{Number(k.used_cost || 0).toFixed(4)}
                      </span>
                      <span className="text-slate-400 mx-1">/</span>
                      <span className={k.budget > 0 ? 'text-indigo-600 dark:text-indigo-400 font-medium' : 'text-slate-400'}>
                        {k.budget > 0 ? `¥${Number(k.budget).toFixed(2)}` : '不限'}
                      </span>
                    </td>
                    <td className="py-3.5 px-5 text-xs text-slate-600 dark:text-slate-300 font-medium">
                      {!k.allowed_models || k.allowed_models.length === 0 ? (
                        <span className="inline-flex items-center px-2 py-0.5 rounded-full text-[11px] bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800/60 font-semibold">
                          全部允许
                        </span>
                      ) : (
                        <span className="font-mono text-[11px] bg-slate-100 dark:bg-slate-800 px-2 py-0.5 rounded border border-slate-200 dark:border-slate-700">
                          {k.allowed_models.join(', ')}
                        </span>
                      )}
                    </td>
                    <td className="py-3.5 px-5 text-slate-600 dark:text-slate-400 font-mono text-xs">
                      {k.rpm ? `${k.rpm} RPM` : '不限'}
                    </td>
                    <td className="py-3.5 px-5">
                      {k.status === 'disabled' ? (
                        <span className="px-2 py-0.5 rounded-full text-xs bg-rose-50 dark:bg-rose-950/50 text-rose-700 dark:text-rose-300 border border-rose-200 dark:border-rose-800/60 font-medium inline-flex items-center space-x-1.5">
                          <span className="w-1.5 h-1.5 rounded-full bg-rose-500 animate-pulse"></span>
                          <span>已停用</span>
                        </span>
                      ) : (
                        <span className="px-2 py-0.5 rounded-full text-xs bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800/60 font-medium inline-flex items-center space-x-1.5">
                          <span className="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>
                          <span>正常</span>
                        </span>
                      )}
                    </td>
                    <td className="py-3.5 px-5 text-right space-x-1.5">
                      <button
                        onClick={() => copyCurlSnippet(k.key)}
                        className="text-xs px-2.5 py-1 rounded-xl bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 text-slate-700 dark:text-slate-300 border border-slate-200 dark:border-slate-700 font-medium transition inline-flex items-center space-x-1 cursor-pointer"
                        title="复制 cURL 快速接入命令"
                      >
                        <Terminal className="w-3 h-3 text-indigo-500" />
                        <span>cURL</span>
                      </button>
                      <button
                        onClick={() => handleToggleKeyStatus(k)}
                        className={`text-xs px-2.5 py-1 rounded-xl border font-medium transition cursor-pointer ${
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
                        className="text-xs px-2.5 py-1 rounded-xl bg-indigo-50 dark:bg-indigo-950/50 text-indigo-700 dark:text-indigo-300 hover:bg-indigo-100 dark:hover:bg-indigo-900/60 border border-indigo-200 dark:border-indigo-800/60 font-medium transition inline-flex items-center space-x-1 cursor-pointer"
                      >
                        <Sparkles className="w-3 h-3 text-indigo-500" />
                        <span>快速接入</span>
                      </button>
                      <button
                        onClick={() => handleDeleteKey(k.id)}
                        className="text-xs px-2 py-1 text-rose-600 dark:text-rose-400 hover:text-rose-800 dark:hover:text-rose-300 transition font-medium cursor-pointer"
                      >
                        删除
                      </button>
                    </td>
                  </tr>
                ))}
                {filteredKeys.length === 0 && (
                  <tr>
                    <td colSpan="9" className="py-10 text-center text-slate-400 text-xs">
                      没有找到匹配的 API 密钥
                      {(searchQuery || statusFilter !== 'all') && (
                        <button
                          onClick={() => { setSearchQuery(''); setStatusFilter('all'); }}
                          className="ml-2 text-indigo-600 hover:underline font-semibold"
                        >
                          清除筛选
                        </button>
                      )}
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}
