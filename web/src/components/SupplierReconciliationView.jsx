import { toCSV } from '../config';
import React, { useState, useEffect, useMemo, useCallback } from 'react';
import {
  DollarSign,
  Download,
  RefreshCw,
  Search,
  ChevronDown,
  ChevronUp,
  ExternalLink,
  Calendar,
  Server,
  Activity,
  Clock,
  Sparkles,
  Layers,
  Cpu,
  Filter,
  X,
  RotateCcw
} from 'lucide-react';

export default function SupplierReconciliationView({
  adminFetch,
  showToast,
  onNavigateToLogs,
  lang = 'zh',
  t
}) {
  const isZh = lang === 'zh';
  const [loading, setLoading] = useState(false);
  const [report, setReport] = useState(null);
  const [timePreset, setTimePreset] = useState('all'); // 'today' | 'yesterday' | '7d' | 'month' | 'all' | 'custom'
  const [customStart, setCustomStart] = useState('');
  const [customEnd, setCustomEnd] = useState('');
  const [channelFilter, setChannelFilter] = useState('all');
  const [modelFilter, setModelFilter] = useState('all');
  const [channelStatusFilter, setChannelStatusFilter] = useState('all'); // 'all' | 'active' | 'disabled' | 'deleted'
  const [trafficStatusFilter, setTrafficStatusFilter] = useState('all'); // 'all' | 'has_traffic' | 'no_traffic' | 'has_errors'
  const [searchQuery, setSearchQuery] = useState('');
  const [expandedSuppliers, setExpandedSuppliers] = useState({});

  const getTimeRangeDates = useCallback((preset) => {
    const now = new Date();
    const pad = (n) => String(n).padStart(2, '0');
    const format = (d) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;

    if (preset === 'today') {
      const start = new Date(now.getFullYear(), now.getMonth(), now.getDate(), 0, 0, 0);
      return { start: format(start), end: format(now) };
    }
    if (preset === 'yesterday') {
      const start = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1, 0, 0, 0);
      const end = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1, 23, 59, 59);
      return { start: format(start), end: format(end) };
    }
    if (preset === '7d') {
      const start = new Date(now.getTime() - 7 * 24 * 3600 * 1000);
      return { start: format(start), end: format(now) };
    }
    if (preset === 'month') {
      const start = new Date(now.getFullYear(), now.getMonth(), 1, 0, 0, 0);
      return { start: format(start), end: format(now) };
    }
    if (preset === 'custom') {
      return { start: customStart, end: customEnd };
    }
    return { start: '', end: '' };
  }, [customStart, customEnd]);

  const fetchReconciliation = useCallback(async () => {
    setLoading(true);
    try {
      const range = getTimeRangeDates(timePreset);
      const params = new URLSearchParams();
      if (range.start) params.set('start_time', range.start);
      if (range.end) params.set('end_time', range.end);
      if (channelFilter !== 'all') params.set('channel', channelFilter);
      if (modelFilter !== 'all') params.set('model', modelFilter);

      const res = await adminFetch(`/api/v1/admin/channels/reconciliation?${params.toString()}`);
      const data = await res.json();
      if (res.ok && data.code === 0) {
        setReport(data.data);
      } else {
        showToast(data.error || (isZh ? '获取对账数据失败' : 'Failed to fetch reconciliation'), 'warning');
      }
    } catch (err) {
      showToast((isZh ? '网络请求异常: ' : 'Network error: ') + err.message, 'warning');
    } finally {
      setLoading(false);
    }
  }, [adminFetch, showToast, isZh, getTimeRangeDates, timePreset, channelFilter, modelFilter]);

  useEffect(() => {
    fetchReconciliation();
  }, [fetchReconciliation, timePreset, channelFilter, modelFilter]);

  const toggleExpand = (channelName) => {
    setExpandedSuppliers(prev => ({
      ...prev,
      [channelName]: !prev[channelName]
    }));
  };

  const handleExportCSV = () => {
    if (!report || !report.suppliers || report.suppliers.length === 0) {
      showToast(isZh ? '暂无可导出的对账数据' : 'No data to export', 'warning');
      return;
    }

    const headers = [
      isZh ? '供应商名称' : 'Supplier Name',
      isZh ? '引擎类型' : 'Engine Type',
      isZh ? '接入状态' : 'Status',
      isZh ? '接口端点' : 'Base URL',
      isZh ? '调用总数' : 'Total Requests',
      isZh ? '成功调用数' : 'Success Requests',
      isZh ? '失败调用数' : 'Failed Requests',
      isZh ? '成功率(%)' : 'Success Rate (%)',
      isZh ? '未缓存输入Token' : 'Uncached Prompt Tokens',
      isZh ? '输出Token' : 'Completion Tokens',
      isZh ? '缓存Token' : 'Cached Tokens',
      isZh ? '生成速率(TPS)' : 'Tokens Per Second (TPS)',
      isZh ? '客户计费金额(元)' : 'Customer Charges (CNY)',
      isZh ? '平均时延(ms)' : 'Avg Duration (ms)',
      isZh ? '首字时延(ms)' : 'Avg TTFT (ms)'
    ];

    const rows = report.suppliers.map(s => {
      const uncachedPrompt = s.uncached_prompt_tokens !== undefined
        ? s.uncached_prompt_tokens
        : Math.max(0, (s.prompt_tokens || 0) - (s.cached_tokens || 0));
      return [
        s.channel_name || '',
        s.channel_type || '',
        s.channel_status || '',
        s.base_url || '',
        s.total_requests || 0,
        s.success_requests || 0,
        s.failed_requests || 0,
        s.success_rate || 0,
        uncachedPrompt,
        s.completion_tokens || 0,
        s.cached_tokens || 0,
        s.avg_tps || 0,
        (s.total_cost || 0).toFixed(4),
        s.avg_duration_ms || 0,
        s.avg_ttft_ms || 0
      ];
    });

    // RFC 4180 escaping + formula-injection guard via the shared helper
    const csvContent = toCSV(headers, rows);
    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `airoute_supplier_reconciliation_${new Date().toISOString().slice(0, 10)}.csv`;
    a.click();
    URL.revokeObjectURL(url);
    showToast(isZh ? '已导出供应商对账单 CSV' : 'Reconciliation CSV exported', 'success');
  };

  const summary = report?.summary || {};

  // Extract all distinct models across suppliers for the model filter dropdown
  const allModelsList = useMemo(() => {
    const set = new Set();
    (report?.suppliers || []).forEach(s => {
      (s.models || []).forEach(m => {
        if (m.model) set.add(m.model);
      });
    });
    return Array.from(set).sort();
  }, [report]);

  // Multi-condition combined filtering across channel, model, status, traffic, and search query
  const suppliers = useMemo(() => {
    return (report?.suppliers || []).filter(s => {
      // 1. Channel status filter
      if (channelStatusFilter !== 'all' && s.channel_status !== channelStatusFilter) {
        return false;
      }
      // 2. Traffic status filter
      const totalReqs = s.total_requests || 0;
      const failedReqs = s.failed_requests || 0;
      if (trafficStatusFilter === 'has_traffic' && totalReqs === 0) return false;
      if (trafficStatusFilter === 'no_traffic' && totalReqs > 0) return false;
      if (trafficStatusFilter === 'has_errors' && failedReqs === 0) return false;

      // 3. Search query
      if (searchQuery.trim()) {
        const q = searchQuery.toLowerCase();
        const matchName = (s.channel_name || '').toLowerCase().includes(q);
        const matchType = (s.channel_type || '').toLowerCase().includes(q);
        const matchUrl = (s.base_url || '').toLowerCase().includes(q);
        const matchModel = (s.models || []).some(m => (m.model || '').toLowerCase().includes(q));
        if (!matchName && !matchType && !matchUrl && !matchModel) return false;
      }
      return true;
    });
  }, [report, channelStatusFilter, trafficStatusFilter, searchQuery]);

  const hasActiveFilters = timePreset !== 'all' ||
    channelFilter !== 'all' ||
    modelFilter !== 'all' ||
    channelStatusFilter !== 'all' ||
    trafficStatusFilter !== 'all' ||
    searchQuery.trim() !== '';

  const handleResetFilters = () => {
    setTimePreset('all');
    setCustomStart('');
    setCustomEnd('');
    setChannelFilter('all');
    setModelFilter('all');
    setChannelStatusFilter('all');
    setTrafficStatusFilter('all');
    setSearchQuery('');
  };

  const summaryUncachedPrompt = summary.uncached_prompt_tokens !== undefined
    ? summary.uncached_prompt_tokens
    : Math.max(0, (summary.prompt_tokens || 0) - (summary.cached_tokens || 0));

  return (
    <div className="space-y-6">
      {/* 1. Header & Controls */}
      <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4">
        <div>
          <div className="flex items-center space-x-3">
            <h2 className="ui-page-title">
              {isZh ? '供应商整体对账' : 'Supplier FinOps Reconciliation'}
            </h2>
            <span className="text-xs font-mono px-2 py-0.5 rounded-md bg-[#EEF2FF] dark:bg-[#242D48] text-[#355CFF] dark:text-[#91A7FF] border border-[#355CFF]/20">
              {summary.active_suppliers_count || 0} {isZh ? '家活跃供应商' : 'active suppliers'}
            </span>
          </div>
          <p className="ui-page-subtitle">
            {isZh
              ? '按上游渠道核对调用量、Token 用量和客户计费金额。上游应付金额请以供应商账单为准。'
              : 'Usage and customer charges grouped by channel. Use supplier invoices for upstream payable amounts.'}
          </p>
        </div>

        <div className="flex items-center space-x-2.5">
          <button
            onClick={fetchReconciliation}
            disabled={loading}
            className="ui-btn-secondary ui-btn-sm"
          >
            <RefreshCw className={`w-3.5 h-3.5 text-[#656B75] dark:text-[#A1A7B0] ${loading ? 'animate-spin' : ''}`} />
            <span>{isZh ? '刷新对账' : 'Refresh'}</span>
          </button>
          <button
            onClick={handleExportCSV}
            disabled={!report || suppliers.length === 0}
            className="ui-btn-primary ui-btn-sm"
          >
            <Download className="w-3.5 h-3.5" />
            <span>{isZh ? '导出对账单' : 'Export Statement'}</span>
          </button>
        </div>
      </div>

      {/* 2. Top Metric Cards (6-Card Clean FinOps Grid) */}
      <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-3">
        {/* Cost */}
        <div className="ui-card p-3.5 shadow-xs">
          <div className="flex items-center justify-between text-[#656B75] dark:text-[#A1A7B0] mb-1">
            <span className="text-xs font-medium">{isZh ? '按渠道汇总客户计费' : 'Customer Charges by Channel'}</span>
            <DollarSign className="w-4 h-4 text-emerald-500" />
          </div>
          <div className="text-lg font-bold font-mono tracking-tight text-[#202124] dark:text-[#F1F2F4] truncate">
            ¥{(summary.total_cost || 0).toFixed(4)}
          </div>
          <div className="ui-meta mt-0.5 truncate">
            {isZh ? '按客户费率计算；不代表上游采购成本' : 'Customer rates; upstream purchase cost unavailable'}
          </div>
        </div>

        {/* Requests */}
        <div className="ui-card p-3.5 shadow-xs">
          <div className="flex items-center justify-between text-[#656B75] dark:text-[#A1A7B0] mb-1">
            <span className="text-xs font-medium">{isZh ? '已对账调用量' : 'Invocations'}</span>
            <Activity className="w-4 h-4 text-[#355CFF] dark:text-[#91A7FF]" />
          </div>
          <div className="text-lg font-bold font-mono tracking-tight text-[#202124] dark:text-[#F1F2F4] truncate">
            {(summary.total_requests || 0).toLocaleString()}
          </div>
          <div className="text-xs text-emerald-600 dark:text-emerald-400 mt-0.5 truncate font-medium">
            {isZh ? `成功率 ${summary.success_rate || 100}%` : `Success ${summary.success_rate || 100}%`}
          </div>
        </div>

        {/* Uncached Input */}
        <div className="ui-card p-3.5 shadow-xs">
          <div className="flex items-center justify-between text-[#656B75] dark:text-[#A1A7B0] mb-1">
            <span className="text-xs font-medium">{isZh ? '未缓存输入 Token' : 'Uncached Input'}</span>
            <Layers className="w-4 h-4 text-sky-500" />
          </div>
          <div className="text-lg font-bold font-mono tracking-tight text-[#202124] dark:text-[#F1F2F4] truncate">
            {summaryUncachedPrompt.toLocaleString()}
          </div>
          <div className="ui-meta mt-0.5 truncate">
            {isZh ? '纯输入结算基数' : 'Full price input'}
          </div>
        </div>

        {/* Completion */}
        <div className="ui-card p-3.5 shadow-xs">
          <div className="flex items-center justify-between text-[#656B75] dark:text-[#A1A7B0] mb-1">
            <span className="text-xs font-medium">{isZh ? '输出补全 Token' : 'Completion'}</span>
            <Cpu className="w-4 h-4 text-purple-500" />
          </div>
          <div className="text-lg font-bold font-mono tracking-tight text-[#202124] dark:text-[#F1F2F4] truncate">
            {(summary.completion_tokens || 0).toLocaleString()}
          </div>
          <div className="ui-meta mt-0.5 truncate">
            {isZh ? '模型输出解码量' : 'Generated tokens'}
          </div>
        </div>

        {/* Cached */}
        <div className="ui-card p-3.5 shadow-xs">
          <div className="flex items-center justify-between text-[#656B75] dark:text-[#A1A7B0] mb-1">
            <span className="text-xs font-medium">{isZh ? '前缀缓存 Token' : 'Cached Tokens'}</span>
            <Sparkles className="w-4 h-4 text-amber-500" />
          </div>
          <div className="text-lg font-bold font-mono tracking-tight text-amber-600 dark:text-amber-400 truncate">
            {(summary.cached_tokens || 0).toLocaleString()}
          </div>
          <div className="ui-meta mt-0.5 truncate">
            {isZh ? '享上游缓存折扣' : 'Prefix cache hits'}
          </div>
        </div>

        {/* TPS */}
        <div className="ui-card p-3.5 shadow-xs">
          <div className="flex items-center justify-between text-[#656B75] dark:text-[#A1A7B0] mb-1">
            <span className="text-xs font-medium">{isZh ? '生成速率 (TPS)' : 'Decoding Speed'}</span>
            <Clock className="w-4 h-4 text-[#355CFF] dark:text-[#91A7FF]" />
          </div>
          <div className="text-lg font-bold font-mono tracking-tight text-[#355CFF] dark:text-[#91A7FF] truncate">
            {summary.avg_tps ? `${summary.avg_tps} tok/s` : '-'}
          </div>
          <div className="ui-meta mt-0.5 truncate">
            TTFT: {summary.avg_ttft_ms || 0} ms
          </div>
        </div>
      </div>

      {/* 3. Multi-Condition Combined Filter Bar */}
      <div className="ui-card p-4 space-y-3">
        {/* Row 1: Time presets and Custom range picker */}
        <div className="flex flex-wrap items-center justify-between gap-3 text-xs">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-[#656B75] dark:text-[#A1A7B0] font-medium flex items-center space-x-1">
              <Calendar className="w-3.5 h-3.5 text-[#656B75] dark:text-[#A1A7B0]" />
              <span>{isZh ? '对账周期:' : 'Period:'}</span>
            </span>
            <div className="inline-flex bg-[#F0F1F2] dark:bg-[#22252B] p-0.5 rounded-lg border border-[#E4E6E9] dark:border-[#2C3038]">
              {[
                { id: 'all', label: isZh ? '全量历史' : 'All Time' },
                { id: 'today', label: isZh ? '今日' : 'Today' },
                { id: 'yesterday', label: isZh ? '昨日' : 'Yesterday' },
                { id: '7d', label: isZh ? '近7天' : 'Last 7 Days' },
                { id: 'month', label: isZh ? '本月' : 'This Month' },
                { id: 'custom', label: isZh ? '自定义' : 'Custom' }
              ].map(p => (
                <button
                  key={p.id}
                  onClick={() => setTimePreset(p.id)}
                  className={`px-3 py-1 rounded-md text-xs font-medium transition cursor-pointer ${
                    timePreset === p.id
                      ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-xs font-semibold'
                      : 'text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'
                  }`}
                >
                  {p.label}
                </button>
              ))}
            </div>

            {/* Custom date range picker if custom selected */}
            {timePreset === 'custom' && (
              <div className="flex items-center space-x-1.5 animate-in fade-in">
                <input
                  type="text"
                  value={customStart}
                  onChange={(e) => setCustomStart(e.target.value)}
                  placeholder="YYYY-MM-DD 00:00:00"
                  className="ui-field px-2.5 py-1 text-xs w-44 font-mono shadow-xs"
                />
                <span className="text-[#656B75] dark:text-[#A1A7B0]">至</span>
                <input
                  type="text"
                  value={customEnd}
                  onChange={(e) => setCustomEnd(e.target.value)}
                  placeholder="YYYY-MM-DD 23:59:59"
                  className="ui-field px-2.5 py-1 text-xs w-44 font-mono shadow-xs"
                />
                <button
                  onClick={fetchReconciliation}
                  className="ui-btn-primary ui-btn-sm"
                >
                  {isZh ? '查询' : 'Query'}
                </button>
              </div>
            )}
          </div>

          <div className="ui-meta font-mono">
            {isZh
              ? `匹配到 ${suppliers.length} / 共 ${(report?.suppliers || []).length} 家供应商`
              : `Showing ${suppliers.length} of ${(report?.suppliers || []).length} suppliers`}
          </div>
        </div>

        {/* Row 2: Combined Filter Selectors & Search Input */}
        <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-5 gap-2.5 text-xs pt-1 border-t border-[#E4E6E9] dark:border-[#2C3038]">
          {/* 1. Supplier / Channel Selector */}
          <div>
            <label className="ui-label-sm">
              {isZh ? '供应商渠道' : 'Supplier'}
            </label>
            <select
              value={channelFilter}
              onChange={(e) => setChannelFilter(e.target.value)}
              className="ui-field ui-field-sm cursor-pointer"
            >
              <option value="all">{isZh ? '全部服务商 (汇总)' : 'All Suppliers'}</option>
              {report?.suppliers?.map(s => (
                <option key={s.channel_name} value={s.channel_name}>
                  {s.channel_name} ({s.channel_type || 'openai'})
                </option>
              ))}
            </select>
          </div>

          {/* 2. Target Model Selector */}
          <div>
            <label className="ui-label-sm">
              {isZh ? '模型维度' : 'Model'}
            </label>
            <select
              value={modelFilter}
              onChange={(e) => setModelFilter(e.target.value)}
              className="ui-field ui-field-sm cursor-pointer font-mono"
            >
              <option value="all">{isZh ? '全部模型 (所有流量)' : 'All Models'}</option>
              {allModelsList.map(m => (
                <option key={m} value={m}>{m}</option>
              ))}
            </select>
          </div>

          {/* 3. Channel Operational Status */}
          <div>
            <label className="ui-label-sm">
              {isZh ? '接入状态' : 'Channel Status'}
            </label>
            <select
              value={channelStatusFilter}
              onChange={(e) => setChannelStatusFilter(e.target.value)}
              className="ui-field ui-field-sm cursor-pointer"
            >
              <option value="all">{isZh ? '全部接入状态' : 'All Statuses'}</option>
              <option value="active">{isZh ? '● 在用服务商 (Active)' : 'Active Only'}</option>
              <option value="disabled">{isZh ? '○ 停用服务商 (Disabled)' : 'Disabled Only'}</option>
              <option value="deleted">{isZh ? '▲ 已下架服务商 (Deleted)' : 'Deleted Only'}</option>
            </select>
          </div>

          {/* 4. Traffic & Error Distribution */}
          <div>
            <label className="ui-label-sm">
              {isZh ? '流量与健康' : 'Traffic Activity'}
            </label>
            <select
              value={trafficStatusFilter}
              onChange={(e) => setTrafficStatusFilter(e.target.value)}
              className="ui-field ui-field-sm cursor-pointer"
            >
              <option value="all">{isZh ? '全部调用分布' : 'All Traffic'}</option>
              <option value="has_traffic">{isZh ? '有调用量 (>0)' : 'Traffic > 0'}</option>
              <option value="no_traffic">{isZh ? '零调用量 (=0)' : 'Zero Traffic'}</option>
              <option value="has_errors">{isZh ? '存在失败调用 (Failures > 0)' : 'With Failures'}</option>
            </select>
          </div>

          {/* 5. Keyword Search */}
          <div>
            <label className="ui-label-sm">
              {isZh ? '关键字搜索' : 'Search Query'}
            </label>
            <div className="relative">
              <Search className="w-3.5 h-3.5 text-[#656B75] dark:text-[#A1A7B0] absolute left-2.5 top-2.5" />
              <input
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder={isZh ? '渠道 / 引擎 / 端点 / 模型...' : 'Channel, engine, url...'}
                className="ui-field ui-field-sm pl-8"
              />
            </div>
          </div>
        </div>

        {/* Row 3: Active Filter Chips Bar (Shown when any filter is active) */}
        {hasActiveFilters && (
          <div className="flex flex-wrap items-center justify-between gap-2 pt-2 border-t border-[#E4E6E9] dark:border-[#2C3038] animate-in fade-in">
            <div className="flex flex-wrap items-center gap-1.5">
              <span className="ui-meta font-medium flex items-center space-x-1 mr-1">
                <Filter className="w-3 h-3 text-[#355CFF] dark:text-[#91A7FF]" />
                <span>{isZh ? '已生效筛选条件:' : 'Active Filters:'}</span>
              </span>

              {timePreset !== 'all' && (
                <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-md bg-[#EEF2FF] dark:bg-[#242D48] text-[#355CFF] dark:text-[#91A7FF] border border-[#355CFF]/20 text-xs">
                  <span>{isZh ? '周期:' : 'Period:'} {timePreset}</span>
                  <button onClick={() => setTimePreset('all')} className="hover:text-rose-500 cursor-pointer">
                    <X className="w-2.5 h-2.5" />
                  </button>
                </span>
              )}

              {channelFilter !== 'all' && (
                <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-md bg-[#EEF2FF] dark:bg-[#242D48] text-[#355CFF] dark:text-[#91A7FF] border border-[#355CFF]/20 text-xs">
                  <span>{isZh ? '渠道:' : 'Channel:'} {channelFilter}</span>
                  <button onClick={() => setChannelFilter('all')} className="hover:text-rose-500 cursor-pointer">
                    <X className="w-2.5 h-2.5" />
                  </button>
                </span>
              )}

              {modelFilter !== 'all' && (
                <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-md bg-purple-50 dark:bg-purple-950/40 text-purple-700 dark:text-purple-300 border border-purple-200/70 dark:border-purple-800 text-xs font-mono">
                  <span>{isZh ? '模型:' : 'Model:'} {modelFilter}</span>
                  <button onClick={() => setModelFilter('all')} className="hover:text-rose-500 cursor-pointer">
                    <X className="w-2.5 h-2.5" />
                  </button>
                </span>
              )}

              {channelStatusFilter !== 'all' && (
                <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-md bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0] border border-[#E4E6E9] dark:border-[#2C3038] text-xs">
                  <span>{isZh ? '状态:' : 'Status:'} {channelStatusFilter}</span>
                  <button onClick={() => setChannelStatusFilter('all')} className="hover:text-rose-500 cursor-pointer">
                    <X className="w-2.5 h-2.5" />
                  </button>
                </span>
              )}

              {trafficStatusFilter !== 'all' && (
                <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-md bg-amber-50 dark:bg-amber-950/40 text-amber-700 dark:text-amber-300 border border-amber-200/70 dark:border-amber-800 text-xs">
                  <span>
                    {trafficStatusFilter === 'has_traffic' ? (isZh ? '有调用量' : 'Has Traffic') :
                     trafficStatusFilter === 'no_traffic' ? (isZh ? '零调用量' : 'No Traffic') :
                     (isZh ? '存在失败调用' : 'Has Errors')}
                  </span>
                  <button onClick={() => setTrafficStatusFilter('all')} className="hover:text-rose-500 cursor-pointer">
                    <X className="w-2.5 h-2.5" />
                  </button>
                </span>
              )}

              {searchQuery.trim() !== '' && (
                <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-md bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0] border border-[#E4E6E9] dark:border-[#2C3038] text-xs">
                  <span>"{searchQuery}"</span>
                  <button onClick={() => setSearchQuery('')} className="hover:text-rose-500 cursor-pointer">
                    <X className="w-2.5 h-2.5" />
                  </button>
                </span>
              )}
            </div>

            <button
              onClick={handleResetFilters}
              className="text-xs text-[#656B75] dark:text-[#A1A7B0] hover:text-rose-600 dark:hover:text-rose-400 font-medium inline-flex items-center space-x-1 cursor-pointer transition"
            >
              <RotateCcw className="w-3 h-3" />
              <span>{isZh ? '重置所有筛选' : 'Reset All'}</span>
            </button>
          </div>
        )}
      </div>

      {/* 4. Suppliers Master Reconciliation Table */}
      <div className="ui-card overflow-hidden">
        {suppliers.length === 0 ? (
          <div className="p-12 text-center flex flex-col items-center justify-center space-y-2">
            <Server className="w-10 h-10 text-[#656B75]/40 dark:text-[#A1A7B0]/40" />
            <h4 className="font-medium text-xs text-[#202124] dark:text-[#F1F2F4]">
              {isZh ? '所选时间范围内暂无对账流水记录' : 'No reconciliation records for selected period'}
            </h4>
            <p className="ui-meta">
              {isZh ? '发起模型调用后，各服务商将自动沉淀对账流水' : 'Invocations through gateway will automatically log here'}
            </p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left border-collapse text-xs min-w-[900px]">
              <thead className="sticky top-0 z-20 shadow-2xs">
                <tr className="text-[#656B75] dark:text-[#A1A7B0] uppercase text-[11px] font-medium tracking-wide">
                  <th className="ui-th min-w-[160px] sticky top-0 z-20 bg-[#F0F1F2]/90 dark:bg-[#191B1F]/90 border-b border-[#E4E6E9] dark:border-[#2C3038]">{isZh ? '供应商' : 'Supplier'}</th>
                  <th className="ui-th text-right w-20 min-w-[75px] sticky top-0 z-20 bg-[#F0F1F2]/90 dark:bg-[#191B1F]/90 border-b border-[#E4E6E9] dark:border-[#2C3038]">{isZh ? '调用量' : 'Calls'}</th>
                  <th className="ui-th text-right w-20 min-w-[70px] sticky top-0 z-20 bg-[#F0F1F2]/90 dark:bg-[#191B1F]/90 border-b border-[#E4E6E9] dark:border-[#2C3038]" title={isZh ? '未缓存输入 Prompt Token' : 'Uncached prompt tokens'}>{isZh ? '输入' : 'Input'}</th>
                  <th className="ui-th text-right w-20 min-w-[70px] sticky top-0 z-20 bg-[#F0F1F2]/90 dark:bg-[#191B1F]/90 border-b border-[#E4E6E9] dark:border-[#2C3038]" title={isZh ? '模型输出 Completion Token' : 'Completion tokens'}>{isZh ? '输出' : 'Output'}</th>
                  <th className="ui-th text-right w-18 min-w-[65px] sticky top-0 z-20 bg-[#F0F1F2]/90 dark:bg-[#191B1F]/90 border-b border-[#E4E6E9] dark:border-[#2C3038]" title={isZh ? '前缀缓存命中 Token' : 'Cached prefix tokens'}>{isZh ? '缓存' : 'Cached'}</th>
                  <th className="ui-th text-right w-20 min-w-[70px] sticky top-0 z-20 bg-[#F0F1F2]/90 dark:bg-[#191B1F]/90 border-b border-[#E4E6E9] dark:border-[#2C3038]" title={isZh ? '生成速率 (Tokens/秒)' : 'Tokens per second'}>{isZh ? 'TPS' : 'TPS'}</th>
                  <th className="ui-th text-right w-20 min-w-[75px] sticky top-0 z-20 bg-[#F0F1F2]/90 dark:bg-[#191B1F]/90 border-b border-[#E4E6E9] dark:border-[#2C3038]">{isZh ? '客户计费' : 'Charges'}</th>
                  <th className="ui-th text-right w-24 min-w-[85px] sticky top-0 z-20 bg-[#F0F1F2]/90 dark:bg-[#191B1F]/90 border-b border-[#E4E6E9] dark:border-[#2C3038]">{isZh ? '耗时 / TTFT' : 'Latency / TTFT'}</th>
                  <th className="ui-th text-center w-24 min-w-[85px] sticky top-0 z-20 bg-[#F0F1F2]/90 dark:bg-[#191B1F]/90 border-b border-[#E4E6E9] dark:border-[#2C3038]">{isZh ? '操作' : 'Actions'}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[#E4E6E9]/60 dark:divide-[#2C3038]/60">
                {suppliers.map((sup) => {
                  const isExpanded = !!expandedSuppliers[sup.channel_name];
                  const hasUsage = (sup.total_requests || 0) > 0;
                  const uncachedPrompt = sup.uncached_prompt_tokens !== undefined
                    ? sup.uncached_prompt_tokens
                    : Math.max(0, (sup.prompt_tokens || 0) - (sup.cached_tokens || 0));

                  return (
                    <React.Fragment key={sup.channel_name || sup.channel_id}>
                      <tr className="ui-tr-hover">
                        {/* Supplier Name & Engine */}
                        <td className="ui-td">
                          <div className="flex items-center space-x-2.5">
                            <div className="w-8 h-8 rounded-lg bg-[#F0F1F2] dark:bg-[#22252B] border border-[#E4E6E9] dark:border-[#2C3038] flex items-center justify-center shrink-0 shadow-2xs">
                              <Server className="w-4 h-4 text-[#355CFF] dark:text-[#91A7FF]" />
                            </div>
                            <div className="min-w-0">
                              <div className="flex items-center space-x-2 font-medium text-[#202124] dark:text-[#F1F2F4]">
                                <span className="font-semibold text-sm">{sup.channel_name || (isZh ? '未命名渠道' : 'Unnamed Channel')}</span>
                                <span className="text-xs px-1.5 py-0.5 rounded bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0] font-mono font-medium border border-[#E4E6E9]/60 dark:border-[#2C3038]/60">
                                  {sup.channel_type || 'openai'}
                                </span>
                                {sup.channel_status === 'active' && (
                                  <span className="text-xs text-emerald-600 dark:text-emerald-400 font-medium flex items-center space-x-1 whitespace-nowrap">
                                    <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 inline-block"></span>
                                    <span>{isZh ? '在用' : 'Active'}</span>
                                  </span>
                                )}
                                {sup.channel_status === 'disabled' && (
                                  <span className="text-xs text-[#656B75] dark:text-[#A1A7B0] font-medium flex items-center space-x-1 whitespace-nowrap">
                                    <span className="w-1.5 h-1.5 rounded-full bg-[#656B75] inline-block"></span>
                                    <span>{isZh ? '停用' : 'Disabled'}</span>
                                  </span>
                                )}
                                {sup.channel_status === 'deleted' && (
                                  <span className="text-xs text-amber-500 dark:text-amber-400 font-medium flex items-center space-x-1 whitespace-nowrap">
                                    <span className="w-1.5 h-1.5 rounded-full bg-amber-500 inline-block"></span>
                                    <span>{isZh ? '已下架' : 'Deleted'}</span>
                                  </span>
                                )}
                              </div>
                              <div className="ui-meta font-mono truncate max-w-xs mt-0.5" title={sup.base_url}>
                                {sup.base_url || (isZh ? '默认网关转发' : 'Default gateway')}
                              </div>
                            </div>
                          </div>
                        </td>

                        {/* Calls / Success */}
                        <td className="ui-td text-right font-mono tabular-nums whitespace-nowrap">
                          <div className="font-semibold text-[#202124] dark:text-[#F1F2F4]">
                            {(sup.total_requests || 0).toLocaleString()}
                          </div>
                          {hasUsage ? (
                            <div className="flex items-center justify-end space-x-1 mt-0.5">
                              <span className={`inline-block text-xs px-1.5 py-0.2 rounded font-medium whitespace-nowrap ${
                                sup.success_rate >= 99
                                  ? 'bg-emerald-50 dark:bg-emerald-950/40 text-emerald-700 dark:text-emerald-300'
                                  : sup.success_rate >= 95
                                  ? 'bg-amber-50 dark:bg-amber-950/40 text-amber-700 dark:text-amber-300'
                                  : 'bg-rose-50 dark:bg-rose-950/40 text-rose-700 dark:text-rose-300'
                              }`}>
                                {sup.success_rate}%
                              </span>
                              {sup.failed_requests > 0 && (
                                <span className="text-xs text-rose-500 dark:text-rose-400">({sup.failed_requests} 败)</span>
                              )}
                            </div>
                          ) : (
                            <span className="inline-block text-xs px-1.5 py-0.2 rounded font-medium mt-0.5 bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0]">
                              {isZh ? '期内无调用' : 'No Traffic'}
                            </span>
                          )}
                        </td>

                        {/* Uncached Prompt Tokens */}
                        <td className="ui-td text-right font-mono tabular-nums text-[#202124] dark:text-[#F1F2F4] font-medium whitespace-nowrap">
                          {uncachedPrompt.toLocaleString()}
                        </td>

                        {/* Completion Tokens */}
                        <td className="ui-td text-right font-mono tabular-nums text-[#202124] dark:text-[#F1F2F4] font-medium whitespace-nowrap">
                          {(sup.completion_tokens || 0).toLocaleString()}
                        </td>

                        {/* Cached Tokens */}
                        <td className="ui-td text-right font-mono tabular-nums whitespace-nowrap">
                          {(sup.cached_tokens || 0) > 0 ? (
                            <span className="text-amber-600 dark:text-amber-400 font-medium">
                              {(sup.cached_tokens || 0).toLocaleString()}
                            </span>
                          ) : (
                            <span className="text-[#656B75] dark:text-[#A1A7B0]">-</span>
                          )}
                        </td>

                        {/* Speed (TPS) */}
                        <td className="ui-td text-right font-mono tabular-nums whitespace-nowrap">
                          {hasUsage && sup.avg_tps > 0 ? (
                            <div className="flex items-center justify-end space-x-1 font-semibold">
                              <span className="text-[#355CFF] dark:text-[#91A7FF]">{sup.avg_tps}</span>
                              <span className="text-xs text-[#656B75] dark:text-[#A1A7B0] font-normal">tok/s</span>
                            </div>
                          ) : (
                            <span className="text-[#656B75] dark:text-[#A1A7B0]">-</span>
                          )}
                        </td>

                        {/* Total Cost */}
                        <td className="ui-td text-right font-mono tabular-nums whitespace-nowrap">
                          <div className="font-bold text-sm text-[#202124] dark:text-[#F1F2F4]">
                            ¥{(sup.total_cost || 0).toFixed(4)}
                          </div>
                        </td>

                        {/* Latency / TTFT */}
                        <td className="ui-td text-right font-mono tabular-nums text-xs text-[#656B75] dark:text-[#A1A7B0] whitespace-nowrap">
                          <div className="font-medium text-[#202124] dark:text-[#F1F2F4]">{sup.avg_duration_ms || 0} ms</div>
                          <div className="text-xs text-[#656B75] dark:text-[#A1A7B0]">TTFT: {sup.avg_ttft_ms || 0} ms</div>
                        </td>

                        {/* Actions */}
                        <td className="ui-td text-center space-x-1.5 whitespace-nowrap">
                          {sup.models && sup.models.length > 0 && (
                            <button
                              onClick={() => toggleExpand(sup.channel_name)}
                              className="ui-btn-secondary ui-btn-sm"
                              title={isZh ? '查看该供应商各模型消耗细项' : 'View model breakdown'}
                            >
                              <span>{sup.models.length} {isZh ? '个模型' : 'models'}</span>
                              {isExpanded ? <ChevronUp className="w-3 h-3 text-[#656B75] dark:text-[#A1A7B0]" /> : <ChevronDown className="w-3 h-3 text-[#656B75] dark:text-[#A1A7B0]" />}
                            </button>
                          )}
                          {onNavigateToLogs && (
                            <button
                              onClick={() => onNavigateToLogs(sup.channel_name)}
                              className="ui-btn-secondary ui-btn-sm"
                              title={isZh ? '在审计日志中查看该供应商全部流水与厂商单号 (chat_id)' : 'Audit logs & chat_id'}
                            >
                              <span>{isZh ? '核对流水' : 'Logs'}</span>
                              <ExternalLink className="w-2.5 h-2.5" />
                            </button>
                          )}
                        </td>
                      </tr>

                      {/* Expandable Model Breakdown Details */}
                      {isExpanded && sup.models && sup.models.length > 0 && (
                        <tr className="bg-[#F0F1F2]/40 dark:bg-[#22252B]/40 border-b border-[#E4E6E9] dark:border-[#2C3038]">
                          <td colSpan="9" className="py-3.5 px-6">
                            <div className="text-xs font-semibold text-[#202124] dark:text-[#F1F2F4] mb-2 flex items-center space-x-2">
                              <Layers className="w-3.5 h-3.5 text-[#355CFF] dark:text-[#91A7FF]" />
                              <span>{isZh ? `[${sup.channel_name}] 模型级对账结算明细` : `[${sup.channel_name}] Model Breakdown`}</span>
                            </div>

                            <div className="border border-[#E4E6E9] dark:border-[#2C3038] rounded-xl overflow-hidden bg-white dark:bg-[#191B1F] text-xs shadow-2xs">
                              <table className="w-full text-left min-w-[850px]">
                                <thead className="bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0] text-[11px] border-b border-[#E4E6E9] dark:border-[#2C3038] uppercase font-semibold">
                                  <tr>
                                    <th className="ui-th min-w-[140px]">{isZh ? '模型' : 'Model'}</th>
                                    <th className="ui-th text-right w-20 min-w-[75px]">{isZh ? '调用量' : 'Calls'}</th>
                                    <th className="ui-th text-right w-20 min-w-[70px]" title={isZh ? '未缓存输入 Prompt Token' : 'Uncached prompt tokens'}>{isZh ? '输入' : 'Input'}</th>
                                    <th className="ui-th text-right w-20 min-w-[70px]" title={isZh ? '模型输出 Completion Token' : 'Completion tokens'}>{isZh ? '输出' : 'Output'}</th>
                                    <th className="ui-th text-right w-18 min-w-[65px]" title={isZh ? '前缀缓存命中 Token' : 'Cached prefix tokens'}>{isZh ? '缓存' : 'Cached'}</th>
                                    <th className="ui-th text-right w-20 min-w-[70px]" title={isZh ? '生成速率 (Tokens/秒)' : 'Tokens per second'}>{isZh ? 'TPS' : 'TPS'}</th>
                                    <th className="ui-th text-right w-20 min-w-[75px]">{isZh ? '客户计费' : 'Charges'}</th>
                                    <th className="ui-th text-right w-24 min-w-[85px]">{isZh ? '耗时 / TTFT' : 'Latency / TTFT'}</th>
                                  </tr>
                                </thead>
                                <tbody className="divide-y divide-[#E4E6E9]/60 dark:divide-[#2C3038]/60 font-mono text-xs">
                                  {sup.models.map((m) => {
                                    const mUncached = m.uncached_prompt_tokens !== undefined
                                      ? m.uncached_prompt_tokens
                                      : Math.max(0, (m.prompt_tokens || 0) - (m.cached_tokens || 0));
                                    return (
                                      <tr key={m.model} className="ui-tr-hover">
                                        <td className="ui-td font-sans font-medium text-[#202124] dark:text-[#F1F2F4]">
                                          {m.model}
                                        </td>
                                        <td className="ui-td text-right tabular-nums whitespace-nowrap">
                                          <span className="font-semibold text-[#202124] dark:text-[#F1F2F4]">{m.total_requests}</span>
                                          <span className="text-xs text-[#656B75] dark:text-[#A1A7B0] ml-1.5">({m.success_requests}/{m.failed_requests})</span>
                                        </td>
                                        <td className="ui-td text-right text-[#202124] dark:text-[#F1F2F4] tabular-nums whitespace-nowrap">
                                          {mUncached.toLocaleString()}
                                        </td>
                                        <td className="ui-td text-right text-[#202124] dark:text-[#F1F2F4] tabular-nums whitespace-nowrap">
                                          {m.completion_tokens.toLocaleString()}
                                        </td>
                                        <td className="ui-td text-right tabular-nums whitespace-nowrap">
                                          {m.cached_tokens > 0 ? (
                                            <span className="text-amber-600 dark:text-amber-400 font-medium">
                                              {m.cached_tokens.toLocaleString()}
                                            </span>
                                          ) : (
                                            <span className="text-[#656B75] dark:text-[#A1A7B0]">-</span>
                                          )}
                                        </td>
                                        <td className="ui-td text-right tabular-nums whitespace-nowrap">
                                          {m.avg_tps > 0 ? (
                                            <span className="text-[#355CFF] dark:text-[#91A7FF] font-medium">
                                              {m.avg_tps} <span className="text-xs text-[#656B75] dark:text-[#A1A7B0] font-normal">tok/s</span>
                                            </span>
                                          ) : (
                                            <span className="text-[#656B75] dark:text-[#A1A7B0]">-</span>
                                          )}
                                        </td>
                                        <td className="ui-td text-right font-bold text-[#202124] dark:text-[#F1F2F4] tabular-nums whitespace-nowrap">
                                          ¥{m.total_cost.toFixed(4)}
                                        </td>
                                        <td className="ui-td text-right text-[#656B75] dark:text-[#A1A7B0] text-xs tabular-nums whitespace-nowrap">
                                          {m.avg_duration_ms}ms / TTFT {m.avg_ttft_ms}ms
                                        </td>
                                      </tr>
                                    );
                                  })}
                                </tbody>
                              </table>
                            </div>
                          </td>
                        </tr>
                      )}
                    </React.Fragment>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
