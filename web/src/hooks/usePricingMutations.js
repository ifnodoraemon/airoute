import { apiMutation } from '../api/mutation';

export default function usePricingMutations({
  adminFetch, checkSlotsOverlap, fetchPrices, formData, isZh, selectedKeys, setFormError, setSaving,
  setSelectedKeys, setShowModal, showToast
}) {
  const handleDelete = async (model, groupName = 'default') => {
    if (!window.confirm(isZh ? `确定删除模型 [${model}] 在 [${groupName}] 分组的定价规则？` : `Are you sure you want to delete pricing rule for model [${model}] in group [${groupName}]?`)) {
      return;
    }
    try {
      const query = new URLSearchParams({ model, group: groupName || 'default' });
      const { data, ok: requestSucceeded } = await apiMutation(`/api/v1/admin/pricing?${query}`, 'DELETE', undefined, adminFetch);

      if (requestSucceeded) {
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
      const { data, ok: requestSucceeded } = await apiMutation('/api/v1/admin/pricing/batch-delete', 'POST', { items }, adminFetch);

      if (requestSucceeded) {
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

      const { data, ok: requestSucceeded } = await apiMutation('/api/v1/admin/pricing', 'POST', payload, adminFetch);

      if (requestSucceeded) {
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
  return {
    handleBatchDelete, handleDelete, handleSave
  };
}
