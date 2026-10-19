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
import { getGatewayOrigin, copyToClipboard, toCSV } from '../config';

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
  onNavigateToPlayground,
  lang = 'zh',
  t
}) {
  const isZh = lang === 'zh';
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
        label: isZh ? 'VIP 组' : 'VIP Group',
        className: 'bg-amber-50 dark:bg-amber-950/40 text-amber-800 dark:text-amber-300 border-amber-200 dark:border-amber-800/60'
      };
    }
    if (g === 'enterprise') {
      return {
        label: isZh ? '企业组' : 'Enterprise Group',
        className: 'bg-purple-50 dark:bg-purple-950/40 text-purple-800 dark:text-purple-300 border-purple-200 dark:border-purple-800/60'
      };
    }
    if (g === 'default') {
      return {
        label: isZh ? '默认组' : 'Default Group',
        className: 'bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300 border-slate-200 dark:border-slate-700'
      };
    }
    return {
      label: isZh ? `${group} 组` : `${group} Group`,
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
        label: g === 'default'
          ? (isZh ? '默认组 (default)' : 'Default (default)')
          : g === 'vip'
          ? (isZh ? 'VIP 组 (vip)' : 'VIP Group (vip)')
          : g === 'enterprise'
          ? (isZh ? '企业组 (enterprise)' : 'Enterprise (enterprise)')
          : (isZh ? `${g} 组` : `${g} Group`)
      }));
    }
    if (userGuaranteedGroup) {
      const set = new Set(['default', userGuaranteedGroup]);
      if (currentKeyGroup) set.add(currentKeyGroup);
      return Array.from(set).map(g => ({
        id: g,
        label: g === 'default'
          ? (isZh ? '默认标准组 (default)' : 'Standard Default (default)')
          : (isZh ? `${g} 组 (账号专属)` : `${g} Group (Dedicated)`)
      }));
    }
    return [
      { id: currentKeyGroup || 'default', label: currentKeyGroup && currentKeyGroup !== 'default' ? (isZh ? `${currentKeyGroup} 组` : `${currentKeyGroup} Group`) : (isZh ? '默认组' : 'Default Group') }
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
      if (showToast) showToast(isZh ? '当前没有可导出的密钥' : 'No keys available for export', 'warning');
      return;
    }
    const headers = isZh ? [
      'ID', '密钥别名', 'API Key', '计费分组', '已用额度(元)', '预算上限(元)', '授权模型', '速率限制(RPM)', '状态', '创建时间'
    ] : [
      'ID', 'Alias', 'API Key', 'Billing Group', 'Used Cost', 'Budget Cap', 'Allowed Models', 'Rate Limit (RPM)', 'Status', 'Created At'
    ];
    const rows = filteredKeys.map(k => [
      k.id,
      k.tenant_id || '',
      k.key || '',
      k.group_name || 'default',
      (k.used_cost || 0).toFixed(4),
      k.budget > 0 ? k.budget.toFixed(2) : (isZh ? '不限' : 'Unlimited'),
      (k.allowed_models || []).join(';') || '*',
      k.rpm || (isZh ? '不限' : 'Unlimited'),
      k.status === 'disabled' ? (isZh ? '已停用' : 'Disabled') : (isZh ? '正常' : 'Active'),
      k.created_at || ''
    ]);
    const csvContent = toCSV(headers, rows);
    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `airoute-api-keys-${new Date().toISOString().slice(0, 10)}.csv`;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
    if (showToast) showToast(isZh ? `成功导出 ${filteredKeys.length} 个密钥数据` : `Successfully exported ${filteredKeys.length} keys`, 'success');
  };

  // Copy cURL sample snippet
  const copyCurlSnippet = async (keyStr) => {
    const origin = getGatewayOrigin();
    const curl = `curl ${origin}/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer ${keyStr}" \\
  -d '{
    "model": "deepseek-v4-flash",
    "messages": [{"role": "user", "content": "Hello Airoute!"}]
  }'`;
    await copyToClipboard(curl);
    if (showToast) showToast(isZh ? 'cURL 调用示例已复制到剪贴板' : 'cURL example copied to clipboard', 'success');
  };

  const activeCount = keys.filter(k => k.status !== 'disabled').length;
  const totalCost = keys.reduce((acc, k) => acc + Number(k.used_cost || 0), 0);

  return (
    <div className="space-y-8">
      {/* Page Header (Directly on canvas) */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <div className="flex items-center space-x-3">
            <h1 className="text-2xl font-semibold tracking-tight text-slate-900 dark:text-white">
              {isZh ? 'API 密钥管理' : 'API Keys'}
            </h1>
            <span className="text-xs font-mono tabular-nums px-2.5 py-0.5 rounded-full bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-300">
              {keys.length} {isZh ? '个密钥' : 'keys'} · {activeCount} {isZh ? '生效中' : 'active'}
            </span>
          </div>
          <p className="text-sm text-slate-500 dark:text-slate-400 mt-1">
            {isZh ? '用于各业务客户端与系统调用接入鉴权、独立计费及模型级权限隔离' : 'Authentication, group pricing, rate limits, and model access scopes for client applications'}
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2.5">
          {onNavigateToPricing && (
            <button
              onClick={onNavigateToPricing}
              className="px-3.5 py-2 bg-white dark:bg-slate-900 hover:bg-slate-50 dark:hover:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-200/80 dark:border-slate-800 rounded-lg text-xs font-medium shadow-sm flex items-center space-x-1.5 transition cursor-pointer"
              title={isZh ? '查看各计费分组的模型定价明细' : 'View model pricing rates per group'}
            >
              <DollarSign className="w-3.5 h-3.5 text-emerald-500" />
              <span>{isZh ? '组内费率明细' : 'Pricing Rates'}</span>
            </button>
          )}
          <button
            onClick={() => setIsMasked(!isMasked)}
            className="px-3.5 py-2 bg-white dark:bg-slate-900 hover:bg-slate-50 dark:hover:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-200/80 dark:border-slate-800 rounded-lg text-xs font-medium shadow-sm flex items-center space-x-1.5 transition cursor-pointer"
            title={isMasked ? (isZh ? '显示明文密钥' : 'Show plaintext key') : (isZh ? '脱敏隐藏密钥' : 'Mask key')}
          >
            {isMasked ? <Eye className="w-3.5 h-3.5 text-slate-400" /> : <EyeOff className="w-3.5 h-3.5 text-slate-400" />}
            <span>{isMasked ? (isZh ? '显示明文' : 'Show Key') : (isZh ? '脱敏遮蔽' : 'Mask Key')}</span>
          </button>
          <button
            onClick={handleExportKeysCSV}
            className="px-3.5 py-2 bg-white dark:bg-slate-900 hover:bg-slate-50 dark:hover:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-200/80 dark:border-slate-800 rounded-lg text-xs font-medium shadow-sm flex items-center space-x-1.5 transition cursor-pointer"
            title={isZh ? '导出当前密钥列表为 CSV' : 'Export keys as CSV'}
          >
            <FileSpreadsheet className="w-3.5 h-3.5 text-slate-400" />
            <span>{isZh ? '导出清单' : 'Export CSV'}</span>
          </button>
          <button
            onClick={handleOpenCreateKey}
            className="px-4 py-2 bg-slate-900 dark:bg-indigo-600 hover:bg-slate-800 dark:hover:bg-indigo-500 text-white rounded-lg text-xs font-medium shadow-sm flex items-center space-x-1.5 transition cursor-pointer"
          >
            <Plus className="w-3.5 h-3.5" />
            <span>{isZh ? '新建密钥' : 'Create Key'}</span>
          </button>
        </div>
      </div>

      {/* Filter Bar (Directly on canvas) */}
      <div className="flex flex-wrap items-center justify-between gap-3 text-xs">
        <div className="relative flex-1 max-w-sm">
          <Search className="w-3.5 h-3.5 text-slate-400 absolute left-3 top-2.5" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder={isZh ? '搜索密钥别名 / 密钥 token / 计费分组...' : 'Search tenant, key token, group...'}
            className="w-full bg-white dark:bg-[#0F1420] border border-slate-200/80 dark:border-slate-800 rounded-lg pl-9 pr-3 py-1.5 text-xs text-slate-800 dark:text-slate-100 placeholder-slate-400 focus:outline-none focus:border-indigo-500 transition shadow-sm"
          />
        </div>

        <div className="flex items-center space-x-2">
          <span className="text-slate-500 dark:text-slate-400 font-medium">{isZh ? '状态:' : 'Status:'}</span>
          <div className="inline-flex bg-slate-100 dark:bg-slate-900 p-0.5 rounded-lg border border-slate-200/80 dark:border-slate-800">
            <button
              onClick={() => setStatusFilter('all')}
              className={`px-3 py-1 rounded-md text-xs font-medium transition cursor-pointer ${
                statusFilter === 'all'
                  ? 'bg-white dark:bg-slate-800 text-slate-900 dark:text-white shadow-sm font-semibold'
                  : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200'
              }`}
            >
              {isZh ? '全部' : 'All'} ({keys.length})
            </button>
            <button
              onClick={() => setStatusFilter('active')}
              className={`px-3 py-1 rounded-md text-xs font-medium transition cursor-pointer ${
                statusFilter === 'active'
                  ? 'bg-white dark:bg-slate-800 text-emerald-600 dark:text-emerald-400 shadow-sm font-semibold'
                  : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200'
              }`}
            >
              {isZh ? '正常' : 'Active'} ({activeCount})
            </button>
            <button
              onClick={() => setStatusFilter('disabled')}
              className={`px-3 py-1 rounded-md text-xs font-medium transition cursor-pointer ${
                statusFilter === 'disabled'
                  ? 'bg-white dark:bg-slate-800 text-rose-600 dark:text-rose-400 shadow-sm font-semibold'
                  : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200'
              }`}
            >
              {isZh ? '已停用' : 'Disabled'} ({keys.length - activeCount})
            </button>
          </div>
        </div>
      </div>

      {/* Keys Content */}
      {keys.length === 0 ? (
        <div className="bg-white dark:bg-[#0F1420] border border-dashed border-slate-300 dark:border-slate-800 rounded-2xl p-12 text-center flex flex-col items-center justify-center space-y-3">
          <div className="w-10 h-10 rounded-xl bg-slate-100 dark:bg-slate-800 border border-slate-200 dark:border-slate-700 flex items-center justify-center text-slate-500 shadow-sm">
            <Key className="w-5 h-5" />
          </div>
          <h4 className="font-semibold text-sm text-slate-900 dark:text-slate-100">{isZh ? '暂无 API 密钥' : 'No API Keys'}</h4>
          <p className="text-xs text-slate-500 max-w-sm">
            {isZh ? 'API 密钥是业务客户端调用大模型网关的身份凭证，创建后即可分配至对应系统使用。' : 'API keys authenticate client apps to access the gateway. Create one to get started.'}
          </p>
          <button
            onClick={handleOpenCreateKey}
            className="px-4 py-2 bg-slate-900 dark:bg-indigo-600 hover:bg-slate-800 dark:hover:bg-indigo-500 text-white rounded-lg text-xs font-medium shadow-sm flex items-center space-x-1.5 transition cursor-pointer mt-2"
          >
            <Plus className="w-3.5 h-3.5" />
            <span>{isZh ? '新建密钥' : 'Create Key'}</span>
          </button>
        </div>
      ) : (
        <div className="space-y-4">
          {selectedKeyIds.length > 0 && (
            <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between bg-indigo-50/70 dark:bg-indigo-950/30 border border-indigo-200/80 dark:border-indigo-800/60 px-5 py-3 rounded-xl animate-in fade-in gap-3">
              <div className="flex items-center space-x-2 text-xs font-semibold text-indigo-900 dark:text-indigo-200">
                <span>{isZh ? `已选中 ${selectedKeyIds.length} 个密钥` : `Selected ${selectedKeyIds.length} keys`}</span>
              </div>
              <div className="flex items-center space-x-2">
                <button
                  onClick={() => handleBatchStatusKeys('active')}
                  className="px-3 py-1.5 bg-emerald-600 hover:bg-emerald-700 text-white text-xs font-medium rounded-lg shadow-sm transition cursor-pointer"
                >
                  {isZh ? '批量启用' : 'Batch Enable'}
                </button>
                <button
                  onClick={() => handleBatchStatusKeys('disabled')}
                  className="px-3 py-1.5 bg-amber-600 hover:bg-amber-700 text-white text-xs font-medium rounded-lg shadow-sm transition cursor-pointer"
                >
                  {isZh ? '批量停用' : 'Batch Disable'}
                </button>
                <button
                  onClick={handleBatchDeleteKeys}
                  className="px-3 py-1.5 bg-rose-600 hover:bg-rose-700 text-white text-xs font-medium rounded-lg shadow-sm transition flex items-center space-x-1 cursor-pointer"
                >
                  <Trash2 className="w-3.5 h-3.5" />
                  <span>{isZh ? '批量注销' : 'Batch Delete'}</span>
                </button>
                <button
                  onClick={() => setSelectedKeyIds([])}
                  className="px-3 py-1.5 bg-white dark:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-200 dark:border-slate-700 text-xs font-medium rounded-lg hover:bg-slate-50 transition cursor-pointer"
                >
                  {isZh ? '取消选择' : 'Deselect All'}
                </button>
              </div>
            </div>
          )}

          <div className="bg-white dark:bg-[#0F1420] border border-slate-200/70 dark:border-slate-800/80 rounded-2xl overflow-hidden shadow-sm">
            <div className="overflow-x-auto w-full">
              <table className="w-full text-left border-collapse min-w-[850px]">
              <thead>
                <tr className="border-b border-slate-100 dark:border-slate-800/80 text-slate-500 dark:text-slate-400 text-xs uppercase bg-slate-50/60 dark:bg-slate-900/40">
                  <th className="py-4 px-6 w-10 text-center">
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
                  <th className="py-4 px-6 font-semibold">{isZh ? '密钥别名' : 'Key Name'}</th>
                  <th className="py-4 px-6 font-semibold">{isZh ? 'API Key 凭据' : 'API Key Token'}</th>
                  <th className="py-4 px-6 font-semibold">
                    <div className="flex items-center space-x-1">
                      <span>{isZh ? '计费分组' : 'Pricing Group'}</span>
                      {onNavigateToPricing && (
                        <button
                          type="button"
                          onClick={onNavigateToPricing}
                          className="text-[10px] text-indigo-600 dark:text-indigo-400 hover:underline font-normal cursor-pointer ml-0.5"
                          title={isZh ? '查看各分组费率详情' : 'View group rates'}
                        >
                          {isZh ? '(费率明细)' : '(Rates)'}
                        </button>
                      )}
                    </div>
                  </th>
                  <th className="py-4 px-6 font-semibold">{isZh ? '已用 / 额度上限' : 'Used / Budget'}</th>
                  <th className="py-4 px-6 font-semibold">{isZh ? '授权模型范围' : 'Allowed Models'}</th>
                  <th className="py-4 px-6 font-semibold">{isZh ? '速率限流' : 'Rate Limit'}</th>
                  <th className="py-4 px-6 font-semibold">{isZh ? '状态' : 'Status'}</th>
                  <th className="py-4 px-6 text-right font-semibold">{isZh ? '操作' : 'Actions'}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800/70 text-sm">
                {filteredKeys.map((k) => (
                  <tr key={k.id} className="hover:bg-slate-50/60 dark:hover:bg-slate-800/40 transition">
                    <td className="py-4 px-6 text-center">
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
                    <td className="py-4 px-6 text-slate-800 dark:text-slate-100 font-medium text-xs">
                      {k.tenant_id}
                    </td>
                    <td className="py-4 px-6 font-mono text-xs text-indigo-700 dark:text-indigo-400">
                      <div className="flex items-center space-x-1.5">
                        <span className="select-all">{maskKeyString(k.key)}</span>
                        <button
                          onClick={() => copyToClipboard(k.key)}
                          className="p-1 hover:bg-indigo-50 dark:hover:bg-indigo-950/60 rounded-md text-slate-400 hover:text-indigo-600 dark:hover:text-indigo-300 transition cursor-pointer"
                          title={isZh ? "复制完整 API Key" : "Copy full API Key"}
                        >
                          {copiedKey === k.key ? (
                            <Check className="w-3.5 h-3.5 text-emerald-500" />
                          ) : (
                            <Copy className="w-3.5 h-3.5" />
                          )}
                        </button>
                      </div>
                    </td>
                    <td className="py-4 px-6 text-xs">
                      {canUserSwitch ? (
                        <select
                          value={k.group_name || 'default'}
                          onChange={(e) => handleUpdateKeyGroup && handleUpdateKeyGroup(k.id, e.target.value)}
                          className="bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded-lg px-2 py-1 text-[11px] font-medium text-slate-800 dark:text-slate-200 focus:outline-none focus:border-indigo-500 cursor-pointer hover:border-slate-300 dark:hover:border-slate-600 transition"
                          title={isZh ? "切换该密钥的扣费分组" : "Change billing group"}
                        >
                          {getGroupOptions(k.group_name).map(opt => (
                            <option key={opt.id} value={opt.id}>
                              {opt.label}
                            </option>
                          ))}
                        </select>
                      ) : (
                        <span
                          className={`inline-flex items-center px-2 py-0.5 rounded-md text-[11px] font-medium border ${getGroupBadge(k.group_name).className}`}
                        >
                          {getGroupBadge(k.group_name).label}
                        </span>
                      )}
                    </td>
                    <td className="py-4 px-6 text-xs font-mono tabular-nums">
                      <span className="text-slate-900 dark:text-slate-100 font-semibold">
                        ¥{Number(k.used_cost || 0).toFixed(4)}
                      </span>
                      <span className="text-slate-400 mx-1">/</span>
                      <span className={k.budget > 0 ? 'text-indigo-600 dark:text-indigo-400 font-medium' : 'text-slate-400'}>
                        {k.budget > 0 ? `¥${Number(k.budget).toFixed(2)}` : (isZh ? '不限' : 'Unlimited')}
                      </span>
                    </td>
                    <td className="py-4 px-6 text-xs text-slate-600 dark:text-slate-300 font-medium">
                      {!k.allowed_models || k.allowed_models.length === 0 ? (
                        <span className="inline-flex items-center px-2 py-0.5 rounded-full text-[11px] bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-300 font-medium">
                          {isZh ? '全部允许' : 'All Allowed'}
                        </span>
                      ) : (
                        <span className="font-mono text-[11px] bg-slate-100 dark:bg-slate-800 px-2 py-0.5 rounded border border-slate-200 dark:border-slate-700">
                          {k.allowed_models.join(', ')}
                        </span>
                      )}
                    </td>
                    <td className="py-4 px-6 text-slate-600 dark:text-slate-400 font-mono tabular-nums text-xs">
                      {k.rpm ? `${k.rpm} RPM` : (isZh ? '不限' : 'Unlimited')}
                    </td>
                    <td className="py-4 px-6">
                      {k.status === 'disabled' ? (
                        <span className="inline-flex items-center space-x-1.5 text-xs text-rose-600 dark:text-rose-400 font-medium">
                          <span className="w-1.5 h-1.5 rounded-full bg-rose-500"></span>
                          <span>{isZh ? '已停用' : 'Disabled'}</span>
                        </span>
                      ) : (
                        <span className="inline-flex items-center space-x-1.5 text-xs text-emerald-600 dark:text-emerald-400 font-medium">
                          <span className="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>
                          <span>{isZh ? '正常' : 'Active'}</span>
                        </span>
                      )}
                    </td>
                    <td className="py-4 px-6 text-right space-x-1.5">
                      <button
                        onClick={() => copyCurlSnippet(k.key)}
                        className="text-xs px-2.5 py-1 rounded-lg bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-300 font-medium transition inline-flex items-center space-x-1 cursor-pointer"
                        title={isZh ? '复制 cURL 快速接入命令' : 'Copy cURL command'}
                      >
                        <Terminal className="w-3 h-3 text-slate-400" />
                        <span>cURL</span>
                      </button>
                      <button
                        onClick={() => handleToggleKeyStatus(k)}
                        className={`text-xs px-2.5 py-1 rounded-lg border font-medium transition cursor-pointer ${
                          k.status === 'disabled'
                            ? 'bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 hover:bg-emerald-100 border-emerald-200 dark:border-emerald-800/60'
                            : 'bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300 hover:bg-slate-200 border-slate-200 dark:border-slate-700'
                        }`}
                        title={k.status === 'disabled' ? (isZh ? '恢复启用' : 'Enable') : (isZh ? '暂停使用' : 'Disable')}
                      >
                        {k.status === 'disabled' ? (isZh ? '启用' : 'Enable') : (isZh ? '停用' : 'Disable')}
                      </button>
                      <button
                        onClick={() => setActiveQuickKey(k)}
                        className="text-xs px-2.5 py-1 rounded-lg bg-indigo-50 dark:bg-indigo-950/50 text-indigo-700 dark:text-indigo-300 hover:bg-indigo-100 dark:hover:bg-indigo-900/60 border border-indigo-200 dark:border-indigo-800/60 font-medium transition inline-flex items-center space-x-1 cursor-pointer"
                      >
                        <Sparkles className="w-3 h-3 text-indigo-500" />
                        <span>{isZh ? '快速接入' : 'Quick Connect'}</span>
                      </button>
                      <button
                        onClick={() => handleDeleteKey(k.id)}
                        className="text-xs px-2 py-1 text-rose-600 dark:text-rose-400 hover:text-rose-800 dark:hover:text-rose-300 transition font-medium cursor-pointer"
                      >
                        {isZh ? '删除' : 'Delete'}
                      </button>
                    </td>
                  </tr>
                ))}
                {filteredKeys.length === 0 && (
                  <tr>
                    <td colSpan="9" className="py-12 text-center text-slate-400 text-xs">
                      {isZh ? '没有找到匹配的 API 密钥' : 'No matching API keys found'}
                      {(searchQuery || statusFilter !== 'all') && (
                        <button
                          onClick={() => { setSearchQuery(''); setStatusFilter('all'); }}
                          className="ml-2 text-indigo-600 hover:underline font-medium"
                        >
                          {isZh ? '清除筛选' : 'Clear Filters'}
                        </button>
                      )}
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
