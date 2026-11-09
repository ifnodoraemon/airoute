import useChannelSave from './useChannelSave';
import useChannelProbe from './useChannelProbe';
import useChannelForm from './useChannelForm';
import { apiMutation } from '../api/mutation';
// The channel (provider) domain: the create/edit form, the probe, the per-row and
// batch operations, and the latency table they fill.
//
// Extracted from App.jsx, where this was roughly 470 lines of state and handlers
// interleaved with the render tree. The hook takes the few things it does not own
// — the toast, the language, the channel list, the console refresh — as options.

// The channel (provider) domain: the create/edit form, the probe, the per-row and
// batch operations, and the latency table they fill.
//
// Extracted from App.jsx, where this was roughly 470 lines of state and handlers
// interleaved with the render tree. The hook takes the few things it does not own
// — the toast, the language, the channel list, the console refresh — as options.

import { useCallback } from 'react';
import { apiFetch, apiJSON } from '../api/client';
import { buildPresetChannel, emptyChannel } from '../lib/providerPresets';

export function useChannelActions({ channels = [], showToast, isZh, refresh }) {
  const {
    batchTesting, cancelProbe, channelLatencies, editingChannelId, newChannel, probeAlert, probeRef, probing,
    selectedChannelIds, setBatchTesting, setChannelDraft, setChannelLatencies, setEditingChannelId,
    setModalVisibility, setNewChannel, setProbeAlert, setProbing, setSelectedChannelIds, setShowChannelModal,
    setSubmittingChannel, setTestingId, showChannelModal, submittingChannel, testingId
  } = useChannelForm({

  });

  // Automatic Zero-Choice Debounced Probe
  const {
    handleProbeChannel
  } = useChannelProbe({
    cancelProbe, editingChannelId, isZh, newChannel, probeRef, setChannelDraft, setProbeAlert, setProbing,
    showToast
  });

  // Quick Presets for Provider (Never inject fake models!)
  const applyPreset = useCallback((presetKey) => {
    setNewChannel(prev => {
      const built = buildPresetChannel(presetKey, prev);
      if (built) {
        showToast(isZh ? built.messageZh : built.messageEn, 'info');
        return built.channel;
      }
      return prev;
    });
  }, [isZh, showToast, setNewChannel]);

  // Toggle protocol checkbox
  const toggleProtocol = useCallback((proto) => {
    setNewChannel(prev => {
      const exists = prev.protocols.includes(proto);
      if (exists) {
        return { ...prev, protocols: prev.protocols.filter(p => p !== proto) };
      }
      return { ...prev, protocols: [...prev.protocols, proto] };
    });
  }, [setNewChannel]);

  const handleBatchPing = useCallback(async () => {
    if (channels.length === 0) return;
    setBatchTesting(true);
    showToast(isZh ? '正在并发检测所有渠道连通性...' : 'Testing all channels in parallel...', 'info');
    const results = {};
    await Promise.all(channels.map(async (ch) => {
      try {
        const res = await apiFetch(`/api/v1/admin/channels/${ch.id}/test`, { method: 'POST' });
        const data = await res.json();
        results[ch.id] = {
          latency_ms: data.latency_ms || 0,
          success: data.code === 0,
          error: data.error || (data.code === 0 ? '' : (isZh ? '连接失败' : 'Connection failed'))
        };
      } catch (e) {
        results[ch.id] = { latency_ms: 0, success: false, error: e.message };
      }
    }));
    setChannelLatencies(results);
    setBatchTesting(false);
    const successCount = Object.values(results).filter(r => r.success).length;
    showToast(
      isZh
        ? `体检完成: ${successCount}/${channels.length} 个渠道响应正常`
        : `Health check complete: ${successCount}/${channels.length} channels responsive`,
      successCount === channels.length ? 'success' : 'warning'
    );
  }, [channels, isZh, setBatchTesting, setChannelLatencies, showToast]);

  const handleBatchTest = handleBatchPing;

  // Handle Channel/Provider Creation & Editing
  const {
    handleCreateChannel
  } = useChannelSave({
    editingChannelId, isZh, newChannel, refresh, setEditingChannelId, setNewChannel, setProbeAlert,
    setShowChannelModal, setSubmittingChannel, showToast, submittingChannel
  });

  const openCreateChannel = useCallback(() => {
    setEditingChannelId(null);
    setNewChannel({
      name: '', type: 'gpustack', base_url: '', api_key: '', priority: 1, weight: 10,
      timeout_seconds: 60, max_concurrency: 0, queue_limit: 0, queue_wait_seconds: 0,
      models_str: '', mapping_rules: [],
      protocols: ['openai_chat', 'openai_response', 'embeddings', 'rerank', 'images'],
    });
    setProbeAlert(null);
    setShowChannelModal(true);
  }, [setEditingChannelId, setNewChannel, setProbeAlert, setShowChannelModal]);

  // Handle Channel Editing
  const handleEditChannel = useCallback((ch) => {
    setEditingChannelId(ch.id);
    setNewChannel({
      name: ch.name || '',
      type: ch.type || 'openai',
      base_url: ch.base_url || '',
      // The list endpoint no longer returns the upstream credential at all: the
      // response carries only `api_key_configured` and a display mask. Keep
      // `api_key` empty so an unchanged field cannot round-trip a secret; the
      // update handler preserves the stored key when the request omits it.
      api_key: '',
      api_key_configured: Boolean(ch.api_key_configured),
      // `??` rather than `||`: a stored 0 is a value, and turning it into 1/10
      // here would silently disagree with what the gateway persisted.
      priority: ch.priority ?? 1,
      weight: ch.weight ?? 10,
      timeout_seconds: ch.timeout_seconds ?? 60,
      max_concurrency: ch.max_concurrency ?? 0,
      queue_limit: ch.queue_limit ?? 0,
      queue_wait_seconds: ch.queue_wait_seconds ?? 0,
      model_capabilities: ch.model_capabilities || {},
      models_str: (ch.models || []).join(', '),
      mapping_rules: Object.entries(ch.model_mapping || {}).map(([from, to]) => ({ from, to })),
      protocols: ch.protocols || emptyChannel().protocols,
    });
    setProbeAlert(null);
    setShowChannelModal(true);
  }, [setEditingChannelId, setNewChannel, setProbeAlert, setShowChannelModal]);

  // Handle Channel Deletion
  const handleDeleteChannel = useCallback(async (id) => {
    if (!window.confirm(isZh ? '确认注销该模型提供商 (Provider)？' : 'Are you sure you want to remove this Provider?')) return;
    try {
      const result = await apiJSON(`/api/v1/admin/channels/${id}`, { method: 'DELETE' });
      showToast(result.warning || (isZh ? 'Provider 已注销，内存已更新' : 'Provider removed, memory updated'), result.warning ? 'warning' : 'info');
      refresh();
    } catch (err) {
      showToast((isZh ? '注销 Provider 失败: ' : 'Failed to remove provider: ') + err.message, 'error');
    }
  }, [isZh, showToast, refresh]);

  // Test Channel
  const handleTestChannel = useCallback(async (ch) => {
    setTestingId(ch.id);
    try {
      const res = await apiFetch(`/api/v1/admin/channels/${ch.id}/test`, { method: 'POST' });
      const data = await res.json();
      if (data.code === 0) {
        setChannelLatencies(prev => ({
          ...prev,
          [ch.id]: { latency_ms: data.latency_ms, success: true, error: '' }
        }));
        showToast(isZh ? `✅ [${ch.name}] 连通测试成功！耗时: ${data.latency_ms} ms` : `✅ [${ch.name}] Test successful! Latency: ${data.latency_ms} ms`, 'success');
      } else {
        setChannelLatencies(prev => ({
          ...prev,
          [ch.id]: { latency_ms: 0, success: false, error: data.error || (isZh ? '连通失败' : 'Failed') }
        }));
        showToast(isZh ? `❌ [${ch.name}] 测试失败: ${data.error || '无法连通'}` : `❌ [${ch.name}] Test failed: ${data.error || 'Unreachable'}`, 'error');
      }
    } catch (e) {
      showToast((isZh ? '请求异常: ' : 'Request error: ') + e.message, 'error');
    } finally {
      setTestingId(null);
    }
  }, [isZh, setChannelLatencies, setTestingId, showToast]);

  const handleBatchDeleteChannels = useCallback(async () => {
    if (selectedChannelIds.length === 0) return;
    if (!window.confirm(isZh ? `确认批量删除选中的 ${selectedChannelIds.length} 个服务商渠道？` : `Confirm delete selected ${selectedChannelIds.length} channels?`)) return;
    try {
      const { data, ok: requestSucceeded } = await apiMutation('/api/v1/admin/channels/batch-delete', 'POST', { ids: selectedChannelIds }, apiFetch);

      if (requestSucceeded) {
        showToast(data.warning || data.message || (isZh ? '已批量删除服务商渠道' : 'Selected channels deleted'), data.warning ? 'warning' : 'success');
        setSelectedChannelIds([]);
        refresh();
      } else {
        showToast(data.error || (isZh ? '批量删除失败' : 'Failed to delete channels'), 'error');
      }
    } catch (e) {
      showToast((isZh ? '请求异常: ' : 'Request error: ') + e.message, 'error');
    }
  }, [selectedChannelIds, isZh, showToast, setSelectedChannelIds, refresh]);

  // `overrideIds` lets a single row act on itself (ChannelsView passes [ch.id]);
  // without it the batch handler only looked at the checkbox selection, so a
  // single-row toggle either did nothing or changed a different, unrelated set of
  // channels.
  const handleBatchStatusChannels = useCallback(async (status, overrideIds) => {
    const targetIds = Array.isArray(overrideIds) && overrideIds.length > 0
      ? overrideIds
      : selectedChannelIds;
    if (targetIds.length === 0) return;
    try {
      const { data, ok: requestSucceeded } = await apiMutation('/api/v1/admin/channels/batch-status', 'POST', { ids: targetIds, status }, apiFetch);

      if (requestSucceeded) {
        showToast(data.warning || data.message || (isZh ? '批量更新状态成功' : 'Batch update status successful'), data.warning ? 'warning' : 'success');
        // Only the checkbox-driven flow owns the selection.
        if (!Array.isArray(overrideIds)) setSelectedChannelIds([]);
        refresh();
      } else {
        showToast(data.error || (isZh ? '更新失败' : 'Update failed'), 'error');
      }
    } catch (e) {
      showToast((isZh ? '请求异常: ' : 'Request error: ') + e.message, 'error');
    }
  }, [selectedChannelIds, showToast, isZh, setSelectedChannelIds, refresh]);

  // Clear the tenant-scoped parts on logout: the selection and the measured
  // latencies belong to the session that measured them.
  const reset = useCallback(() => {
    setShowChannelModal(false);
    setEditingChannelId(null);
    setProbeAlert(null);
    setNewChannel(emptyChannel());
    setSelectedChannelIds([]);
    setChannelLatencies({});
    setTestingId(null);
    setBatchTesting(false);
  }, [setBatchTesting, setChannelLatencies, setEditingChannelId, setNewChannel, setProbeAlert, setSelectedChannelIds, setShowChannelModal, setTestingId]);

  return {
    // state
    showChannelModal,
    editingChannelId,
    newChannel,
    submittingChannel,
    probing,
    probeAlert,
    testingId,
    batchTesting,
    channelLatencies,
    selectedChannelIds,
    // setters the form binds to
    setNewChannel,
    setShowChannelModal: setModalVisibility,
    setEditingChannelId,
    setSelectedChannelIds,
    setProbeAlert,
    // actions
    openCreateChannel,
    handleProbeChannel,
    applyPreset,
    toggleProtocol,
    handleBatchPing,
    handleBatchTest,
    handleCreateChannel,
    handleEditChannel,
    handleDeleteChannel,
    handleTestChannel,
    handleBatchDeleteChannels,
    handleBatchStatusChannels,
    reset,
  };
}
