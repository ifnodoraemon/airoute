import React, { useState, useEffect } from 'react';
import {
  DollarSign,
  Sparkles,
  Plus,
  RefreshCw,
  Edit3,
  Trash2,
  AlertCircle,
  Clock,
  Moon,
  Sun,
  X,
  Check,
  CheckCircle2,
  Calendar,
  Zap,
  Info
} from 'lucide-react';

function formatDaysLabel(days) {
  if (!days || days.length === 0 || days.length === 7) return '';
  const dayNames = { 1: '周一', 2: '周二', 3: '周三', 4: '周四', 5: '周五', 6: '周六', 7: '周日' };
  const sorted = [...days].sort((a, b) => a - b);
  if (sorted.length === 5 && sorted.every((d, i) => d === i + 1)) return '工作日';
  if (sorted.length === 2 && sorted[0] === 6 && sorted[1] === 7) return '周末';
  return sorted.map(d => dayNames[d] || d).join('、');
}

function parseMinutes(t) {
  if (!t) return null;
  const parts = t.trim().split(':');
  if (parts.length !== 2) return null;
  const h = parseInt(parts[0], 10);
  const m = parseInt(parts[1], 10);
  if (isNaN(h) || isNaN(m) || h < 0 || h > 24 || m < 0 || m > 59) return null;
  return h * 60 + m;
}

export function checkSlotsOverlap(slots) {
  if (!slots || slots.length < 2) return null;
  const dayNames = { 1: '周一', 2: '周二', 3: '周三', 4: '周四', 5: '周五', 6: '周六', 7: '周日' };

  for (let i = 0; i < slots.length; i++) {
    const s1 = slots[i];
    const s1Start = parseMinutes(s1.start);
    const s1End = parseMinutes(s1.end);
    if (s1Start === null || s1End === null) {
      return `时段「${s1.name || i + 1}」时间格式无效 (格式需为 HH:MM)`;
    }
    if (s1Start === s1End) {
      return `时段「${s1.name || i + 1}」开始时间与结束时间不能相同`;
    }

    const segs1 = s1Start < s1End 
      ? [{ s: s1Start, e: s1End }] 
      : [{ s: s1Start, e: 1440 }, { s: 0, e: s1End }];

    const days1 = (s1.days && s1.days.length > 0) ? s1.days : [1, 2, 3, 4, 5, 6, 7];

    for (let j = i + 1; j < slots.length; j++) {
      const s2 = slots[j];
      const s2Start = parseMinutes(s2.start);
      const s2End = parseMinutes(s2.end);
      if (s2Start === null || s2End === null) continue;
      if (s2Start === s2End) {
        return `时段「${s2.name || j + 1}」开始时间与结束时间不能相同`;
      }

      const days2 = (s2.days && s2.days.length > 0) ? s2.days : [1, 2, 3, 4, 5, 6, 7];
      const commonDays = days1.filter(d => days2.includes(d));
      if (commonDays.length === 0) {
        continue;
      }

      const segs2 = s2Start < s2End 
        ? [{ s: s2Start, e: s2End }] 
        : [{ s: s2Start, e: 1440 }, { s: 0, e: s2End }];

      for (const seg1 of segs1) {
        for (const seg2 of segs2) {
          if (Math.max(seg1.s, seg2.s) < Math.min(seg1.e, seg2.e)) {
            const overlapDaysStr = commonDays.length === 7 ? '每天' : commonDays.map(d => dayNames[d] || d).join('、');
            return `时段「${s1.name || i + 1}」(${s1.start}-${s1.end}) 与 时段「${s2.name || j + 1}」(${s2.start}-${s2.end}) 在【${overlapDaysStr}】存在时间重叠，请调整时段避免冲突`;
          }
        }
      }
    }
  }
  return null;
}

function parseSlotsFromPrice(p) {
  if (p && p.off_peak_slots) {
    try {
      const parsed = JSON.parse(p.off_peak_slots);
      if (Array.isArray(parsed) && parsed.length > 0) {
        return parsed.map((s, idx) => ({
          id: `slot-${idx + 1}-${Date.now()}`,
          name: s.name || `时段 ${idx + 1}`,
          start: s.start || '00:00',
          end: s.end || '08:30',
          discount: s.discount !== undefined ? parseFloat(s.discount) : 0.5,
          days: Array.isArray(s.days) ? s.days : []
        }));
      }
    } catch (e) {}
  }
  if (p && (p.off_peak_start || p.off_peak_end)) {
    return [{
      id: `slot-1-${Date.now()}`,
      name: '时段 1',
      start: p.off_peak_start || '00:00',
      end: p.off_peak_end || '08:30',
      discount: p.off_peak_discount ?? 0.5,
      days: []
    }];
  }
  return [
    { id: `slot-1-${Date.now()}`, name: '时段 1', start: '00:00', end: '08:30', discount: 0.5, days: [] }
  ];
}

export default function PricingManager({ adminFetch, showToast, stats = {} }) {
  const [prices, setPrices] = useState([]);
  const [loading, setLoading] = useState(false);
  const [showModal, setShowModal] = useState(false);
  const [editingPrice, setEditingPrice] = useState(null);
  const [serverTime, setServerTime] = useState('');
  const [serverWeekday, setServerWeekday] = useState('');
  const [isWeekend, setIsWeekend] = useState(false);
  const [platformModels, setPlatformModels] = useState([]);
  const [selectedKeys, setSelectedKeys] = useState([]);
  const [groupFilter, setGroupFilter] = useState('all');

  // Clean, flexible form state (supports multiple arbitrary time windows & user group tiers)
  const [formData, setFormData] = useState({
    model: '',
    group_name: 'default',
    prompt_price: 2.0,
    completion_price: 8.0,
    cache_read_price: 0.2,
    fixed_price: 0.0,
    currency: 'CNY',
    off_peak_enabled: true,
    weekend_all_day: true,
    slots: [
      { id: 'slot-1', name: '时段 1', start: '00:00', end: '08:30', discount: 0.5, days: [] }
    ]
  });
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState('');

  const fetchPrices = async () => {
    setLoading(true);
    try {
      const [res, modelsRes] = await Promise.all([
        adminFetch('/api/v1/admin/pricing'),
        adminFetch('/api/v1/admin/models').catch(() => null)
      ]);
      const data = await res.json();
      if (res.ok && data.code === 0) {
        setPrices(data.data || []);
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
      showToast('网络请求异常: ' + err.message, 'error');
    } finally {
      setLoading(false);
    }
  };

  const availableGroups = React.useMemo(() => {
    const set = new Set(['default', 'vip', 'enterprise']);
    prices.forEach(p => {
      if (p.group_name) set.add(p.group_name);
    });
    return Array.from(set);
  }, [prices]);

  const filteredPrices = React.useMemo(() => {
    if (groupFilter === 'all') return prices;
    return prices.filter(p => (p.group_name || 'default') === groupFilter);
  }, [prices, groupFilter]);

  const unpricedModels = React.useMemo(() => {
    const curGroup = formData.group_name || 'default';
    const configured = new Set(prices.filter(p => (p.group_name || 'default') === curGroup).map(p => p.model));
    return (platformModels || []).filter(m => !configured.has(m));
  }, [platformModels, prices, formData.group_name]);

  const getGroupBadge = (group) => {
    const g = (group || 'default').toLowerCase();
    if (g === 'vip') {
      return {
        label: 'VIP 用户组',
        className: 'bg-amber-50 text-amber-700 border-amber-200'
      };
    }
    if (g === 'enterprise') {
      return {
        label: '企业大客户',
        className: 'bg-purple-50 text-purple-700 border-purple-200'
      };
    }
    if (g === 'default') {
      return {
        label: '默认基础组',
        className: 'bg-indigo-50 text-indigo-700 border-indigo-200'
      };
    }
    return {
      label: `${group} 组`,
      className: 'bg-sky-50 text-sky-700 border-sky-200'
    };
  };

  const handleSelectModelCandidate = (m) => {
    const lower = m.toLowerCase();
    let promptP = 2.0;
    let compP = 8.0;
    let cacheP = 0.2;
    if (lower.includes('image') || lower.includes('seedream') || lower.includes('dall-e') || lower.includes('flux')) {
      promptP = 0.10;
      compP = 0.10;
      cacheP = 0.01;
    } else if (lower.includes('video') || lower.includes('seedance') || lower.includes('wan') || lower.includes('kling') || lower.includes('cogvideo') || lower.includes('sora')) {
      promptP = 0.50;
      compP = 0.50;
      cacheP = 0.05;
    } else if (lower.includes('embed') || lower.includes('bge-') || lower.includes('rerank')) {
      promptP = 0.05;
      compP = 0.05;
      cacheP = 0.005;
    }
    setFormData(prev => ({
      ...prev,
      model: m,
      prompt_price: promptP,
      completion_price: compP,
      cache_read_price: cacheP
    }));
  };

  useEffect(() => {
    fetchPrices();
  }, []);

  const handleOpenAdd = () => {
    setEditingPrice(null);
    setFormData({
      model: '',
      group_name: groupFilter !== 'all' ? groupFilter : 'default',
      prompt_price: 2.0,
      completion_price: 8.0,
      cache_read_price: 0.2,
      fixed_price: 0.0,
      currency: 'CNY',
      off_peak_enabled: true,
      weekend_all_day: true,
      slots: [
        { id: `slot-${Date.now()}`, name: '时段 1', start: '00:00', end: '08:30', discount: 0.5, days: [] }
      ]
    });
    setFormError('');
    setShowModal(true);
  };

  const handleOpenEdit = (p) => {
    setEditingPrice(p);
    setFormData({
      model: p.model,
      group_name: p.group_name || 'default',
      prompt_price: p.prompt_price ?? 2.0,
      completion_price: p.completion_price ?? 8.0,
      cache_read_price: p.cache_read_price ?? 0.2,
      fixed_price: p.fixed_price ?? 0.0,
      currency: p.currency || 'CNY',
      off_peak_enabled: p.off_peak_enabled !== false,
      weekend_all_day: p.weekend_all_day !== false,
      slots: parseSlotsFromPrice(p)
    });
    setFormError('');
    setShowModal(true);
  };

  const handleAddSlot = () => {
    const nextId = `slot-${Date.now()}-${formData.slots.length + 1}`;
    setFormData(prev => ({
      ...prev,
      slots: [
        ...prev.slots,
        {
          id: nextId,
          name: `时段 ${prev.slots.length + 1}`,
          start: '12:00',
          end: '14:00',
          discount: 0.8,
          days: []
        }
      ]
    }));
  };

  const handleRemoveSlot = (slotId) => {
    setFormData(prev => ({
      ...prev,
      slots: prev.slots.filter(s => s.id !== slotId)
    }));
  };

  const handleUpdateSlot = (slotId, field, value) => {
    setFormData(prev => ({
      ...prev,
      slots: prev.slots.map(s => s.id === slotId ? { ...s, [field]: value } : s)
    }));
  };

  const isDayActive = (slotDays, day) => {
    if (!slotDays || slotDays.length === 0) return true;
    return slotDays.includes(day);
  };

  const toggleSlotDay = (slotId, day) => {
    setFormData(prev => ({
      ...prev,
      slots: prev.slots.map(s => {
        if (s.id !== slotId) return s;
        const current = (!s.days || s.days.length === 0) ? [1, 2, 3, 4, 5, 6, 7] : [...s.days];
        let updated;
        if (current.includes(day)) {
          if (current.length === 1) return s; // Keep at least one day
          updated = current.filter(d => d !== day);
        } else {
          updated = [...current, day].sort((a, b) => a - b);
        }
        if (updated.length === 7) updated = [];
        return { ...s, days: updated };
      })
    }));
  };

  const handleDelete = async (model, groupName = 'default') => {
    if (!window.confirm(`确定删除模型 [${model}] 在 [${groupName}] 分组的定价规则？`)) {
      return;
    }
    try {
      const res = await adminFetch(`/api/v1/admin/pricing/${encodeURIComponent(model)}?group=${encodeURIComponent(groupName || 'default')}`, {
        method: 'DELETE'
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(`已删除 [${groupName}] 组模型 [${model}] 定价规则`, 'success');
        fetchPrices();
      } else {
        showToast(data.error || '删除失败', 'warning');
      }
    } catch (err) {
      showToast('请求异常: ' + err.message, 'error');
    }
  };

  const handleBatchDelete = async () => {
    if (selectedKeys.length === 0) return;
    if (!window.confirm(`确定批量删除选中的 ${selectedKeys.length} 个模型定价规则？`)) {
      return;
    }
    const items = selectedKeys.map(k => {
      const idx = k.indexOf('::');
      if (idx === -1) return { group: 'default', model: k };
      return { group: k.substring(0, idx), model: k.substring(idx + 2) };
    });
    try {
      const res = await adminFetch('/api/v1/admin/pricing/batch-delete', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ items })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(data.message || '已批量删除定价规则', 'success');
        setSelectedKeys([]);
        fetchPrices();
      } else {
        showToast(data.error || '批量删除失败', 'warning');
      }
    } catch (err) {
      showToast('请求异常: ' + err.message, 'error');
    }
  };

  const handleSave = async (e) => {
    e.preventDefault();
    if (!formData.model.trim()) {
      setFormError('请输入模型标识 (例如 deepseek-chat 或 gpt-4o)');
      return;
    }

    if (formData.off_peak_enabled) {
      if (!formData.slots || formData.slots.length === 0) {
        setFormError('开启分时优惠后，请至少添加一个时间段');
        return;
      }
      for (let i = 0; i < formData.slots.length; i++) {
        const s = formData.slots[i];
        if (!s.start || !s.end) {
          setFormError(`时段 [${s.name || i + 1}] 开始与结束时间不能为空`);
          return;
        }
      }
      const overlapErr = checkSlotsOverlap(formData.slots);
      if (overlapErr) {
        setFormError(overlapErr);
        return;
      }
    }

    setSaving(true);
    setFormError('');

    const primarySlot = (formData.slots && formData.slots[0]) || { start: '00:00', end: '08:30', discount: 0.5 };
    const serializedSlots = formData.off_peak_enabled && formData.slots
      ? JSON.stringify(formData.slots.map(s => ({
          name: s.name || '',
          start: s.start || '00:00',
          end: s.end || '08:30',
          discount: parseFloat(s.discount) || 0.5,
          days: Array.isArray(s.days) ? s.days : []
        })))
      : '';

    try {
      const payload = {
        model: formData.model.trim(),
        group_name: (formData.group_name || 'default').trim(),
        prompt_price: parseFloat(formData.prompt_price) || 0,
        completion_price: parseFloat(formData.completion_price) || 0,
        cache_read_price: parseFloat(formData.cache_read_price) || 0,
        fixed_price: parseFloat(formData.fixed_price) || 0,
        currency: formData.currency || 'CNY',
        off_peak_enabled: !!formData.off_peak_enabled,
        off_peak_mode: formData.off_peak_enabled ? 'custom' : 'none',
        off_peak_discount: parseFloat(primarySlot.discount) || 0.5,
        off_peak_start: primarySlot.start || '00:00',
        off_peak_end: primarySlot.end || '08:30',
        off_peak_slots: serializedSlots,
        weekend_all_day: !!formData.weekend_all_day
      };

      const res = await adminFetch('/api/v1/admin/pricing', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(`已保存 [${payload.group_name}] 组模型 [${payload.model}] 定价规则`, 'success');
        setShowModal(false);
        fetchPrices();
      } else {
        setFormError(data.error || '保存失败');
      }
    } catch (err) {
      setFormError('网络请求异常: ' + err.message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-6 animate-in fade-in">
      {/* 1. Header with live status */}
      <div className="bg-white border border-slate-200/80 rounded-3xl p-6 shadow-xs">
        <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
          <div>
            <h2 className="text-lg font-bold text-slate-900 tracking-tight">
              模型定价
            </h2>
          </div>

          <div className="flex items-center space-x-2 shrink-0">
            <button
              onClick={handleOpenAdd}
              className="px-4 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-2xl text-xs font-semibold shadow-xs flex items-center space-x-1.5 transition cursor-pointer"
            >
              <Plus className="w-3.5 h-3.5" />
              <span>新建定价规则</span>
            </button>
            <button
              onClick={fetchPrices}
              disabled={loading}
              className="p-2 bg-slate-50 hover:bg-slate-100 border border-slate-200 text-slate-600 rounded-2xl transition cursor-pointer"
              title="刷新费率"
            >
              <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin text-indigo-600' : ''}`} />
            </button>
          </div>
        </div>

        {/* Server Time Indicator (Lightweight & Clean) */}
        <div className="mt-4 pt-4 border-t border-slate-100 flex flex-wrap items-center gap-4 text-xs text-slate-600">
          <div className="flex items-center space-x-1.5 font-mono">
            <Clock className="w-3.5 h-3.5 text-indigo-500" />
            <span>网关当前时区: <strong>CST (UTC+8)</strong></span>
            {serverTime && <span className="text-indigo-600 font-bold ml-1">{serverTime}</span>}
            {serverWeekday && <span className="text-slate-400">({serverWeekday})</span>}
          </div>
          {isWeekend && (
            <span className="px-2 py-0.5 rounded-full text-[11px] font-medium bg-emerald-50 text-emerald-700 border border-emerald-200">
              周末时段 (全天享受闲时优惠)
            </span>
          )}
        </div>
      </div>

      {/* Batch Action Bar */}
      {selectedKeys.length > 0 && (
        <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between bg-indigo-50 border border-indigo-200 px-5 py-3 rounded-2xl animate-in fade-in gap-3">
          <div className="flex items-center space-x-2 text-xs font-semibold text-indigo-900">
            <span>已选中 {selectedKeys.length} 个模型定价规则</span>
          </div>
          <div className="flex items-center space-x-2">
            <button
              onClick={handleBatchDelete}
              className="px-3 py-1.5 bg-rose-600 hover:bg-rose-700 text-white rounded-xl text-xs font-semibold shadow-xs transition flex items-center space-x-1 cursor-pointer"
            >
              <Trash2 className="w-3.5 h-3.5" />
              <span>批量删除</span>
            </button>
            <button
              onClick={() => setSelectedKeys([])}
              className="px-3 py-1.5 bg-white text-slate-700 border border-slate-200 text-xs font-medium rounded-xl hover:bg-slate-50 transition cursor-pointer"
            >
              取消选择
            </button>
          </div>
        </div>
      )}

      {/* 2. Rates Table (High Whitespace & Clean) */}
      <div className="bg-white border border-slate-200/80 rounded-3xl shadow-xs overflow-hidden">
        {/* Group Filter Tabs */}
        <div className="flex flex-wrap items-center gap-2 px-6 pt-5 pb-3 border-b border-slate-100">
          <span className="text-xs text-slate-400 font-medium mr-1">价格分组:</span>
          {[
            { id: 'all', label: '全部规则' },
            { id: 'default', label: '默认组 (default)' },
            { id: 'vip', label: 'VIP组 (vip)' },
            { id: 'enterprise', label: '企业组 (enterprise)' },
            ...availableGroups.filter(g => !['default', 'vip', 'enterprise'].includes(g)).map(g => ({ id: g, label: `${g} 组` }))
          ].map(tab => {
            const count = tab.id === 'all' ? prices.length : prices.filter(p => (p.group_name || 'default') === tab.id).length;
            const isActive = groupFilter === tab.id;
            return (
              <button
                key={tab.id}
                onClick={() => { setGroupFilter(tab.id); setSelectedKeys([]); }}
                className={`px-3 py-1.5 rounded-xl text-xs font-semibold transition cursor-pointer flex items-center space-x-1.5 ${
                  isActive
                    ? 'bg-indigo-600 text-white shadow-2xs'
                    : 'bg-slate-100 text-slate-600 hover:bg-slate-200/80 hover:text-slate-900'
                }`}
              >
                <span>{tab.label}</span>
                <span className={`text-[10px] px-1.5 py-0.2 rounded-full ${isActive ? 'bg-indigo-700/80 text-indigo-100' : 'bg-slate-200 text-slate-500'}`}>
                  {count}
                </span>
              </button>
            );
          })}
        </div>

        {loading && prices.length === 0 ? (
          <div className="p-12 text-center text-slate-400 text-xs flex items-center justify-center space-x-2">
            <RefreshCw className="w-4 h-4 animate-spin text-indigo-600" />
            <span>加载费率规则中...</span>
          </div>
        ) : filteredPrices.length === 0 ? (
          <div className="p-12 text-center text-slate-400 text-xs">
            {prices.length === 0 ? '暂无配置的定价规则，点击上方「新建定价规则」进行添加' : '当前分组下暂无定价规则，点击上方「新建定价规则」添加'}
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs border-collapse">
              <thead>
                <tr className="border-b border-slate-100 text-slate-400 font-medium bg-slate-50/50">
                  <th className="py-3 px-4 w-10 text-center">
                    <input
                      type="checkbox"
                      className="rounded border-slate-300 text-indigo-600 focus:ring-indigo-500 cursor-pointer"
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
                  <th className="py-3 px-6">模型标识</th>
                  <th className="py-3 px-4">适用用户组</th>
                  <th className="py-3 px-4">基准输入 (1M)</th>
                  <th className="py-3 px-4">基准输出 (1M)</th>
                  <th className="py-3 px-4">缓存命中 (1M)</th>
                  <th className="py-3 px-4">分时优惠时段</th>
                  <th className="py-3 px-4">当前生效费率</th>
                  <th className="py-3 px-6 text-right">操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 text-slate-700">
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
                      name: '优惠时段',
                      start: p.off_peak_start || '00:00',
                      end: p.off_peak_end || '08:30',
                      discount: p.off_peak_discount ?? 0.5
                    }];
                  }

                  return (
                    <tr key={itemKey} className="hover:bg-slate-50/60 transition-colors">
                      <td className="py-4 px-4 text-center" onClick={(e) => e.stopPropagation()}>
                        <input
                          type="checkbox"
                          className="rounded border-slate-300 text-indigo-600 focus:ring-indigo-500 cursor-pointer"
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
                      <td className="py-4 px-6 font-mono font-bold text-slate-900">
                        {p.model}
                      </td>

                      <td className="py-4 px-4">
                        <span className={`inline-flex items-center px-2 py-0.5 rounded-lg text-xs font-semibold border ${grpBadge.className}`}>
                          {grpBadge.label}
                        </span>
                      </td>

                      <td className="py-4 px-4 font-mono font-medium">
                        {currencySym}{Number(p.prompt_price || 0).toFixed(2)}
                      </td>

                      <td className="py-4 px-4 font-mono font-medium">
                        {currencySym}{Number(p.completion_price || 0).toFixed(2)}
                      </td>

                      <td className="py-4 px-4 font-mono text-emerald-700">
                        <div>
                          {currencySym}{Number(p.cache_read_price || 0).toFixed(2)}
                          {p.prompt_price > 0 && (
                            <span className="text-[10px] text-slate-400 ml-1">
                              ({Math.round(((p.prompt_price - p.cache_read_price) / p.prompt_price) * 100)}% 节省)
                            </span>
                          )}
                        </div>
                      </td>

                      {/* Multi-slot Off-peak policy column */}
                      <td className="py-4 px-4 font-sans">
                        {p.off_peak_enabled ? (
                          <div className="space-y-1.5">
                            <div className="flex flex-wrap gap-1.5">
                              {displaySlots.map((slot, sIdx) => {
                                const daysStr = formatDaysLabel(slot.days);
                                return (
                                  <span
                                    key={sIdx}
                                    className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-lg bg-indigo-50 border border-indigo-200 text-indigo-700 text-xs font-semibold"
                                  >
                                    <Moon className="w-3 h-3 text-indigo-500" />
                                    <span>
                                      {slot.name ? `${slot.name}: ` : ''}
                                      {daysStr ? `${daysStr} ` : ''}
                                      {slot.start}-{slot.end} ({Math.round((slot.discount ?? 0.5) * 10)}折)
                                    </span>
                                  </span>
                                );
                              })}
                            </div>
                            {p.weekend_all_day && (
                              <div className="text-[11px] text-slate-400 flex items-center space-x-1">
                                <Calendar className="w-3 h-3 text-slate-400" />
                                <span>周六日全天享受优惠</span>
                              </div>
                            )}
                          </div>
                        ) : (
                          <span className="text-slate-400 text-xs">全天统一定价</span>
                        )}
                      </td>

                      {/* Live effective rate */}
                      <td className="py-4 px-4 font-mono">
                        {isOff ? (
                          <div className="space-y-0.5">
                            <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-md bg-emerald-50 border border-emerald-200 text-emerald-700 text-[11px] font-bold">
                              <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse"></span>
                              <span>优惠 {Math.round(discount * 10)}折 生效中</span>
                            </span>
                            <div className="text-xs text-emerald-700 font-bold">
                              输入 {currencySym}{effPrompt.toFixed(2)} / 输出 {currencySym}{effComp.toFixed(2)}
                            </div>
                          </div>
                        ) : (
                          <div className="space-y-0.5">
                            <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-md bg-slate-100 text-slate-600 text-[11px] font-medium">
                              <span>基准单价</span>
                            </span>
                            <div className="text-xs text-slate-700 font-medium">
                              输入 {currencySym}{effPrompt.toFixed(2)} / 输出 {currencySym}{effComp.toFixed(2)}
                            </div>
                          </div>
                        )}
                      </td>

                      {/* Actions */}
                      <td className="py-4 px-6 text-right space-x-2 font-sans">
                        <button
                          onClick={() => handleOpenEdit(p)}
                          className="px-3 py-1 rounded-xl bg-indigo-50 hover:bg-indigo-100 text-indigo-700 text-xs font-medium transition cursor-pointer"
                        >
                          编辑
                        </button>
                        <button
                          onClick={() => handleDelete(p.model, p.group_name || 'default')}
                          className="px-3 py-1 rounded-xl bg-slate-100 hover:bg-rose-50 text-slate-500 hover:text-rose-600 text-xs font-medium transition cursor-pointer"
                        >
                          删除
                        </button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* 3. Flexible Multi-Slot Edit / Add Modal */}
      {showModal && (
        <div className="fixed inset-0 bg-slate-900/60 flex items-center justify-center p-4 z-50 backdrop-blur-sm animate-in fade-in">
          <div className="bg-white border border-slate-200 rounded-3xl p-6 sm:p-8 max-w-xl w-full shadow-2xl space-y-5 animate-in zoom-in-95 duration-150 max-h-[90vh] overflow-y-auto">
            <div className="border-b border-slate-100 pb-3 flex items-center justify-between">
              <div>
                <h3 className="font-bold text-slate-900 text-base">
                  {editingPrice ? `编辑 [${editingPrice.model}] 定价` : '新建模型定价与分时规则'}
                </h3>
              </div>
              <button
                onClick={() => setShowModal(false)}
                className="p-1.5 hover:bg-slate-100 rounded-xl text-slate-400 hover:text-slate-600 transition cursor-pointer"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            {formError && (
              <div className="p-3 bg-rose-50 border border-rose-200 rounded-2xl text-xs text-rose-700 flex items-center space-x-2">
                <AlertCircle className="w-4 h-4 shrink-0" />
                <span>{formError}</span>
              </div>
            )}

            <form onSubmit={handleSave} className="space-y-4 text-xs">
              {/* Field 1: Model Identifier */}
              <div>
                <label className="block text-slate-700 font-semibold mb-1">
                  模型名称 (Model) <span className="text-rose-500">*</span>
                </label>
                <input
                  type="text"
                  required
                  disabled={!!editingPrice}
                  list="unpriced-models-datalist"
                  placeholder="如 deepseek-chat 或点击下方一键选择"
                  value={formData.model}
                  onChange={(e) => setFormData({ ...formData, model: e.target.value })}
                  className="w-full bg-slate-50 border border-slate-200 rounded-xl px-3 py-2 font-mono text-slate-900 focus:outline-none focus:border-indigo-500 disabled:opacity-60 text-xs"
                />
                <datalist id="unpriced-models-datalist">
                  {unpricedModels.map(m => (
                    <option key={m} value={m} />
                  ))}
                </datalist>

                {!editingPrice && unpricedModels.length > 0 && (
                  <div className="mt-2 pt-2 border-t border-slate-100">
                    <span className="text-[11px] text-slate-500 font-medium block mb-1.5">
                      待配置定价的平台模型 (点击自动填入并适配参考费率):
                    </span>
                    <div className="flex flex-wrap gap-1.5 max-h-24 overflow-y-auto">
                      {unpricedModels.map(m => {
                        const isSelected = formData.model === m;
                        return (
                          <button
                            key={m}
                            type="button"
                            onClick={() => handleSelectModelCandidate(m)}
                            className={`px-2 py-0.5 rounded-lg border text-[11px] font-mono transition cursor-pointer ${
                              isSelected
                                ? 'bg-indigo-600 text-white border-indigo-600 font-semibold shadow-2xs'
                                : 'bg-white hover:bg-indigo-50 border-slate-200 text-slate-700 hover:text-indigo-700 hover:border-indigo-200'
                            }`}
                          >
                            {m}
                          </button>
                        );
                      })}
                    </div>
                  </div>
                )}
              </div>

              {/* Field: User Group Tier */}
              <div>
                <label className="block text-slate-700 font-semibold mb-1">
                  适用用户组 (Group Tier) <span className="text-rose-500">*</span>
                </label>
                {editingPrice ? (
                  <div className="flex items-center space-x-2 py-1">
                    <span className={`px-2.5 py-1 rounded-xl text-xs font-semibold border ${getGroupBadge(formData.group_name).className}`}>
                      {getGroupBadge(formData.group_name).label} ({formData.group_name})
                    </span>
                    <span className="text-[11px] text-slate-400">（已生效规则分组不可变更）</span>
                  </div>
                ) : (
                  <div className="space-y-2">
                    <div className="flex flex-wrap items-center gap-1.5">
                      {[
                        { id: 'default', label: '默认组 (default)' },
                        { id: 'vip', label: 'VIP组 (vip)' },
                        { id: 'enterprise', label: '企业组 (enterprise)' }
                      ].map(g => (
                        <button
                          key={g.id}
                          type="button"
                          onClick={() => setFormData({ ...formData, group_name: g.id })}
                          className={`px-3 py-1.5 rounded-xl text-xs font-medium border transition cursor-pointer ${
                            formData.group_name === g.id
                              ? 'bg-indigo-600 text-white border-indigo-600 font-semibold shadow-2xs'
                              : 'bg-white hover:bg-indigo-50/60 border-slate-200 text-slate-700'
                          }`}
                        >
                          {g.label}
                        </button>
                      ))}
                    </div>
                    <input
                      type="text"
                      placeholder="或输入自定义用户组标识 (如 partner / test 等)"
                      value={formData.group_name}
                      onChange={(e) => setFormData({ ...formData, group_name: e.target.value.toLowerCase().replace(/[^a-z0-9_-]/g, '') })}
                      className="w-full bg-slate-50 border border-slate-200 rounded-xl px-3 py-2 font-mono text-slate-900 focus:outline-none focus:border-indigo-500 text-xs"
                    />
                  </div>
                )}
              </div>

              {/* Field 2: Base Rates */}
              <div className="p-4 rounded-2xl bg-slate-50 border border-slate-200/80 space-y-3">
                <span className="font-bold text-slate-800 block text-xs">
                  基准单价 (每 1,000,000 Tokens)
                </span>
                <div className="grid grid-cols-3 gap-3">
                  <div>
                    <label className="block text-slate-600 font-medium mb-1">
                      输入单价 (¥)
                    </label>
                    <input
                      type="number"
                      step="0.01"
                      min="0"
                      required
                      value={formData.prompt_price}
                      onChange={(e) => {
                        const val = parseFloat(e.target.value) || 0;
                        setFormData({
                          ...formData,
                          prompt_price: e.target.value,
                          cache_read_price: (val * 0.1).toFixed(2)
                        });
                      }}
                      className="w-full bg-white border border-slate-200 rounded-xl px-2.5 py-1.5 font-mono text-slate-900 focus:outline-none focus:border-indigo-500 text-xs"
                    />
                  </div>

                  <div>
                    <label className="block text-slate-600 font-medium mb-1">
                      输出单价 (¥)
                    </label>
                    <input
                      type="number"
                      step="0.01"
                      min="0"
                      required
                      value={formData.completion_price}
                      onChange={(e) => setFormData({ ...formData, completion_price: e.target.value })}
                      className="w-full bg-white border border-slate-200 rounded-xl px-2.5 py-1.5 font-mono text-slate-900 focus:outline-none focus:border-indigo-500 text-xs"
                    />
                  </div>

                  <div>
                    <label className="block text-emerald-700 font-medium mb-1">
                      缓存命中 (¥)
                    </label>
                    <input
                      type="number"
                      step="0.01"
                      min="0"
                      value={formData.cache_read_price}
                      onChange={(e) => setFormData({ ...formData, cache_read_price: e.target.value })}
                      className="w-full bg-white border border-emerald-300 rounded-xl px-2.5 py-1.5 font-mono text-slate-900 focus:outline-none focus:border-emerald-500 text-xs"
                    />
                  </div>
                </div>
              </div>

              {/* Field 3: Time-of-Use Multi-Slot Flexible Configuration */}
              <div className="p-4 rounded-2xl bg-indigo-50/40 border border-indigo-100 space-y-4">
                <div className="flex items-center justify-between">
                  <div className="flex items-center space-x-2">
                    <Moon className="w-4 h-4 text-indigo-600" />
                    <span className="font-bold text-slate-900">
                      开启分时优惠
                    </span>
                  </div>
                  <label className="relative inline-flex items-center cursor-pointer">
                    <input
                      type="checkbox"
                      checked={formData.off_peak_enabled}
                      onChange={(e) => setFormData({ ...formData, off_peak_enabled: e.target.checked })}
                      className="sr-only peer"
                    />
                    <div className="w-9 h-5 bg-slate-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-slate-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-indigo-600"></div>
                  </label>
                </div>

                {formData.off_peak_enabled && (
                  <div className="space-y-3 pt-1 animate-in fade-in">
                    <div className="flex items-center justify-between">
                      <span className="text-xs font-semibold text-slate-700">
                        优惠时间段列表 (支持自由配置任意多个时段)
                      </span>
                      <button
                        type="button"
                        onClick={handleAddSlot}
                        className="inline-flex items-center space-x-1 px-3 py-1 rounded-xl bg-indigo-600 hover:bg-indigo-700 text-white text-xs font-semibold shadow-2xs transition cursor-pointer"
                      >
                        <Plus className="w-3.5 h-3.5" />
                        <span>添加时间段</span>
                      </button>
                    </div>

                    {/* Slots List */}
                    <div className="space-y-2.5">
                      {formData.slots.map((slot, index) => (
                        <div key={slot.id || index} className="p-3 bg-white rounded-xl border border-slate-200 shadow-2xs space-y-2.5">
                          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2.5">
                            <div className="flex items-center space-x-2 flex-1">
                              <span className="w-5 h-5 rounded-full bg-indigo-100 text-indigo-700 text-[10px] font-bold flex items-center justify-center shrink-0">
                                {index + 1}
                              </span>
                              <input
                                type="text"
                                placeholder="时段名称 (选填)"
                                value={slot.name}
                                onChange={(e) => handleUpdateSlot(slot.id, 'name', e.target.value)}
                                className="w-28 px-2.5 py-1.5 bg-slate-50 border border-slate-200 rounded-lg text-xs text-slate-800 focus:outline-none focus:border-indigo-500"
                              />
                            </div>

                            <div className="flex items-center space-x-2">
                              <input
                                type="text"
                                placeholder="00:00"
                                value={slot.start}
                                onChange={(e) => handleUpdateSlot(slot.id, 'start', e.target.value)}
                                className="w-18 px-2 py-1.5 bg-slate-50 border border-slate-200 rounded-lg font-mono text-center text-xs text-slate-800 focus:outline-none focus:border-indigo-500"
                              />
                              <span className="text-slate-400 text-xs">至</span>
                              <input
                                type="text"
                                placeholder="08:30"
                                value={slot.end}
                                onChange={(e) => handleUpdateSlot(slot.id, 'end', e.target.value)}
                                className="w-18 px-2 py-1.5 bg-slate-50 border border-slate-200 rounded-lg font-mono text-center text-xs text-slate-800 focus:outline-none focus:border-indigo-500"
                              />
                            </div>

                            <div className="flex items-center space-x-2">
                              <select
                                value={slot.discount}
                                onChange={(e) => handleUpdateSlot(slot.id, 'discount', parseFloat(e.target.value))}
                                className="px-2 py-1.5 bg-slate-50 border border-slate-200 rounded-lg text-xs font-mono text-slate-800 focus:outline-none focus:border-indigo-500"
                              >
                                <option value="0.1">1 折 (10%)</option>
                                <option value="0.2">2 折 (20%)</option>
                                <option value="0.3">3 折 (30%)</option>
                                <option value="0.4">4 折 (40%)</option>
                                <option value="0.5">5 折 (50%)</option>
                                <option value="0.6">6 折 (60%)</option>
                                <option value="0.7">7 折 (70%)</option>
                                <option value="0.8">8 折 (80%)</option>
                                <option value="0.85">8.5 折 (85%)</option>
                                <option value="0.9">9 折 (90%)</option>
                              </select>

                              {formData.slots.length > 1 && (
                                <button
                                  type="button"
                                  onClick={() => handleRemoveSlot(slot.id)}
                                  className="p-1.5 text-slate-400 hover:text-rose-600 hover:bg-rose-50 rounded-lg transition cursor-pointer"
                                  title="删除该时段"
                                >
                                  <Trash2 className="w-3.5 h-3.5" />
                                </button>
                              )}
                            </div>
                          </div>

                          {/* Zero-burden Weekday Selector */}
                          <div className="flex items-center justify-between pt-1.5 border-t border-slate-100 text-xs">
                            <span className="text-slate-400 text-[11px]">生效周期:</span>
                            <div className="flex items-center space-x-1">
                              {[
                                { label: '一', val: 1 },
                                { label: '二', val: 2 },
                                { label: '三', val: 3 },
                                { label: '四', val: 4 },
                                { label: '五', val: 5 },
                                { label: '六', val: 6 },
                                { label: '日', val: 7 }
                              ].map(({ label, val }) => {
                                const isActive = isDayActive(slot.days, val);
                                return (
                                  <button
                                    key={val}
                                    type="button"
                                    onClick={() => toggleSlotDay(slot.id, val)}
                                    className={`w-6 h-6 rounded-md text-[11px] font-medium transition cursor-pointer flex items-center justify-center ${
                                      isActive
                                        ? 'bg-indigo-600 text-white font-semibold shadow-2xs'
                                        : 'bg-slate-100 text-slate-400 hover:bg-slate-200'
                                    }`}
                                    title={`周${label}`}
                                  >
                                    {label}
                                  </button>
                                );
                              })}
                              <span className="text-[11px] text-indigo-600 font-medium ml-1.5">
                                {formatDaysLabel(slot.days) || '每天'}
                              </span>
                            </div>
                          </div>
                        </div>
                      ))}
                    </div>

                    <div className="pt-1">
                      <label className="flex items-center space-x-2 text-slate-700 cursor-pointer">
                        <input
                          type="checkbox"
                          checked={formData.weekend_all_day}
                          onChange={(e) => setFormData({ ...formData, weekend_all_day: e.target.checked })}
                          className="rounded border-slate-300 text-indigo-600 focus:ring-indigo-500"
                        />
                        <span className="font-medium">周六与周日全天享受优惠折扣</span>
                      </label>
                    </div>
                  </div>
                )}
              </div>

              {/* Field 4: Live Preview Box */}
              <div className="p-3.5 rounded-2xl bg-slate-50 border border-slate-200/80 space-y-2 text-xs">
                <div className="font-semibold text-slate-700">
                  <span>实时计费预览</span>
                </div>
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 text-xs">
                  <div className="p-2.5 rounded-xl bg-white/90 border border-indigo-100 space-y-1">
                    <span className="font-semibold text-slate-700 block flex items-center space-x-1">
                      <Sun className="w-3.5 h-3.5 text-amber-500 inline mr-1" />
                      基准单价
                    </span>
                    <p className="text-slate-600 font-mono text-[11px]">
                      输入: <strong>¥{parseFloat(formData.prompt_price || 0).toFixed(2)}</strong> / 1M
                    </p>
                    <p className="text-slate-600 font-mono text-[11px]">
                      输出: <strong>¥{parseFloat(formData.completion_price || 0).toFixed(2)}</strong> / 1M
                    </p>
                    <p className="text-emerald-700 font-mono text-[11px]">
                      缓存: <strong>¥{parseFloat(formData.cache_read_price || 0).toFixed(2)}</strong> / 1M
                    </p>
                  </div>

                  {formData.off_peak_enabled && formData.slots && formData.slots.length > 0 ? (
                    formData.slots.map((s, idx) => (
                      <div key={idx} className="p-2.5 rounded-xl bg-indigo-600/10 border border-indigo-200 space-y-1">
                        <span className="font-semibold text-indigo-900 block flex items-center space-x-1">
                          <Moon className="w-3.5 h-3.5 text-indigo-600 inline mr-1" />
                          {s.name || `时段 ${idx + 1}`} ({formatDaysLabel(s.days) || '每天'} {s.start || '00:00'}-{s.end || '08:30'}) · {Math.round((s.discount || 0.5) * 10)}折
                        </span>
                        <p className="text-indigo-900 font-mono text-[11px]">
                          输入: <strong className="text-indigo-700">¥{(parseFloat(formData.prompt_price || 0) * (s.discount || 0.5)).toFixed(2)}</strong> / 1M
                        </p>
                        <p className="text-indigo-900 font-mono text-[11px]">
                          输出: <strong className="text-indigo-700">¥{(parseFloat(formData.completion_price || 0) * (s.discount || 0.5)).toFixed(2)}</strong> / 1M
                        </p>
                      </div>
                    ))
                  ) : (
                    <div className="p-2.5 rounded-xl bg-slate-50 border border-slate-200 flex items-center justify-center text-slate-400 text-xs">
                      全天按基准单价计费
                    </div>
                  )}
                </div>
              </div>

              {/* Actions */}
              <div className="pt-2 flex items-center justify-end space-x-2.5">
                <button
                  type="button"
                  onClick={() => setShowModal(false)}
                  className="px-4 py-2 rounded-xl bg-slate-100 text-slate-700 hover:bg-slate-200 transition font-medium"
                >
                  取消
                </button>
                <button
                  type="submit"
                  disabled={saving}
                  className="px-5 py-2 rounded-xl bg-indigo-600 hover:bg-indigo-700 text-white font-bold transition shadow-sm flex items-center space-x-1.5 disabled:opacity-50"
                >
                  {saving ? (
                    <>
                      <RefreshCw className="w-3.5 h-3.5 animate-spin" />
                      <span>正在保存...</span>
                    </>
                  ) : (
                    <span>保存定价</span>
                  )}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
