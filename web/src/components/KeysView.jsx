import { useMemo } from 'react';
import {
  Key,
  Plus,
  Trash2,
  Copy,
  Check,
  Sparkles,
  DollarSign,
  Search,
  FileSpreadsheet,
  Terminal,
  Activity
} from 'lucide-react';
import { getGatewayOrigin, toCSV } from '../config';
import useTableControls from '../hooks/useTableControls';
import useUrlFilters from '../hooks/useUrlFilters';
import { SortableTh, ColumnToggle } from './TableControls';
import { availableKeyModels } from '../lib/keyModels';
import { inferClientModality } from '../lib/modelModality';

// Low-priority columns are dropped at narrow widths so that name, status and
// actions stay reachable on a phone-width viewport without horizontal hunting.
const COLUMN_CLASS = {
  token: 'hidden md:table-cell',
  group: 'hidden md:table-cell',
  usage: 'hidden sm:table-cell',
  budget: 'hidden sm:table-cell',
  models: 'hidden lg:table-cell',
  rpm: 'hidden md:table-cell'
};

export default function KeysView({
  keys = [],
  models = [],
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
  groupMultipliers = {},
  onNavigateToPricing,
  onNavigateToDashboard,
  showToast,
  onNavigateToPlayground,
  lang = 'zh',
  t
}) {
  const isZh = lang === 'zh';
  // Filters live in the URL (keys.q / keys.status) so they survive module
  // switches and the current view can be shared as a link.
  const [filters, setFilters, resetFilters] = useUrlFilters('keys', { q: '', status: 'all' });
  const searchQuery = filters.q || '';
  const statusFilter = filters.status || 'all'; // 'all' | 'active' | 'disabled'

  const columns = useMemo(() => [
    { key: 'name', label: isZh ? '密钥别名' : 'Key Name', sortValue: (k) => k.tenant_id || '' },
    { key: 'token', label: isZh ? 'API Key 凭据' : 'API Key Token', sortable: false },
    { key: 'group', label: isZh ? '计费分组' : 'Pricing Group', sortValue: (k) => (k.group_name || 'default').toLowerCase() },
    { key: 'usage', label: isZh ? '已用额度' : 'Spent', sortValue: (k) => Number(k.used_cost || 0) },
    // Unlimited budgets/rpm sort as the largest finite value so they cluster
    // at one end without producing NaN comparisons.
    { key: 'budget', label: isZh ? '预算上限' : 'Budget Cap', sortValue: (k) => (Number(k.budget) > 0 ? Number(k.budget) : Number.MAX_SAFE_INTEGER) },
    { key: 'models', label: isZh ? '授权模型范围' : 'Allowed Models', sortValue: (k) => (k.allowed_models || []).length },
    { key: 'rpm', label: isZh ? '速率限流' : 'Rate Limit', sortValue: (k) => (Number(k.rpm) > 0 ? Number(k.rpm) : Number.MAX_SAFE_INTEGER) },
    { key: 'status', label: isZh ? '状态' : 'Status', sortValue: (k) => (k.status === 'disabled' ? 1 : 0) }
  ], [isZh]);

  // Column visibility is remembered per browser; default sort puts the heaviest
  // spenders first so quota/budget outliers are visible without scanning.
  const controls = useTableControls({
    columns,
    storageKey: 'airoute.keys.columns',
    initialSort: { key: 'usage', dir: 'desc' }
  });
  const { sort, toggleSort, sortRows, isVisible } = controls;

  const isAdmin = adminUser?.role === 'admin';
  const userGuaranteedGroup = adminUser?.group_name && adminUser.group_name !== 'default' ? adminUser.group_name : null;
  const canUserSwitch = isAdmin || !!userGuaranteedGroup;

  const getGroupBadge = (group) => {
    const g = (group || 'default').toLowerCase();
    const mult = groupMultipliers?.[g] ?? (g === 'vip' ? 0.8 : g === 'enterprise' ? 1.5 : 1.0);
    if (g === 'vip') {
      return {
        label: isZh ? `VIP 组 · ${mult}x` : `VIP · ${mult}x`,
        className: 'bg-amber-50 dark:bg-amber-950/40 text-amber-800 dark:text-amber-300 border-amber-200 dark:border-amber-800/60'
      };
    }
    if (g === 'enterprise') {
      return {
        label: isZh ? `企业组 · ${mult}x` : `Enterprise · ${mult}x`,
        className: 'bg-purple-50 dark:bg-purple-950/40 text-purple-800 dark:text-purple-300 border-purple-200 dark:border-purple-800/60'
      };
    }
    if (g === 'default') {
      return {
        label: isZh ? '默认组 · 1.0x' : 'Default · 1.0x',
        className: 'bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300 border-slate-200 dark:border-slate-700'
      };
    }
    return {
      label: isZh ? `${group} 组 · ${mult}x` : `${group} · ${mult}x`,
      className: 'bg-sky-50 dark:bg-sky-950/40 text-sky-800 dark:text-sky-300 border-sky-200 dark:border-sky-800/60'
    };
  };

  const getGroupOptions = (currentKeyGroup) => {
    if (isAdmin) {
      const set = new Set(['default']);
      (pricingGroups || []).forEach(g => set.add(g));
      if (currentKeyGroup) set.add(currentKeyGroup);
      return Array.from(set).map(g => {
        const mult = groupMultipliers?.[g] ?? (g === 'vip' ? 0.8 : g === 'enterprise' ? 1.5 : 1.0);
        return {
          id: g,
          label: g === 'default'
            ? (isZh ? '默认组 (default · 1.0x 基准)' : 'Default (1.0x)')
            : g === 'vip'
            ? (isZh ? `VIP 组 (vip · ${mult}x 8折)` : `VIP (${mult}x)`)
            : g === 'enterprise'
            ? (isZh ? `企业组 (enterprise · ${mult}x)` : `Enterprise (${mult}x)`)
            : (isZh ? `${g} 组 (${mult}x)` : `${g} Group (${mult}x)`)
        };
      });
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

  // A stored credential is never rendered. The server exposes a stable, non-reversible
  // fingerprint (`kfp_…`) for exactly this purpose; rows created before the column
  // existed fall back to an opaque mask with no characters from the key itself.
  const maskKeyString = (kStr) => (kStr ? 'sk-airoute-••••••••••••' : '-');
  const keyIdentifier = (k) => k.key_fingerprint || maskKeyString(k.key);

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

  const sortedKeys = sortRows(filteredKeys);
  const allVisibleSelected = sortedKeys.length > 0 && sortedKeys.every((k) => selectedKeyIds.includes(k.id));

  // Export keys to CSV
  const handleExportKeysCSV = () => {
    if (sortedKeys.length === 0) {
      if (showToast) showToast(isZh ? '当前没有可导出的密钥' : 'No keys available for export', 'warning');
      return;
    }
    const headers = isZh ? [
      'ID', '密钥别名', '密钥指纹', '计费分组', '已用额度(元)', '预算上限(元)', '授权模型', '速率限制(RPM)', '状态', '创建时间'
    ] : [
      'ID', 'Alias', 'Key Fingerprint', 'Billing Group', 'Used Cost', 'Budget Cap', 'Allowed Models', 'Rate Limit (RPM)', 'Status', 'Created At'
    ];
    const rows = sortedKeys.map(k => [
      k.id,
      k.tenant_id || '',
      // Never export a stored credential: the fingerprint identifies the key.
      keyIdentifier(k),
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
    if (showToast) showToast(isZh ? `成功导出 ${sortedKeys.length} 个密钥数据` : `Successfully exported ${sortedKeys.length} keys`, 'success');
  };

  // Copy cURL sample snippet
  const copyCurlSnippet = async (key) => {
    const model = availableKeyModels(key, models).find(name => inferClientModality(name) === 'chat');
    if (!model) {
      showToast?.(isZh ? '该密钥暂无可用聊天模型，请检查授权范围和模型配置' : 'No available chat model for this key. Check model access and configuration.', 'warning');
      return;
    }
    // The stored credential is not readable from the console (plaintext is shown
    // once, at creation), so the snippet carries a placeholder plus the fingerprint
    // that identifies which key to paste in.
    const keyStr = '<YOUR_API_KEY>';
    const fingerprint = key.key_fingerprint || '';
    const origin = getGatewayOrigin();
    const curl = `${fingerprint ? `# key fingerprint: ${fingerprint}\n` : ''}curl ${origin}/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer ${keyStr}" \\
  -d '{
    "model": "${model}",
    "messages": [{"role": "user", "content": "Hello Airoute!"}]
  }'`;
    // copyToClipboard reports whether the write succeeded (modern clipboard API or
    // the execCommand fallback). Claiming success unconditionally left the operator
    // with an empty clipboard and a "copied" toast.
    const copied = await copyToClipboard(curl);
    if (copied === false) {
      showToast?.(isZh ? 'cURL 示例复制失败，请手动选中并复制' : 'Could not copy the cURL example — select it and copy manually', 'error');
      return;
    }
    if (showToast) showToast(isZh ? 'cURL 调用示例已复制到剪贴板' : 'cURL example copied to clipboard', 'success');
  };

  const activeCount = keys.filter(k => k.status !== 'disabled').length;

  return (
    <div className="space-y-6">
      {/* Page Header (Directly on canvas) */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <div className="flex items-center space-x-3">
            <h1 className="ui-page-title">
              {isZh ? 'API 密钥管理' : 'API Keys'}
            </h1>
            <span className="text-xs font-mono tabular-nums px-2.5 py-0.5 rounded-full bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0]">
              {keys.length} {isZh ? '个密钥' : 'keys'} · {activeCount} {isZh ? '生效中' : 'active'}
            </span>
          </div>
          <p className="ui-page-subtitle mt-1">
            {isZh ? '客户端鉴权、分组计费与模型权限' : 'Auth, group pricing and model scopes'}
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2.5">
          {onNavigateToPricing && (
            <button
              onClick={onNavigateToPricing}
              className="ui-btn-secondary"
              title={isZh ? '查看各计费分组的模型定价明细' : 'View model pricing rates per group'}
            >
              <DollarSign className="w-3.5 h-3.5 text-emerald-500" />
              <span>{isZh ? '组内费率明细' : 'Pricing Rates'}</span>
            </button>
          )}
          <button
            onClick={handleExportKeysCSV}
            className="ui-btn-secondary"
            title={isZh ? '导出当前密钥列表为 CSV' : 'Export keys as CSV'}
          >
            <FileSpreadsheet className="w-3.5 h-3.5 text-[#656B75] dark:text-[#A1A7B0]" />
            <span>{isZh ? '导出清单' : 'Export CSV'}</span>
          </button>
          <button
            onClick={handleOpenCreateKey}
            className="ui-btn-primary"
          >
            <Plus className="w-3.5 h-3.5" />
            <span>{isZh ? '新建密钥' : 'Create Key'}</span>
          </button>
        </div>
      </div>

      {/* Filter Bar (Directly on canvas) */}
      <div className="flex flex-wrap items-center justify-between gap-3 text-xs">
        <div className="relative flex-1 max-w-sm">
          <Search className="w-3.5 h-3.5 text-[#656B75] dark:text-[#A1A7B0] absolute left-3 top-1/2 -translate-y-1/2" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setFilters({ q: e.target.value })}
            placeholder={isZh ? '搜索密钥别名 / 密钥 token / 计费分组...' : 'Search tenant, key token, group...'}
            className="ui-field pl-9"
          />
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <span className="ui-meta font-medium">{isZh ? '状态:' : 'Status:'}</span>
          <div className="inline-flex bg-[#F0F1F2] dark:bg-[#22252B] p-0.5 rounded-lg border border-[#E4E6E9] dark:border-[#2C3038]">
            <button
              onClick={() => setFilters({ status: 'all' })}
              className={`px-3 py-1 rounded-md text-xs font-medium transition-colors cursor-pointer ${
                statusFilter === 'all'
                  ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-2xs font-semibold'
                  : 'text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'
              }`}
            >
              {isZh ? '全部' : 'All'} ({keys.length})
            </button>
            <button
              onClick={() => setFilters({ status: 'active' })}
              className={`px-3 py-1 rounded-md text-xs font-medium transition-colors cursor-pointer ${
                statusFilter === 'active'
                  ? 'bg-white dark:bg-[#191B1F] text-emerald-600 dark:text-emerald-400 shadow-2xs font-semibold'
                  : 'text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'
              }`}
            >
              {isZh ? '正常' : 'Active'} ({activeCount})
            </button>
            <button
              onClick={() => setFilters({ status: 'disabled' })}
              className={`px-3 py-1 rounded-md text-xs font-medium transition-colors cursor-pointer ${
                statusFilter === 'disabled'
                  ? 'bg-white dark:bg-[#191B1F] text-rose-600 dark:text-rose-400 shadow-2xs font-semibold'
                  : 'text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'
              }`}
            >
              {isZh ? '已停用' : 'Disabled'} ({keys.length - activeCount})
            </button>
          </div>
          <ColumnToggle controls={controls} lang={lang} />
        </div>
      </div>

      {/* Keys Content */}
      {keys.length === 0 ? (
        <div className="bg-white dark:bg-[#191B1F] border border-dashed border-[#E4E6E9] dark:border-[#2C3038] rounded-2xl p-12 text-center flex flex-col items-center justify-center space-y-3">
          <div className="w-10 h-10 rounded-xl bg-slate-100 dark:bg-slate-800 border border-slate-200 dark:border-slate-700 flex items-center justify-center text-slate-400 shadow-sm">
            <Key className="w-5 h-5" />
          </div>
          <h4 className="font-semibold text-sm text-slate-900 dark:text-slate-100">{isZh ? '暂无 API 密钥' : 'No API Keys'}</h4>
          <p className="ui-meta max-w-sm">
            {isZh ? 'API 密钥是业务客户端调用大模型网关的身份凭证，创建后即可分配至对应系统使用。' : 'API keys authenticate client apps to access the gateway. Create one to get started.'}
          </p>
          <button
            onClick={handleOpenCreateKey}
            className="ui-btn-primary mt-2"
          >
            <Plus className="w-3.5 h-3.5" />
            <span>{isZh ? '新建密钥' : 'Create Key'}</span>
          </button>
        </div>
      ) : (
        <div className="space-y-4">
          {selectedKeyIds.length > 0 && (
            <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between bg-[#EEF2FF] dark:bg-[#242D48] border border-[#355CFF]/30 dark:border-[#91A7FF]/30 px-5 py-3 rounded-xl animate-in fade-in gap-3">
              <div className="flex items-center space-x-2 text-xs font-semibold text-[#355CFF] dark:text-[#91A7FF]">
                <span>{isZh ? `已选中 ${selectedKeyIds.length} 个密钥` : `Selected ${selectedKeyIds.length} keys`}</span>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <button
                  onClick={() => handleBatchStatusKeys('active')}
                  className="ui-btn ui-btn-sm bg-emerald-600 text-white hover:bg-emerald-700 shadow-xs"
                >
                  {isZh ? '批量启用' : 'Batch Enable'}
                </button>
                <button
                  onClick={() => handleBatchStatusKeys('disabled')}
                  className="ui-btn ui-btn-sm bg-amber-600 text-white hover:bg-amber-700 shadow-xs"
                >
                  {isZh ? '批量停用' : 'Batch Disable'}
                </button>
                <button
                  onClick={handleBatchDeleteKeys}
                  className="ui-btn-danger ui-btn-sm"
                >
                  <Trash2 className="w-3.5 h-3.5" />
                  <span>{isZh ? '批量注销' : 'Batch Delete'}</span>
                </button>
                <button
                  onClick={() => setSelectedKeyIds([])}
                  className="ui-btn-secondary ui-btn-sm"
                >
                  {isZh ? '取消选择' : 'Deselect All'}
                </button>
              </div>
            </div>
          )}

          <div className="ui-card overflow-hidden">
            {filteredKeys.length === 0 ? (
              <div className="p-12 text-center flex flex-col items-center justify-center space-y-3">
                <div className="w-10 h-10 rounded-xl bg-[#F0F1F2] dark:bg-[#22252B] border border-[#E4E6E9] dark:border-[#2C3038] flex items-center justify-center text-[#656B75] dark:text-[#A1A7B0] shadow-2xs">
                  <Search className="w-5 h-5" />
                </div>
                <h4 className="font-semibold text-sm text-[#202124] dark:text-[#F1F2F4]">
                  {isZh ? '没有匹配的密钥' : 'No matching keys'}
                </h4>
                <p className="ui-meta max-w-sm">
                  {isZh ? '当前筛选条件下没有密钥，可调整搜索词或状态筛选后重试。' : 'No key matches the current search and status filters. Adjust them and try again.'}
                </p>
                <button onClick={resetFilters} className="ui-btn-secondary ui-btn-sm mt-1">
                  {isZh ? '清除筛选' : 'Clear filters'}
                </button>
              </div>
            ) : (
              <div className="overflow-x-auto w-full">
                <table className="w-full text-left border-collapse sm:min-w-[760px]">
                  <thead>
                    <tr className="border-b border-[#E4E6E9] dark:border-[#2C3038] bg-[#F7F7F5] dark:bg-[#191B1F]">
                      <th scope="col" className="ui-th w-10 text-center">
                        <input
                          type="checkbox"
                          className="rounded border-[#E4E6E9] text-[#355CFF] focus:ring-[#355CFF] cursor-pointer"
                          checked={allVisibleSelected}
                          onChange={(e) => {
                            if (e.target.checked) {
                              setSelectedKeyIds(sortedKeys.map((k) => k.id));
                            } else {
                              setSelectedKeyIds([]);
                            }
                          }}
                        />
                      </th>
                      {columns.filter((col) => isVisible(col.key)).map((col) => (
                        <SortableTh
                          key={col.key}
                          column={col}
                          sort={sort}
                          onSort={toggleSort}
                          className={COLUMN_CLASS[col.key] || ''}
                        />
                      ))}
                      <th scope="col" className="ui-th text-right">{isZh ? '操作' : 'Actions'}</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-[#E4E6E9]/60 dark:divide-[#2C3038]/60 text-[13px]">
                    {sortedKeys.map((k) => (
                      <tr key={k.id} className="hover:bg-[#F0F1F2]/70 dark:hover:bg-[#22252B]/60 transition-colors duration-150">
                        <td className="ui-td text-center">
                          <input
                            type="checkbox"
                            className="rounded border-[#E4E6E9] text-[#355CFF] focus:ring-[#355CFF] cursor-pointer"
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
                        {isVisible('name') && (
                          <td className="ui-td font-medium text-[#202124] dark:text-[#F1F2F4]">
                            {k.tenant_id}
                          </td>
                        )}
                        {isVisible('token') && (
                          <td className={`ui-td font-mono text-[#355CFF] dark:text-[#91A7FF] ${COLUMN_CLASS.token}`}>
                            <div className="flex items-center space-x-1.5">
                              <span className="select-all" title={isZh ? '密钥指纹（不可反推密钥）' : 'Key fingerprint (not reversible)'}>
                                {keyIdentifier(k)}
                              </span>
                              <button
                                onClick={async () => {
                                  const value = k.key_fingerprint || keyIdentifier(k);
                                  const copied = await copyToClipboard(value);
                                  if (copied === false) {
                                    showToast?.(isZh ? '复制密钥指纹失败，请手动选中复制' : 'Could not copy the key fingerprint — select it and copy manually', 'error');
                                  }
                                }}
                                className="p-1 hover:bg-[#EEF2FF] dark:hover:bg-[#242D48] rounded-md text-[#656B75] dark:text-[#A1A7B0] hover:text-[#355CFF] dark:hover:text-[#91A7FF] transition-colors cursor-pointer"
                                title={isZh ? '复制密钥指纹' : 'Copy key fingerprint'}
                              >
                                {copiedKey === (k.key_fingerprint || keyIdentifier(k)) ? (
                                  <Check className="w-3.5 h-3.5 text-emerald-500" />
                                ) : (
                                  <Copy className="w-3.5 h-3.5" />
                                )}
                              </button>
                            </div>
                          </td>
                        )}
                        {isVisible('group') && (
                          <td className={`ui-td ${COLUMN_CLASS.group}`}>
                            {canUserSwitch ? (
                              <select
                                value={k.group_name || 'default'}
                                onChange={(e) => handleUpdateKeyGroup && handleUpdateKeyGroup(k.id, e.target.value)}
                                className="ui-field ui-field-sm w-auto cursor-pointer font-medium"
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
                                className={`inline-flex items-center px-2 py-0.5 rounded-md text-xs font-medium border ${getGroupBadge(k.group_name).className}`}
                              >
                                {getGroupBadge(k.group_name).label}
                              </span>
                            )}
                          </td>
                        )}
                        {isVisible('usage') && (
                          <td className={`ui-td font-mono tabular-nums ${COLUMN_CLASS.usage}`}>
                            <span className="text-[#202124] dark:text-[#F1F2F4] font-semibold">
                              ¥{Number(k.used_cost || 0).toFixed(4)}
                            </span>
                          </td>
                        )}
                        {isVisible('budget') && (
                          <td className={`ui-td font-mono tabular-nums ${COLUMN_CLASS.budget}`}>
                            <span className={k.budget > 0 ? 'text-[#355CFF] dark:text-[#91A7FF] font-medium' : 'text-[#656B75] dark:text-[#A1A7B0]'}>
                              {k.budget > 0 ? `¥${Number(k.budget).toFixed(2)}` : (isZh ? '不限' : 'Unlimited')}
                            </span>
                          </td>
                        )}
                        {isVisible('models') && (
                          <td className={`ui-td font-medium text-[#656B75] dark:text-[#A1A7B0] ${COLUMN_CLASS.models}`}>
                            {!k.allowed_models || k.allowed_models.length === 0 ? (
                              <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0] font-medium">
                                {isZh ? '全部允许' : 'All Allowed'}
                              </span>
                            ) : (
                              <span className="font-mono text-xs bg-[#F0F1F2] dark:bg-[#22252B] px-2 py-0.5 rounded border border-[#E4E6E9] dark:border-[#2C3038] text-[#202124] dark:text-[#F1F2F4]">
                                {k.allowed_models.join(', ')}
                              </span>
                            )}
                          </td>
                        )}
                        {isVisible('rpm') && (
                          <td className={`ui-td font-mono tabular-nums ${COLUMN_CLASS.rpm}`}>
                            {k.rpm ? `${k.rpm} RPM` : (isZh ? '不限' : 'Unlimited')}
                          </td>
                        )}
                        {isVisible('status') && (
                          <td className="ui-td">
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
                        )}
                        <td className="ui-td text-right whitespace-nowrap space-x-1.5">
                          <button
                            onClick={() => onNavigateToDashboard && onNavigateToDashboard(k.key, k.id)}
                            className="text-xs px-2.5 py-1 rounded-lg bg-[#EEF2FF] dark:bg-[#242D48] hover:bg-[#355CFF]/15 text-[#355CFF] dark:text-[#91A7FF] border border-[#355CFF]/30 dark:border-[#91A7FF]/30 font-medium transition-colors inline-flex items-center space-x-1 cursor-pointer"
                            title={isAdmin ? (isZh ? '查看该密钥的 RPM / TPM 指标' : 'View RPM/TPM metrics') : (isZh ? '查看该密钥的调用记录与扣费' : 'View this key’s usage and charges')}
                          >
                            <Activity className="w-3 h-3 text-[#355CFF] dark:text-[#91A7FF]" />
                            <span className="hidden lg:inline">{isAdmin ? (isZh ? '遥测' : 'Telemetry') : (isZh ? '用量' : 'Usage')}</span>
                          </button>
                          <button
                            onClick={() => copyCurlSnippet(k)}
                            className="text-xs px-2.5 py-1 rounded-lg bg-[#F0F1F2] dark:bg-[#22252B] hover:bg-[#E4E6E9] dark:hover:bg-[#2C3038] text-[#202124] dark:text-[#F1F2F4] font-medium transition-colors inline-flex items-center space-x-1 cursor-pointer"
                            title={isZh ? '复制 cURL 快速接入命令' : 'Copy cURL command'}
                          >
                            <Terminal className="w-3 h-3 text-[#656B75] dark:text-[#A1A7B0]" />
                            <span className="hidden sm:inline">cURL</span>
                          </button>
                          <button
                            onClick={() => handleToggleKeyStatus(k)}
                            className={`text-xs px-2.5 py-1 rounded-lg border font-medium transition-colors cursor-pointer ${
                              k.status === 'disabled'
                                ? 'bg-emerald-500/10 dark:bg-emerald-950/40 text-emerald-600 dark:text-emerald-400 hover:bg-emerald-500/20 border-emerald-500/30'
                                : 'bg-[#F0F1F2] dark:bg-[#22252B] text-[#202124] dark:text-[#F1F2F4] hover:bg-[#E4E6E9] dark:hover:bg-[#2C3038] border-[#E4E6E9] dark:border-[#2C3038]'
                            }`}
                            title={k.status === 'disabled' ? (isZh ? '恢复启用' : 'Enable') : (isZh ? '暂停使用' : 'Disable')}
                          >
                            {k.status === 'disabled' ? (isZh ? '启用' : 'Enable') : (isZh ? '停用' : 'Disable')}
                          </button>
                          <button
                            onClick={() => setActiveQuickKey(k)}
                            className="text-xs px-2.5 py-1 rounded-lg bg-[#EEF2FF] dark:bg-[#242D48] text-[#355CFF] dark:text-[#91A7FF] hover:bg-[#355CFF]/15 border border-[#355CFF]/30 dark:border-[#91A7FF]/30 font-medium transition-colors inline-flex items-center space-x-1 cursor-pointer"
                            title={isZh ? '打开快速接入指引' : 'Open quick connect guide'}
                          >
                            <Sparkles className="w-3 h-3 text-[#355CFF] dark:text-[#91A7FF]" />
                            <span className="hidden md:inline">{isZh ? '快速接入' : 'Quick Connect'}</span>
                          </button>
                          <button
                            onClick={() => handleDeleteKey(k.id)}
                            className="text-xs px-2 py-1 text-rose-600 dark:text-rose-400 hover:text-rose-700 dark:hover:text-rose-300 transition-colors font-medium cursor-pointer"
                          >
                            {isZh ? '删除' : 'Delete'}
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
