import React, { useState, useEffect, useCallback } from 'react';
import {
  Plus,
  RefreshCw,
  Trash2,
  AlertCircle,
  Moon,
  X,
  CheckCircle2,
  Calendar,
  Clock,
  Info,
  ArrowRight,
  Layers,
  Percent,
  Sliders,
  Settings2,
  Tag
} from 'lucide-react';
import useModalA11y from '../hooks/useModalA11y';

function formatDaysLabel(days, isZh = true) {
  if (!days || days.length === 0 || days.length === 7) return '';
  const dayNames = isZh 
    ? { 1: '周一', 2: '周二', 3: '周三', 4: '周四', 5: '周五', 6: '周六', 7: '周日' }
    : { 1: 'Mon', 2: 'Tue', 3: 'Wed', 4: 'Thu', 5: 'Fri', 6: 'Sat', 7: 'Sun' };
  const sorted = [...days].sort((a, b) => a - b);
  if (sorted.length === 5 && sorted.every((d, i) => d === i + 1)) return isZh ? '工作日' : 'Weekdays';
  if (sorted.length === 2 && sorted[0] === 6 && sorted[1] === 7) return isZh ? '周末' : 'Weekends';
  return sorted.map(d => dayNames[d] || d).join(isZh ? '、' : ', ');
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

export function checkSlotsOverlap(slots, isZh = true) {
  if (!slots || slots.length < 2) return null;
  const dayNames = isZh 
    ? { 1: '周一', 2: '周二', 3: '周三', 4: '周四', 5: '周五', 6: '周六', 7: '周日' }
    : { 1: 'Mon', 2: 'Tue', 3: 'Wed', 4: 'Thu', 5: 'Fri', 6: 'Sat', 7: 'Sun' };

  for (let i = 0; i < slots.length; i++) {
    const s1 = slots[i];
    const s1Start = parseMinutes(s1.start);
    const s1End = parseMinutes(s1.end);
    if (s1Start === null || s1End === null) {
      return isZh ? `时段「${s1.name || i + 1}」时间格式无效 (格式需为 HH:MM)` : `Slot "${s1.name || i + 1}" format invalid (must be HH:MM)`;
    }
    if (s1Start === s1End) {
      return isZh ? `时段「${s1.name || i + 1}」开始时间与结束时间不能相同` : `Slot "${s1.name || i + 1}" start and end time cannot be the same`;
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
        return isZh ? `时段「${s2.name || j + 1}」开始时间与结束时间不能相同` : `Slot "${s2.name || j + 1}" start and end time cannot be the same`;
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
            const overlapDaysStr = commonDays.length === 7 ? (isZh ? '每天' : 'Everyday') : commonDays.map(d => dayNames[d] || d).join(isZh ? '、' : ', ');
            return isZh 
              ? `时段「${s1.name || i + 1}」(${s1.start}-${s1.end}) 与 时段「${s2.name || j + 1}」(${s2.start}-${s2.end}) 在【${overlapDaysStr}】存在时间重叠，请调整时段避免冲突`
              : `Slot "${s1.name || i + 1}" (${s1.start}-${s1.end}) overlaps with slot "${s2.name || j + 1}" (${s2.start}-${s2.end}) on [${overlapDaysStr}]. Please adjust to resolve.`;
          }
        }
      }
    }
  }
  return null;
}

function parseSlotsFromPrice(p, isZh = true) {
  if (p && p.off_peak_slots) {
    try {
      const parsed = JSON.parse(p.off_peak_slots);
      if (Array.isArray(parsed) && parsed.length > 0) {
        return parsed.map((s, idx) => ({
          id: `slot-${idx + 1}-${Date.now()}`,
          name: s.name || (isZh ? `时段 ${idx + 1}` : `Slot ${idx + 1}`),
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
      name: isZh ? '时段 1' : 'Slot 1',
      start: p.off_peak_start || '00:00',
      end: p.off_peak_end || '08:30',
      discount: p.off_peak_discount ?? 0.5,
      days: []
    }];
  }
  return [
    { id: `slot-1-${Date.now()}`, name: isZh ? '时段 1' : 'Slot 1', start: '00:00', end: '08:30', discount: 0.5, days: [] }
  ];
}

export default function PricingManager({
  adminFetch,
  showToast,
  stats = {},
  isAdmin = true,
  lang = 'zh',
  t,
  modelRoutes = [],
  setCurrentTab,
  groupMultipliers,
  onUpdateGroupMultipliers
}) {
  const isZh = lang === 'zh';
  const [pricingSubTab, setPricingSubTab] = useState('models'); // 'models' | 'groups'
  const [localGroupMultipliers, setLocalGroupMultipliers] = useState(groupMultipliers || { default: 1.0, vip: 0.8, enterprise: 1.5 });
  const [savingGroups, setSavingGroups] = useState(false);
  const [showAddGroupModal, setShowAddGroupModal] = useState(false);
  const [newGroupInputKey, setNewGroupInputKey] = useState('');
  const [newGroupInputMultiplier, setNewGroupInputMultiplier] = useState(0.8);
  const [tierPreviewGroup, setTierPreviewGroup] = useState('default');
  const [prices, setPrices] = useState([]);
  const [loading, setLoading] = useState(false);
  const [showModal, setShowModal] = useState(false);
  useModalA11y(showModal, () => setShowModal(false));
  useModalA11y(showAddGroupModal, () => setShowAddGroupModal(false));
  const [editingPrice, setEditingPrice] = useState(null);
  // The gateway reports its own clock because off-peak windows are evaluated in
  // the gateway's time zone, not the browser's. The header renders these next to
  // the rate table so "off-peak active" is interpretable.
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
      { id: 'slot-1', name: isZh ? '时段 1' : 'Slot 1', start: '00:00', end: '08:30', discount: 0.5, days: [] }
    ]
  });
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState('');

  // Wrapped so the mount effect can depend on it without re-running every render.
  // Its own dependencies are the props it reads, all of which are stable in
  // practice: the parent passes a module-level fetch and a memoised toast.
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
  }, [adminFetch, showToast, isZh]);

  useEffect(() => {
    if (groupMultipliers && typeof groupMultipliers === 'object') {
      setLocalGroupMultipliers(groupMultipliers);
    }
  }, [groupMultipliers]);

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

  const handleSaveGroupMultipliers = async (newMultipliers) => {
    const payload = newMultipliers || localGroupMultipliers;
    setSavingGroups(true);
    try {
      const res = await adminFetch('/api/v1/admin/pricing/groups', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        if (data.data) {
          setLocalGroupMultipliers(data.data);
        }
        showToast(isZh ? '分组折扣定价已保存并全网实时生效' : 'Group discount tiers saved and active in real-time', 'success');
        if (onUpdateGroupMultipliers) onUpdateGroupMultipliers();
      } else {
        showToast((isZh ? '保存分组倍率失败: ' : 'Failed to save group multipliers: ') + (data.error || '未知错误'), 'error');
      }
    } catch (err) {
      showToast((isZh ? '网络请求异常: ' : 'Network error: ') + err.message, 'error');
    } finally {
      setSavingGroups(false);
    }
  };

  const handleUpdateSingleMultiplier = (g, mult) => {
    const val = parseFloat(mult);
    if (isNaN(val) || val <= 0) return;
    setLocalGroupMultipliers(prev => ({
      ...prev,
      [g]: Math.round(val * 100) / 100
    }));
  };

  const handleDeleteGroupTier = (g) => {
    if (g === 'default') {
      showToast(isZh ? '默认基准组不可删除' : 'Default tier cannot be deleted', 'warning');
      return;
    }
    if (!window.confirm(isZh ? `确定删除计费分组 [${g}]？该组下的密钥后续将回退至默认 1.0x 基准价。` : `Delete group tier [${g}]?`)) {
      return;
    }
    const copy = { ...localGroupMultipliers };
    delete copy[g];
    setLocalGroupMultipliers(copy);
    handleSaveGroupMultipliers(copy);
  };

  const handleCreateNewGroupTier = (e) => {
    e.preventDefault();
    const cleanKey = newGroupInputKey.trim().toLowerCase().replace(/[^a-z0-9_-]/g, '');
    if (!cleanKey) {
      showToast(isZh ? '请输入有效的分组标识符 (英文小写/数字/下划线)' : 'Enter valid group identifier', 'warning');
      return;
    }
    const mult = parseFloat(newGroupInputMultiplier);
    if (isNaN(mult) || mult <= 0) {
      showToast(isZh ? '请输入有效的折扣倍数 (> 0)' : 'Enter valid multiplier (> 0)', 'warning');
      return;
    }
    const updated = {
      ...localGroupMultipliers,
      [cleanKey]: Math.round(mult * 100) / 100
    };
    setLocalGroupMultipliers(updated);
    setShowAddGroupModal(false);
    setNewGroupInputKey('');
    setNewGroupInputMultiplier(0.8);
    handleSaveGroupMultipliers(updated);
  };

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
  }, [fetchPrices]);

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
        { id: `slot-${Date.now()}`, name: isZh ? '时段 1' : 'Slot 1', start: '00:00', end: '08:30', discount: 0.5, days: [] }
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
      slots: parseSlotsFromPrice(p, isZh)
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
          name: isZh ? `时段 ${prev.slots.length + 1}` : `Slot ${prev.slots.length + 1}`,
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
    if (!window.confirm(isZh ? `确定删除模型 [${model}] 在 [${groupName}] 分组的定价规则？` : `Are you sure you want to delete pricing rule for model [${model}] in group [${groupName}]?`)) {
      return;
    }
    try {
      const query = new URLSearchParams({ model, group: groupName || 'default' });
      const res = await adminFetch(`/api/v1/admin/pricing?${query}`, {
        method: 'DELETE'
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(data.warning || (isZh ? `已删除 [${groupName}] 组模型 [${model}] 定价规则` : `Deleted pricing rule for model [${model}] in [${groupName}]`), data.warning ? 'warning' : 'success');
        fetchPrices();
      } else {
        showToast(data.error || (isZh ? '删除失败' : 'Failed to delete'), 'warning');
      }
    } catch (err) {
      showToast((isZh ? '请求异常: ' : 'Request error: ') + err.message, 'error');
    }
  };

  const handleBatchDelete = async () => {
    if (selectedKeys.length === 0) return;
    if (!window.confirm(isZh ? `确定批量删除选中的 ${selectedKeys.length} 个模型定价规则？` : `Delete selected ${selectedKeys.length} pricing rules?`)) {
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
        showToast(data.warning || (data.message || (isZh ? '已批量删除定价规则' : 'Batch deleted pricing rules')), data.warning ? 'warning' : 'success');
        setSelectedKeys([]);
        fetchPrices();
      } else {
        showToast(data.error || (isZh ? '批量删除失败' : 'Batch delete failed'), 'warning');
      }
    } catch (err) {
      showToast((isZh ? '请求异常: ' : 'Request error: ') + err.message, 'error');
    }
  };

  const handleSave = async (e) => {
    e.preventDefault();
    if (!formData.model.trim()) {
      setFormError(isZh ? '请输入模型标识 (例如 deepseek-chat 或 gpt-4o)' : 'Please enter model ID (e.g. deepseek-chat or gpt-4o)');
      return;
    }

    if (formData.off_peak_enabled) {
      if (!formData.slots || formData.slots.length === 0) {
        setFormError(isZh ? '开启分时优惠后，请至少添加一个时间段' : 'Please add at least one time slot when off-peak discount is enabled');
        return;
      }
      for (let i = 0; i < formData.slots.length; i++) {
        const s = formData.slots[i];
        if (!s.start || !s.end) {
          setFormError(isZh ? `时段 [${s.name || i + 1}] 开始与结束时间不能为空` : `Slot [${s.name || i + 1}] start and end time cannot be empty`);
          return;
        }
      }
      const overlapErr = checkSlotsOverlap(formData.slots, isZh);
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
        showToast(data.warning || (isZh ? `已保存 [${payload.group_name}] 组模型 [${payload.model}] 定价规则` : `Saved pricing rule for [${payload.model}] in [${payload.group_name}]`), data.warning ? 'warning' : 'success');
        setShowModal(false);
        fetchPrices();
      } else {
        setFormError(data.error || (isZh ? '保存失败' : 'Failed to save'));
      }
    } catch (err) {
      setFormError((isZh ? '网络请求异常: ' : 'Network error: ') + err.message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-6 animate-in fade-in">
      {/* 1. Header (Directly on canvas) */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight text-slate-900 dark:text-slate-100">
            {isZh ? '模型定价' : 'Model Pricing'}
          </h1>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
            {isZh ? '配置与管理官方参考基准定价与业务分组折扣倍率' : 'Configure official baseline token pricing and tier discount multipliers'}
          </p>
        </div>

        <div className="flex items-center space-x-2 shrink-0">
          {isAdmin && (
            <button
              onClick={handleOpenAdd}
              className="px-4 py-2 bg-slate-900 dark:bg-indigo-600 hover:bg-slate-800 dark:hover:bg-indigo-500 text-white rounded-lg text-xs font-medium shadow-sm flex items-center space-x-1.5 transition cursor-pointer"
            >
              <Plus className="w-3.5 h-3.5" />
              <span>{isZh ? '新建定价规则' : 'New Pricing Rule'}</span>
            </button>
          )}
          <button
            onClick={fetchPrices}
            disabled={loading}
            className="p-2 bg-white dark:bg-slate-800 hover:bg-slate-50 dark:hover:bg-slate-700 border border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300 rounded-lg transition cursor-pointer shadow-sm"
            title={isZh ? '刷新费率' : 'Refresh'}
          >
            <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin text-indigo-600' : ''}`} />
          </button>
        </div>
      </div>

      {/* 1.5 Sub Navigation: Model Baseline Pricing vs Group Discount Tiers */}
      <div className="flex items-center space-x-2 border-b border-slate-200/80 dark:border-slate-800 pb-2">
        <button
          type="button"
          onClick={() => setPricingSubTab('models')}
          className={`flex items-center space-x-2 px-4 py-2 rounded-xl text-xs font-semibold transition cursor-pointer ${
            pricingSubTab === 'models'
              ? 'bg-slate-900 dark:bg-indigo-600 text-white shadow-sm'
              : 'bg-white dark:bg-slate-800/80 border border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200'
          }`}
        >
          <Layers className="w-4 h-4" />
          <span>{isZh ? '模型官方基准定价' : 'Model Base Rates'}</span>
          <span className="text-[10px] px-1.5 py-0.2 rounded-full bg-slate-700/80 dark:bg-indigo-700/80 text-white font-mono">
            {prices.length}
          </span>
        </button>

        <button
          type="button"
          onClick={() => setPricingSubTab('groups')}
          className={`flex items-center space-x-2 px-4 py-2 rounded-xl text-xs font-semibold transition cursor-pointer ${
            pricingSubTab === 'groups'
              ? 'bg-slate-900 dark:bg-indigo-600 text-white shadow-sm'
              : 'bg-white dark:bg-slate-800/80 border border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200'
          }`}
        >
          <Percent className="w-4 h-4" />
          <span>{isZh ? '分组折扣定价' : 'Group Discount Tiers'}</span>
          <span className="text-[10px] px-1.5 py-0.2 rounded-full bg-slate-700/80 dark:bg-indigo-700/80 text-white font-mono">
            {Object.keys(localGroupMultipliers).length}
          </span>
        </button>
      </div>

      {/* Batch Action Bar (Admin only) */}
      {isAdmin && selectedKeys.length > 0 && (
        <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between bg-indigo-50 dark:bg-indigo-950/40 border border-indigo-200 dark:border-indigo-800 px-5 py-3 rounded-xl animate-in fade-in gap-3">
          <div className="flex items-center space-x-2 text-xs font-semibold text-indigo-900 dark:text-indigo-200">
            <span>{isZh ? `已选中 ${selectedKeys.length} 个模型定价规则` : `${selectedKeys.length} pricing rule(s) selected`}</span>
          </div>
          <div className="flex items-center space-x-2">
            <button
              onClick={handleBatchDelete}
              className="px-3 py-1.5 bg-rose-600 hover:bg-rose-700 text-white rounded-lg text-xs font-medium shadow-sm transition flex items-center space-x-1 cursor-pointer"
            >
              <Trash2 className="w-3.5 h-3.5" />
              <span>{isZh ? '批量删除' : 'Delete Selected'}</span>
            </button>
            <button
              onClick={() => setSelectedKeys([])}
              className="px-3 py-1.5 bg-white dark:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-200 dark:border-slate-700 text-xs font-medium rounded-lg hover:bg-slate-50 dark:hover:bg-slate-700 transition cursor-pointer"
            >
              {isZh ? '取消选择' : 'Deselect'}
            </button>
          </div>
        </div>
      )}

      {/* 2. Rates Table (High Whitespace & Clean) */}
      {pricingSubTab === 'models' && (
        <div className="space-y-4">
          {/* Architecture Guidance Banner */}
          <div className="p-3.5 rounded-2xl border border-indigo-100 dark:border-indigo-900/60 bg-indigo-50/60 dark:bg-indigo-950/30 flex items-start sm:items-center justify-between gap-3 text-xs">
            <div className="flex items-center space-x-2.5 text-indigo-950 dark:text-indigo-200 min-w-0">
              <Info className="w-4 h-4 text-indigo-600 dark:text-indigo-400 shrink-0 mt-0.5 sm:mt-0" />
              <span>
                {isZh
                  ? '模型定价统一定义供应商官方基准单价（Prompt / Completion / 缓存与闲时规则）。业务分组（VIP 8折、企业 1.5倍等）在此基准上按折扣倍数自动换算生效，无需为每个分组重复配置相同模型。'
                  : 'Model rates define official baseline prices. Group tiers automatically inherit these rates multiplied by their discount factor.'}
              </span>
            </div>
            <button
              type="button"
              onClick={() => setPricingSubTab('groups')}
              className="shrink-0 text-indigo-600 dark:text-indigo-400 font-semibold hover:underline inline-flex items-center space-x-1 cursor-pointer"
            >
              <span>{isZh ? '配置分组倍率' : 'Configure Multipliers'}</span>
              <ArrowRight className="w-3 h-3" />
            </button>
          </div>

          <div className="bg-white dark:bg-[#0F1420] border border-slate-200/70 dark:border-slate-800/80 rounded-2xl shadow-sm overflow-hidden">
            {/* Group Filter Tabs & Tier Preview */}
            <div className="flex flex-wrap items-center justify-between gap-2 px-6 pt-5 pb-3 border-b border-slate-100 dark:border-slate-800">
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-xs text-slate-400 font-medium mr-1">{isZh ? '价格分组:' : 'Price Group:'}</span>
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
                          ? 'bg-slate-900 dark:bg-indigo-600 text-white shadow-sm'
                          : 'bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-400 hover:bg-slate-200/80 dark:hover:bg-slate-700 hover:text-slate-900 dark:hover:text-slate-200'
                      }`}
                    >
                      <span>{tab.label}</span>
                      <span className={`text-[10px] px-1.5 py-0.2 rounded-full ${isActive ? 'bg-slate-800 dark:bg-indigo-700/80 text-white' : 'bg-slate-200 dark:bg-slate-700 text-slate-600 dark:text-slate-400'}`}>
                        {count}
                      </span>
                    </button>
                  );
                })}
              </div>

              {/* Rate Preview Tier Selector */}
              <div className="flex items-center space-x-2">
                <span className="text-xs text-slate-400 font-medium">{isZh ? '折算生效预览:' : 'Preview Tier:'}</span>
                <select
                  value={tierPreviewGroup}
                  onChange={(e) => setTierPreviewGroup(e.target.value)}
                  className="text-xs font-semibold py-1 px-2.5 rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 text-slate-700 dark:text-slate-200 cursor-pointer focus:outline-none focus:ring-1 focus:ring-indigo-500"
                >
                  <option value="default">{isZh ? '官方基准定价 (1.0x)' : 'Official Baseline (1.0x)'}</option>
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
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs border-collapse">
              <thead>
                <tr className="border-b border-slate-100 dark:border-slate-800 text-slate-400 dark:text-slate-400 font-medium bg-slate-50/50 dark:bg-slate-900/50">
                  {isAdmin && (
                    <th className="py-3 px-4 w-10 text-center">
                      <input
                        type="checkbox"
                        className="rounded-sm border-slate-300 dark:border-slate-700 text-indigo-600 focus:ring-indigo-500 cursor-pointer"
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
                  <th className="py-3 px-6">{isZh ? '模型标识' : 'Model'}</th>
                  <th className="py-3 px-4">{isZh ? '适用用户组' : 'Group Tier'}</th>
                  <th className="py-3 px-4">{isZh ? '基准输入 (1M)' : 'Prompt (1M)'}</th>
                  <th className="py-3 px-4">{isZh ? '基准输出 (1M)' : 'Completion (1M)'}</th>
                  <th className="py-3 px-4">{isZh ? '缓存命中 (1M)' : 'Cache Hit (1M)'}</th>
                  <th className="py-3 px-4">{isZh ? '分时优惠时段' : 'Off-Peak Windows'}</th>
                  <th className="py-3 px-4">{isZh ? '当前生效费率' : 'Live Rate'}</th>
                  {isAdmin && <th className="py-3 px-6 text-right">{isZh ? '操作' : 'Actions'}</th>}
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800/80 text-slate-700 dark:text-slate-300">
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
                    <tr key={itemKey} className="hover:bg-slate-50/60 dark:hover:bg-slate-800/40 transition-colors">
                      {isAdmin && (
                        <td className="py-4 px-4 text-center" onClick={(e) => e.stopPropagation()}>
                          <input
                            type="checkbox"
                            className="rounded-sm border-slate-300 dark:border-slate-700 text-indigo-600 focus:ring-indigo-500 cursor-pointer"
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
                      <td className="py-4 px-6 font-mono font-bold text-slate-900 dark:text-slate-100">
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

                      <td className="py-4 px-4 font-mono text-emerald-700 dark:text-emerald-400">
                        <div>
                          {currencySym}{Number(p.cache_read_price || 0).toFixed(2)}
                          {p.prompt_price > 0 && (
                            <span className="text-xs text-slate-500 dark:text-slate-400 ml-1">
                              ({Math.round(((p.prompt_price - p.cache_read_price) / p.prompt_price) * 100)}% {isZh ? '节省' : 'Saved'})
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
                                const daysStr = formatDaysLabel(slot.days, isZh);
                                return (
                                  <span
                                    key={sIdx}
                                    className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-lg bg-indigo-50 dark:bg-indigo-950/40 border border-indigo-200 dark:border-indigo-800 text-indigo-700 dark:text-indigo-300 text-xs font-semibold"
                                  >
                                    <Moon className="w-3 h-3 text-indigo-500" />
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
                                <Calendar className="w-3 h-3 text-slate-400" />
                                <span>{isZh ? '周六日全天享受优惠' : 'Weekend all-day discount active'}</span>
                              </div>
                            )}
                          </div>
                        ) : (
                          <span className="text-slate-400 text-xs">{isZh ? '全天统一定价' : 'Uniform 24/7 Pricing'}</span>
                        )}
                      </td>

                      {/* Live effective rate */}
                      <td className="py-4 px-4 font-mono">
                        {(() => {
                          const previewMult = tierPreviewGroup !== 'default' ? (localGroupMultipliers[tierPreviewGroup] ?? 1.0) : 1.0;
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
                              <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-md bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-300 text-xs font-medium">
                                <span>{isZh ? '基准单价' : 'Standard Rate'}</span>
                                {showTierTag && <span className="text-[10px] ml-1 font-sans text-indigo-600 dark:text-indigo-400 font-bold">({tierPreviewGroup} {previewMult}x)</span>}
                              </span>
                              <div className="text-xs text-slate-700 dark:text-slate-300 font-medium">
                                {isZh ? '输入' : 'Prompt'} {currencySym}{calcPrompt.toFixed(2)} / {isZh ? '输出' : 'Output'} {currencySym}{calcComp.toFixed(2)}
                              </div>
                            </div>
                          );
                        })()}
                      </td>

                      {/* Actions */}
                      {isAdmin && (
                        <td className="py-4 px-6 text-right space-x-2 font-sans">
                          <button
                            onClick={() => handleOpenEdit(p)}
                            className="px-3 py-1 rounded-xl bg-indigo-50 dark:bg-indigo-950/40 hover:bg-indigo-100 dark:hover:bg-indigo-900/60 text-indigo-700 dark:text-indigo-300 text-xs font-medium transition cursor-pointer"
                          >
                            {isZh ? '编辑' : 'Edit'}
                          </button>
                          <button
                            onClick={() => handleDelete(p.model, p.group_name || 'default')}
                            className="px-3 py-1 rounded-xl bg-slate-100 dark:bg-slate-800 hover:bg-rose-50 dark:hover:bg-rose-950/40 text-slate-500 dark:text-slate-400 hover:text-rose-600 dark:hover:text-rose-400 text-xs font-medium transition cursor-pointer"
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
        )}
      </div>
    </div>
  )}

      {/* 2.5 Group Discount Tiers Management View */}
      {pricingSubTab === 'groups' && (
        <div className="space-y-6">
          {/* Architecture Banner */}
          <div className="bg-gradient-to-r from-indigo-500/10 via-purple-500/10 to-transparent border border-indigo-200/80 dark:border-indigo-800/60 rounded-2xl p-5 shadow-xs">
            <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
              <div className="space-y-1">
                <div className="flex items-center space-x-2 text-indigo-950 dark:text-indigo-200 font-semibold text-sm">
                  <Sliders className="w-4 h-4 text-indigo-600 dark:text-indigo-400 shrink-0" />
                  <span>{isZh ? '业务分组折扣定价体系' : 'Group Discount & Tier Pricing'}</span>
                </div>
                <div className="text-xs text-slate-600 dark:text-slate-400 font-mono">
                  {isZh
                    ? '扣费公式: 实际调用单价 = 官方模型基准单价 × 密钥所属分组倍率 (× 闲时时段折扣)'
                    : 'Charge Formula: Base Model Rate * Group Multiplier (* Off-Peak Discount)'}
                </div>
              </div>

              {isAdmin && (
                <div className="flex items-center space-x-2 shrink-0">
                  <button
                    type="button"
                    onClick={() => setShowAddGroupModal(true)}
                    className="px-3.5 py-2 bg-indigo-50 dark:bg-indigo-950/60 hover:bg-indigo-100 dark:hover:bg-indigo-900/80 text-indigo-700 dark:text-indigo-300 border border-indigo-300 dark:border-indigo-700 rounded-xl text-xs font-semibold flex items-center space-x-1.5 transition cursor-pointer"
                  >
                    <Plus className="w-3.5 h-3.5" />
                    <span>{isZh ? '新建分组' : 'New Group Tier'}</span>
                  </button>
                  <button
                    type="button"
                    disabled={savingGroups}
                    onClick={() => handleSaveGroupMultipliers()}
                    className="px-4 py-2 bg-slate-900 dark:bg-indigo-600 hover:bg-slate-800 dark:hover:bg-indigo-500 text-white rounded-xl text-xs font-semibold shadow-sm flex items-center space-x-1.5 transition cursor-pointer disabled:opacity-50"
                  >
                    <span>{savingGroups ? (isZh ? '保存中...' : 'Saving...') : (isZh ? '保存全部倍率' : 'Save Multipliers')}</span>
                  </button>
                </div>
              )}
            </div>
          </div>

          {/* Group Tiers Grid */}
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {Object.entries(localGroupMultipliers).map(([grp, mult]) => {
              const isDefault = grp === 'default';
              const isVip = grp === 'vip';
              const isEnterprise = grp === 'enterprise';
              const discountPercent = Math.round((1 - mult) * 100);

              return (
                <div
                  key={grp}
                  className="bg-white dark:bg-[#0F1420] border border-slate-200/80 dark:border-slate-800 rounded-2xl p-5 shadow-sm space-y-4 hover:border-indigo-300 dark:hover:border-indigo-700 transition flex flex-col justify-between"
                >
                  <div className="space-y-3">
                    <div className="flex items-start justify-between">
                      <div>
                        <div className="flex items-center space-x-2">
                          <span className="font-bold text-slate-900 dark:text-slate-100 text-sm font-mono">
                            {grp.toUpperCase()}
                          </span>
                          <span className={`px-2 py-0.5 rounded-md text-[10px] font-bold border ${getGroupBadge(grp).className}`}>
                            {getGroupBadge(grp).label}
                          </span>
                        </div>
                        <div className="text-[11px] text-slate-500 dark:text-slate-400 mt-1">
                          {isDefault
                            ? (isZh ? '官方基准参考价 (100% 基线)' : 'Official Baseline (100%)')
                            : isVip
                            ? (isZh ? 'VIP专享优惠，全模型适用' : 'VIP Discount Tier')
                            : isEnterprise
                            ? (isZh ? '企业专属保障，含高并发与SLA' : 'Enterprise Tier with SLA')
                            : (isZh ? '自定义业务折扣组' : 'Custom Discount Group')}
                        </div>
                      </div>

                      {!isDefault && isAdmin && (
                        <button
                          type="button"
                          onClick={() => handleDeleteGroupTier(grp)}
                          className="p-1.5 text-slate-400 hover:text-rose-600 dark:hover:text-rose-400 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition cursor-pointer"
                          title={isZh ? '删除该分组' : 'Delete Group'}
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                        </button>
                      )}
                    </div>

                    {/* Multiplier display & adjustments */}
                    <div className="bg-slate-50 dark:bg-slate-900/60 rounded-xl p-3 border border-slate-100 dark:border-slate-800/80 space-y-2">
                      <div className="flex items-center justify-between">
                        <span className="text-xs text-slate-500 dark:text-slate-400 font-medium">
                          {isZh ? '计费倍数 (Multiplier)' : 'Rate Multiplier'}
                        </span>
                        <div className="flex items-center space-x-1.5">
                          {isDefault ? (
                            <span className="font-mono font-bold text-sm text-slate-700 dark:text-slate-300">
                              1.00x
                            </span>
                          ) : (
                            <div className="flex items-center space-x-1">
                              <input
                                type="number"
                                step="0.05"
                                min="0.01"
                                max="10"
                                disabled={!isAdmin}
                                value={mult}
                                onChange={(e) => handleUpdateSingleMultiplier(grp, e.target.value)}
                                className="w-20 px-2 py-1 text-right font-mono font-bold text-sm bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-lg text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-indigo-500"
                              />
                              <span className="text-xs font-mono text-slate-400">x</span>
                            </div>
                          )}
                        </div>
                      </div>

                      <div className="flex items-center justify-between text-[11px]">
                        <span className="text-slate-400">
                          {isZh ? '折算比例:' : 'Ratio:'}
                        </span>
                        <span className={`font-semibold ${mult < 1.0 ? 'text-emerald-600 dark:text-emerald-400' : mult > 1.0 ? 'text-amber-600 dark:text-amber-400' : 'text-slate-600 dark:text-slate-300'}`}>
                          {mult === 1.0
                            ? (isZh ? '官方基准 (100%)' : 'Standard 100%')
                            : mult < 1.0
                            ? (isZh ? `${Math.round(mult * 10)}折优惠 (-${discountPercent}%)` : `${discountPercent}% Discount`)
                            : (isZh ? `${Math.round(mult * 100)}% 上浮 (+${Math.round((mult - 1) * 100)}%)` : `+${Math.round((mult - 1) * 100)}% Surcharge`)}
                        </span>
                      </div>

                      {!isDefault && isAdmin && (
                        <div className="pt-2 flex flex-wrap gap-1 border-t border-slate-200/60 dark:border-slate-800/60">
                          {[0.5, 0.7, 0.8, 0.9, 1.0, 1.2, 1.5].map((preset) => (
                            <button
                              key={preset}
                              type="button"
                              onClick={() => handleUpdateSingleMultiplier(grp, preset)}
                              className={`px-2 py-0.5 rounded text-[10px] font-mono font-medium transition cursor-pointer ${
                                mult === preset
                                  ? 'bg-indigo-600 text-white font-bold'
                                  : 'bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-700'
                              }`}
                            >
                              {preset}x
                            </button>
                          ))}
                        </div>
                      )}
                    </div>
                  </div>
                </div>
              );
            })}
          </div>

          {/* Add Group Modal */}
          {showAddGroupModal && (
            <div
              className="fixed inset-0 bg-slate-900/60 flex items-center justify-center p-4 z-50 backdrop-blur-sm animate-in fade-in"
              onClick={(e) => {
                if (e.target === e.currentTarget) setShowAddGroupModal(false);
              }}
            >
              <div className="bg-white dark:bg-[#0F1420] border border-slate-200/80 dark:border-slate-800 rounded-2xl p-6 max-w-md w-full shadow-2xl space-y-4 animate-in zoom-in-95">
                <div className="flex items-center justify-between border-b border-slate-100 dark:border-slate-800 pb-3">
                  <h3 className="font-semibold text-slate-900 dark:text-slate-100 text-sm">
                    {isZh ? '新建计费分组' : 'New Pricing Group Tier'}
                  </h3>
                  <button
                    type="button"
                    onClick={() => setShowAddGroupModal(false)}
                    className="p-1 hover:bg-slate-100 dark:hover:bg-slate-800 rounded-lg text-slate-400 hover:text-slate-600 transition"
                  >
                    <X className="w-4 h-4" />
                  </button>
                </div>

                <form onSubmit={handleCreateNewGroupTier} className="space-y-4 text-xs">
                  <div>
                    <label className="block text-slate-700 dark:text-slate-300 font-semibold mb-1">
                      {isZh ? '分组英文标识 (Group ID)' : 'Group Identifier'} <span className="text-rose-500">*</span>
                    </label>
                    <input
                      type="text"
                      required
                      placeholder={isZh ? '如 partner / test_tier / svip' : 'e.g. partner / svip'}
                      value={newGroupInputKey}
                      onChange={(e) => setNewGroupInputKey(e.target.value.toLowerCase().replace(/[^a-z0-9_-]/g, ''))}
                      className="w-full px-3 py-2 rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 text-slate-900 dark:text-slate-100 font-mono text-xs focus:outline-none focus:ring-1 focus:ring-indigo-500"
                    />
                    <div className="ui-meta mt-1">
                      {isZh ? '仅支持小写字母、数字、下划线及连字符' : 'Lowercase letters, digits, hyphen, underscore only'}
                    </div>
                  </div>

                  <div>
                    <label className="block text-slate-700 dark:text-slate-300 font-semibold mb-1">
                      {isZh ? '费率倍数 (Multiplier)' : 'Multiplier'} <span className="text-rose-500">*</span>
                    </label>
                    <div className="flex items-center space-x-2">
                      <input
                        type="number"
                        step="0.05"
                        min="0.01"
                        max="10"
                        required
                        value={newGroupInputMultiplier}
                        onChange={(e) => setNewGroupInputMultiplier(e.target.value)}
                        className="w-full px-3 py-2 rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 text-slate-900 dark:text-slate-100 font-mono text-xs focus:outline-none focus:ring-1 focus:ring-indigo-500"
                      />
                      <span className="font-mono text-slate-400">x</span>
                    </div>
                    <div className="ui-meta mt-1">
                      {isZh ? '0.8 为 8折，1.5 为 1.5倍。该分组密钥将按此倍率乘以官方基准单价扣费。' : 'Base model prices will be multiplied by this factor.'}
                    </div>
                  </div>

                  <div className="flex justify-end space-x-2 pt-2">
                    <button
                      type="button"
                      onClick={() => setShowAddGroupModal(false)}
                      className="px-4 py-2 border border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-400 rounded-xl text-xs hover:bg-slate-50 dark:hover:bg-slate-800 transition"
                    >
                      {isZh ? '取消' : 'Cancel'}
                    </button>
                    <button
                      type="submit"
                      className="px-4 py-2 bg-slate-900 dark:bg-indigo-600 hover:bg-slate-800 dark:hover:bg-indigo-500 text-white rounded-xl text-xs font-semibold shadow-sm transition"
                    >
                      {isZh ? '确认添加' : 'Add Group'}
                    </button>
                  </div>
                </form>
              </div>
            </div>
          )}
        </div>
      )}

      {/* 3. Flexible Multi-Slot Edit / Add Modal */}
      {showModal && (
        <div
          className="fixed inset-0 bg-slate-900/60 flex items-center justify-center p-4 z-50 backdrop-blur-sm animate-in fade-in"
          onClick={(e) => {
            if (e.target === e.currentTarget) setShowModal(false);
          }}
        >
          <div className="bg-white dark:bg-[#0F1420] border border-slate-200/80 dark:border-slate-800 rounded-2xl p-6 sm:p-8 max-w-xl w-full shadow-2xl space-y-5 animate-in zoom-in-95 duration-150 max-h-[90vh] overflow-y-auto">
            <div className="border-b border-slate-100 dark:border-slate-800 pb-3 flex items-center justify-between">
              <div>
                <h3 className="font-semibold text-slate-900 dark:text-slate-100 text-base">
                  {editingPrice 
                    ? (isZh ? `编辑 [${editingPrice.model}] 定价` : `Edit [${editingPrice.model}] Pricing`) 
                    : (isZh ? '新建模型定价与分时规则' : 'New Pricing & Off-Peak Rule')}
                </h3>
              </div>
              <button
                onClick={() => setShowModal(false)}
                className="p-1.5 hover:bg-slate-100 dark:hover:bg-slate-800 rounded-lg text-slate-400 hover:text-slate-600 dark:hover:text-slate-300 transition cursor-pointer"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            {formError && (
              <div className="p-3 bg-rose-50 dark:bg-rose-950/40 border border-rose-200 dark:border-rose-800 rounded-lg text-xs text-rose-700 dark:text-rose-300 flex items-center space-x-2">
                <AlertCircle className="w-4 h-4 shrink-0" />
                <span>{formError}</span>
              </div>
            )}

            <form onSubmit={handleSave} className="space-y-4 text-xs">
              {/* Field 1: Model Identifier */}
              <div>
                <label className="block text-slate-700 dark:text-slate-300 font-semibold mb-1">
                  {isZh ? '模型名称 (Model)' : 'Model ID'} <span className="text-rose-500">*</span>
                </label>
                <input
                  type="text"
                  required
                  disabled={!!editingPrice}
                  list="unpriced-models-datalist"
                  placeholder={isZh ? '如 deepseek-chat 或点击下方一键选择' : 'e.g. deepseek-chat or select below'}
                  value={formData.model}
                  onChange={(e) => setFormData({ ...formData, model: e.target.value })}
                  className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded-lg px-3 py-2 font-mono text-slate-900 dark:text-slate-100 focus:outline-none focus:border-indigo-500 disabled:opacity-60 text-xs"
                />
                <datalist id="unpriced-models-datalist">
                  {unpricedModels.map(m => (
                    <option key={m} value={m} />
                  ))}
                </datalist>
                {/* The placeholder promised "click below to select", but no such row
                    existed: handleSelectModelCandidate was written and never wired.
                    It prefills a rate that suits the model's kind, which is the
                    part an operator cannot guess. */}
                {!editingPrice && candidateModels.length > 0 && (
                  <div className="mt-2.5 space-y-1.5">
                    <div className="text-[11px] text-slate-500 dark:text-slate-400 font-medium flex items-center justify-between">
                      <span>{isZh ? '快捷选择模型 (点击自动适配建议费率):' : 'Quick Select (auto-prefill rate):'}</span>
                      {candidateModels.some(c => c.isUnpriced) && (
                        <span className="text-amber-600 dark:text-amber-400 text-[10px] flex items-center gap-1">
                          <span className="w-1.5 h-1.5 rounded-full bg-amber-500 inline-block animate-pulse" />
                          {isZh ? '含未配费率模型' : 'Unpriced available'}
                        </span>
                      )}
                    </div>
                    <div className="flex flex-wrap gap-1.5 max-h-32 overflow-y-auto pr-0.5">
                      {candidateModels.map(({ model: m, isUnpriced }) => (
                        <button
                          key={m}
                          type="button"
                          onClick={() => handleSelectModelCandidate(m)}
                          className={`px-2 py-1 rounded-lg text-xs font-mono border transition cursor-pointer flex items-center space-x-1.5 ${
                            formData.model === m
                              ? 'bg-indigo-50 dark:bg-indigo-950/60 text-indigo-700 dark:text-indigo-300 border-indigo-300 dark:border-indigo-700 font-semibold shadow-xs'
                              : isUnpriced
                              ? 'bg-amber-50/70 dark:bg-amber-950/40 text-amber-800 dark:text-amber-300 border-amber-200 dark:border-amber-800/80 hover:border-amber-400'
                              : 'bg-slate-50 dark:bg-slate-900 text-slate-600 dark:text-slate-300 border-slate-200 dark:border-slate-700 hover:border-indigo-300'
                          }`}
                          title={isUnpriced ? (isZh ? '当前组尚未配置该模型专属费率' : 'Unpriced in this group') : (isZh ? '点击自动填入并预设费率' : 'Click to select and prefill rate')}
                        >
                          <span>{m}</span>
                          {isUnpriced && (
                            <span className="w-1.5 h-1.5 rounded-full bg-amber-500 inline-block shrink-0" title={isZh ? '待配置专属费率' : 'Unpriced'} />
                          )}
                        </button>
                      ))}
                    </div>
                  </div>
                )}
              </div>

              {/* Field: User Group Tier */}
              <div>
                <label className="block text-slate-700 dark:text-slate-300 font-semibold mb-1">
                  {isZh ? '定价范围与分组 (Scope)' : 'Pricing Scope'} <span className="text-rose-500">*</span>
                </label>
                <div className="ui-meta mb-1.5 text-slate-500 dark:text-slate-400">
                  {isZh
                    ? '推荐保留为 [默认官方基准 (default)]。所有业务分组（如 VIP 8折、企业组 1.5倍）自动按倍率继承此单价；仅当此模型在某分组需特殊单独议价覆盖时才选择特定组。'
                    : 'Recommended: Default (default). All groups automatically inherit via their multiplier.'}
                </div>
                {editingPrice ? (
                  <div className="flex items-center space-x-2 py-1">
                    <span className={`px-2.5 py-1 rounded-lg text-xs font-semibold border ${getGroupBadge(formData.group_name).className}`}>
                      {getGroupBadge(formData.group_name).label} ({formData.group_name})
                    </span>
                  </div>
                ) : (
                  <div className="space-y-2">
                    <div className="flex flex-wrap items-center gap-1.5">
                      {availableGroups.map(g => (
                        <button
                          key={g}
                          type="button"
                          onClick={() => setFormData({ ...formData, group_name: g })}
                          className={`px-3 py-1.5 rounded-lg text-xs font-medium border transition cursor-pointer ${
                            formData.group_name === g
                              ? 'bg-slate-900 dark:bg-indigo-600 text-white border-slate-900 dark:border-indigo-600 font-semibold shadow-sm'
                              : 'bg-white dark:bg-slate-800 hover:bg-slate-50 dark:hover:bg-slate-700 border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-300'
                          }`}
                        >
                          {g === 'default' 
                            ? (isZh ? '默认组 (default)' : 'Default (default)') 
                            : g === 'vip' 
                              ? (isZh ? 'VIP组 (vip)' : 'VIP (vip)') 
                              : g === 'enterprise' 
                                ? (isZh ? '企业组 (enterprise)' : 'Enterprise (enterprise)') 
                                : `${g} ${isZh ? '组' : 'Tier'}`}
                        </button>
                      ))}
                    </div>
                    <input
                      type="text"
                      placeholder={isZh ? '或输入自定义用户组标识 (如 partner / test 等)' : 'Or input custom group identifier (e.g. partner / test)'}
                      value={formData.group_name}
                      onChange={(e) => setFormData({ ...formData, group_name: e.target.value.toLowerCase().replace(/[^a-z0-9_-]/g, '') })}
                      className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded-xl px-3 py-2 font-mono text-slate-900 dark:text-slate-100 dark:placeholder-slate-500 focus:outline-none focus:border-indigo-500 text-xs"
                    />
                  </div>
                )}
              </div>

              {/* Field 2: Base Rates */}
              <div className="p-4 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-200/80 dark:border-slate-800 space-y-3">
                <span className="font-bold text-slate-800 dark:text-slate-200 block text-xs">
                  {isZh ? '基准单价 (每 1,000,000 Tokens)' : 'Base Price (per 1,000,000 Tokens)'}
                </span>
                <div className="grid grid-cols-3 gap-3">
                  <div>
                    <label className="block text-slate-600 dark:text-slate-300 font-medium mb-1">
                      {isZh ? '输入单价 (¥)' : 'Prompt Price (¥)'}
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
                      className="w-full bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded-xl px-2.5 py-1.5 font-mono text-slate-900 dark:text-slate-100 focus:outline-none focus:border-indigo-500 text-xs"
                    />
                  </div>

                  <div>
                    <label className="block text-slate-600 dark:text-slate-300 font-medium mb-1">
                      {isZh ? '输出单价 (¥)' : 'Completion Price (¥)'}
                    </label>
                    <input
                      type="number"
                      step="0.01"
                      min="0"
                      required
                      value={formData.completion_price}
                      onChange={(e) => setFormData({ ...formData, completion_price: e.target.value })}
                      className="w-full bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded-xl px-2.5 py-1.5 font-mono text-slate-900 dark:text-slate-100 focus:outline-none focus:border-indigo-500 text-xs"
                    />
                  </div>

                  <div>
                    <label className="block text-emerald-700 dark:text-emerald-400 font-medium mb-1">
                      {isZh ? '缓存命中 (¥)' : 'Cache Hit (¥)'}
                    </label>
                    <input
                      type="number"
                      step="0.01"
                      min="0"
                      value={formData.cache_read_price}
                      onChange={(e) => setFormData({ ...formData, cache_read_price: e.target.value })}
                      className="w-full bg-white dark:bg-slate-900 border border-emerald-300 dark:border-emerald-800 rounded-xl px-2.5 py-1.5 font-mono text-slate-900 dark:text-slate-100 focus:outline-none focus:border-emerald-500 text-xs"
                    />
                  </div>
                </div>
              </div>

              {/* Field 3: Time-of-Use Multi-Slot Flexible Configuration */}
              <div className="p-4 rounded-2xl bg-indigo-50/40 dark:bg-indigo-950/20 border border-indigo-100 dark:border-indigo-900 space-y-4">
                <div className="flex items-center justify-between">
                  <div className="flex items-center space-x-2">
                    <Moon className="w-4 h-4 text-indigo-600 dark:text-indigo-400" />
                    <span className="font-bold text-slate-900 dark:text-slate-100">
                      {isZh ? '开启分时优惠' : 'Enable Off-Peak Discount'}
                    </span>
                  </div>
                  <label className="relative inline-flex items-center cursor-pointer">
                    <input
                      type="checkbox"
                      checked={formData.off_peak_enabled}
                      onChange={(e) => setFormData({ ...formData, off_peak_enabled: e.target.checked })}
                      className="sr-only peer"
                    />
                    <div className="w-9 h-5 bg-slate-200 dark:bg-slate-700 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white dark:after:bg-slate-300 after:border-slate-300 dark:after:border-slate-500 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-indigo-600"></div>
                  </label>
                </div>

                {formData.off_peak_enabled && (
                  <div className="space-y-3 pt-1 animate-in fade-in">
                    <div className="flex items-center justify-between">
                      <span className="text-xs font-semibold text-slate-700 dark:text-slate-300">
                        {isZh ? '优惠时间段列表 (支持自由配置任意多个时段)' : 'Time Windows (Multiple custom windows supported)'}
                      </span>
                      <button
                        type="button"
                        onClick={handleAddSlot}
                        className="inline-flex items-center space-x-1 px-3 py-1 rounded-lg bg-slate-900 dark:bg-indigo-600 hover:bg-slate-800 dark:hover:bg-indigo-500 text-white text-xs font-medium shadow-sm transition cursor-pointer"
                      >
                        <Plus className="w-3.5 h-3.5" />
                        <span>{isZh ? '添加时间段' : 'Add Time Window'}</span>
                      </button>
                    </div>

                    {/* Slots List */}
                    <div className="space-y-2.5">
                      {formData.slots.map((slot, index) => (
                        <div key={slot.id || index} className="p-3 bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-800 shadow-sm space-y-2.5">
                          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2.5">
                            <div className="flex items-center space-x-2 flex-1">
                              <span className="w-5 h-5 rounded-full bg-indigo-100 dark:bg-indigo-950 text-indigo-700 dark:text-indigo-300 text-[10px] font-bold flex items-center justify-center shrink-0">
                                {index + 1}
                              </span>
                              <input
                                type="text"
                                placeholder={isZh ? '时段名称 (选填)' : 'Slot name (optional)'}
                                value={slot.name}
                                onChange={(e) => handleUpdateSlot(slot.id, 'name', e.target.value)}
                                className="w-28 px-2.5 py-1.5 bg-slate-50 dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-lg text-xs text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500"
                              />
                            </div>

                            <div className="flex items-center space-x-2">
                              <input
                                type="text"
                                placeholder="00:00"
                                value={slot.start}
                                onChange={(e) => handleUpdateSlot(slot.id, 'start', e.target.value)}
                                className="w-18 px-2 py-1.5 bg-slate-50 dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-lg font-mono text-center text-xs text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500"
                              />
                              <span className="text-slate-400 text-xs">{isZh ? '至' : 'to'}</span>
                              <input
                                type="text"
                                placeholder="08:30"
                                value={slot.end}
                                onChange={(e) => handleUpdateSlot(slot.id, 'end', e.target.value)}
                                className="w-18 px-2 py-1.5 bg-slate-50 dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-lg font-mono text-center text-xs text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500"
                              />
                            </div>

                            <div className="flex items-center space-x-2">
                              <select
                                value={slot.discount}
                                onChange={(e) => handleUpdateSlot(slot.id, 'discount', parseFloat(e.target.value))}
                                className="px-2 py-1.5 bg-slate-50 dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-lg text-xs font-mono text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500"
                              >
                                <option value="0.1">{isZh ? '1 折 (10%)' : '90% Off (10%)'}</option>
                                <option value="0.2">{isZh ? '2 折 (20%)' : '80% Off (20%)'}</option>
                                <option value="0.3">{isZh ? '3 折 (30%)' : '70% Off (30%)'}</option>
                                <option value="0.4">{isZh ? '4 折 (40%)' : '60% Off (40%)'}</option>
                                <option value="0.5">{isZh ? '5 折 (50%)' : '50% Off (50%)'}</option>
                                <option value="0.6">{isZh ? '6 折 (60%)' : '40% Off (60%)'}</option>
                                <option value="0.7">{isZh ? '7 折 (70%)' : '30% Off (70%)'}</option>
                                <option value="0.8">{isZh ? '8 折 (80%)' : '20% Off (80%)'}</option>
                                <option value="0.85">{isZh ? '8.5 折 (85%)' : '15% Off (85%)'}</option>
                                <option value="0.9">{isZh ? '9 折 (90%)' : '10% Off (90%)'}</option>
                              </select>

                              {formData.slots.length > 1 && (
                                <button
                                  type="button"
                                  onClick={() => handleRemoveSlot(slot.id)}
                                  className="p-1.5 text-slate-400 hover:text-rose-600 hover:bg-rose-50 dark:hover:bg-rose-950/40 rounded-lg transition cursor-pointer"
                                  title={isZh ? '删除该时段' : 'Delete slot'}
                                >
                                  <Trash2 className="w-3.5 h-3.5" />
                                </button>
                              )}
                            </div>
                          </div>

                          {/* Zero-burden Weekday Selector */}
                          <div className="flex items-center justify-between pt-1.5 border-t border-slate-100 dark:border-slate-800 text-xs">
                            <span className="ui-meta">{isZh ? '生效周期:' : 'Active on:'}</span>
                            <div className="flex items-center space-x-1">
                              {[
                                { label: isZh ? '一' : 'M', val: 1 },
                                { label: isZh ? '二' : 'T', val: 2 },
                                { label: isZh ? '三' : 'W', val: 3 },
                                { label: isZh ? '四' : 'T', val: 4 },
                                { label: isZh ? '五' : 'F', val: 5 },
                                { label: isZh ? '六' : 'S', val: 6 },
                                { label: isZh ? '日' : 'S', val: 7 }
                              ].map(({ label, val }) => {
                                const isActive = isDayActive(slot.days, val);
                                return (
                                  <button
                                    key={val}
                                    type="button"
                                    onClick={() => toggleSlotDay(slot.id, val)}
                                    className={`w-6 h-6 rounded-md text-[11px] font-medium transition cursor-pointer flex items-center justify-center ${
                                      isActive
                                        ? 'bg-slate-900 dark:bg-indigo-600 text-white font-semibold shadow-sm'
                                        : 'bg-slate-100 dark:bg-slate-800 text-slate-400 hover:bg-slate-200 dark:hover:bg-slate-700'
                                    }`}
                                    title={isZh ? `周${label}` : `Day ${val}`}
                                  >
                                    {label}
                                  </button>
                                );
                              })}
                              <span className="text-[11px] text-indigo-600 dark:text-indigo-400 font-medium ml-1.5">
                                {formatDaysLabel(slot.days, isZh) || (isZh ? '每天' : 'Everyday')}
                              </span>
                            </div>
                          </div>
                        </div>
                      ))}
                    </div>

                    <div className="pt-1">
                      <label className="flex items-center space-x-2 text-slate-700 dark:text-slate-300 cursor-pointer">
                        <input
                          type="checkbox"
                          checked={formData.weekend_all_day}
                          onChange={(e) => setFormData({ ...formData, weekend_all_day: e.target.checked })}
                          className="rounded-sm border-slate-300 dark:border-slate-700 text-indigo-600 focus:ring-indigo-500"
                        />
                        <span className="font-medium">{isZh ? '周六与周日全天享受优惠折扣' : 'All-day discount on Saturdays and Sundays'}</span>
                      </label>
                    </div>
                  </div>
                )}
              </div>

              {/* Actions */}
              <div className="pt-2 flex items-center justify-end space-x-2.5">
                <button
                  type="button"
                  onClick={() => setShowModal(false)}
                  className="px-4 py-2 rounded-lg bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300 hover:bg-slate-200 dark:hover:bg-slate-700 transition font-medium cursor-pointer"
                >
                  {isZh ? '取消' : 'Cancel'}
                </button>
                <button
                  type="submit"
                  disabled={saving}
                  className="px-4 py-2 rounded-lg bg-slate-900 dark:bg-indigo-600 hover:bg-slate-800 dark:hover:bg-indigo-500 text-white font-medium transition shadow-sm flex items-center space-x-1.5 disabled:opacity-50 cursor-pointer"
                >
                  {saving ? (
                    <>
                      <RefreshCw className="w-3.5 h-3.5 animate-spin" />
                      <span>{isZh ? '正在保存...' : 'Saving...'}</span>
                    </>
                  ) : (
                    <span>{isZh ? '保存定价' : 'Save Pricing'}</span>
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
