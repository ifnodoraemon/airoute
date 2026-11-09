import { apiMutation } from '../api/mutation';

export default function usePricingGroups({
  adminFetch, isZh, localGroupMultipliers, newGroupInputKey, newGroupInputMultiplier,
  onUpdateGroupMultipliers, setLocalGroupMultipliers, setNewGroupInputKey, setNewGroupInputMultiplier,
  setSavingGroups, setShowAddGroupModal, showToast
}) {
  const handleSaveGroupMultipliers = async (newMultipliers) => {
    const payload = newMultipliers || localGroupMultipliers;
    setSavingGroups(true);
    try {
      const { data, ok: requestSucceeded } = await apiMutation('/api/v1/admin/pricing/groups', 'POST', payload, adminFetch);

      if (requestSucceeded) {
        if (data.data) {
          setLocalGroupMultipliers(data.data);
        }
        showToast(isZh ? '客户分组已保存并全网实时生效' : 'Group discount tiers saved and active in real-time', 'success');
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
  return {
    handleCreateNewGroupTier, handleDeleteGroupTier, handleSaveGroupMultipliers, handleUpdateSingleMultiplier
  };
}
