

import { useCallback } from 'react';
import { apiJSON } from '../api/client';
import { emptyChannel } from '../lib/providerPresets';

export default function useChannelSave({
  editingChannelId, isZh, newChannel, refresh, setEditingChannelId, setNewChannel, setProbeAlert,
  setShowChannelModal, setSubmittingChannel, showToast, submittingChannel
}) {
  const handleCreateChannel = useCallback(async (e) => {
    e.preventDefault();
    if (submittingChannel) return;
    const cleanUrl = (newChannel.base_url || '').trim();
    if (!cleanUrl.startsWith('http://') && !cleanUrl.startsWith('https://')) {
      showToast(isZh ? '接口地址必须以 http:// 或 https:// 开头' : 'Base URL must start with http:// or https://', 'warning');
      return;
    }

    const modelMapping = Object.create(null);
    for (const rule of newChannel.mapping_rules || []) {
      const from = rule.from.trim();
      const to = rule.to.trim();
      if (!from || !to || Object.hasOwn(modelMapping, from)) {
        showToast(isZh ? '请补全模型映射，且来源模型不能重复' : 'Complete each mapping and use unique source models', 'warning');
        return;
      }
      modelMapping[from] = to;
    }

    const typedApiKey = (newChannel.api_key || '').trim();
    const payload = {
      ...newChannel,
      name: (newChannel.name || '').trim(),
      base_url: cleanUrl,
      models: newChannel.models_str.split(',').map(s => s.trim()).filter(Boolean),
      model_mapping: modelMapping,
    };
    delete payload.models_str;
    delete payload.mapping_rules;
    // Client-side form bookkeeping, not part of the channel schema.
    delete payload.api_key_configured;
    delete payload.detected_thinking_capabilities;
    // An edit that did not touch the credential must not send one: the service
    // keeps the stored key when `api_key` is absent/empty, and re-submitting a
    // masked placeholder would otherwise overwrite the real credential.
    if (editingChannelId && !typedApiKey) {
      delete payload.api_key;
    }

    setSubmittingChannel(true);
    try {
      if (editingChannelId) {
        const result = await apiJSON(`/api/v1/admin/channels/${editingChannelId}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        });
        showToast(result.warning || (isZh ? 'Provider 更新成功，内存已热重载' : 'Provider updated successfully, memory hot-reloaded'), result.warning ? 'warning' : 'success');
      } else {
        const result = await apiJSON('/api/v1/admin/channels', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        });
        showToast(result.warning || (isZh ? 'Provider 创建成功，内存已热重载' : 'Provider created successfully, memory hot-reloaded'), result.warning ? 'warning' : 'success');
      }
    } catch (err) {
      showToast((isZh ? '保存 Provider 失败: ' : 'Failed to save provider: ') + err.message, 'error');
      return;
    } finally {
      setSubmittingChannel(false);
    }

    setShowChannelModal(false);
    setEditingChannelId(null);
    setProbeAlert(null);
    setNewChannel(emptyChannel());
    refresh();
  }, [submittingChannel, newChannel, editingChannelId, setSubmittingChannel, setShowChannelModal, setEditingChannelId, setProbeAlert, setNewChannel, refresh, showToast, isZh]);
  return {
    handleCreateChannel
  };
}
