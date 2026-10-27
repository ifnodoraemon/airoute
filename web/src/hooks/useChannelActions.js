// The channel (provider) domain: the create/edit form, the probe, the per-row and
// batch operations, and the latency table they fill.
//
// Extracted from App.jsx, where this was roughly 470 lines of state and handlers
// interleaved with the render tree. The hook takes the few things it does not own
// — the toast, the language, the channel list, the console refresh — as options.

import { useCallback, useState, useRef, useEffect } from 'react';
import { apiFetch, apiJSON } from '../api/client';
import { buildPresetChannel, emptyChannel } from '../lib/providerPresets';

export function useChannelActions({ channels = [], showToast, isZh, refresh }) {
  const [showChannelModal, setShowChannelModal] = useState(false);
  const [editingChannelId, setEditingChannelId] = useState(null);
  const [newChannel, setChannelDraft] = useState(() => emptyChannel());
  const [submittingChannel, setSubmittingChannel] = useState(false);
  const [probing, setProbing] = useState(false);
  const [probeAlert, setProbeAlert] = useState(null);
  const [testingId, setTestingId] = useState(null);
  const [batchTesting, setBatchTesting] = useState(false);
  const [channelLatencies, setChannelLatencies] = useState({});
  const [selectedChannelIds, setSelectedChannelIds] = useState([]);

  const probeRef = useRef(null);
  const cancelProbe = useCallback(() => {
    probeRef.current?.abort();
    probeRef.current = null;
    setProbing(false);
  }, []);
  const setNewChannel = useCallback((update) => {
    cancelProbe();
    setChannelDraft(update);
  }, [cancelProbe]);
  useEffect(() => () => probeRef.current?.abort(), []);
  const setModalVisibility = useCallback((visible) => {
    cancelProbe();
    setShowChannelModal(visible);
  }, [cancelProbe]);

  // Automatic Zero-Choice Debounced Probe
  const triggerProbe = useCallback(async (url, apiKey, currentType) => {
    if (!url || !url.trim()) {
      showToast(isZh ? '请先输入下游服务的 Base URL' : 'Please enter the downstream Base URL first', 'warning');
      return;
    }
    cancelProbe();
    const controller = new AbortController();
    probeRef.current = controller;
    setProbing(true);
    setProbeAlert(null);
    try {
      const res = await apiFetch('/api/v1/admin/channels/probe', {
        signal: controller.signal,
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          base_url: url.trim(),
          api_key: apiKey || '',
          type: currentType || 'openai',
        }),
      });
      const data = await res.json();
      if (controller.signal.aborted || probeRef.current !== controller) return;
      if (data.code === 0 && data.data) {
        const d = data.data;
        setChannelDraft(prev => {
          const shouldUpdateName = !prev.name || prev.name.includes('upstream') || prev.name.includes('cluster') || prev.name.includes('instance') || prev.name.includes('official') || prev.name.includes('direct');
          const shouldUpdateBaseUrl = d.suggested_base_url && (!prev.base_url.includes('/v1-openai') && !prev.base_url.includes('/v1'));
          return {
            ...prev,
            base_url: shouldUpdateBaseUrl ? d.suggested_base_url : prev.base_url,
            type: d.type || prev.type,
            name: shouldUpdateName ? (d.suggested_name || prev.name) : prev.name,
            models_str: (d.models && d.models.length > 0) ? d.models.join(', ') : prev.models_str,
            protocols: (d.protocols && d.protocols.length > 0) ? d.protocols : prev.protocols,
          };
        });
        const isAuthWarning = d.message && (d.message.includes('401') || d.message.includes('保护') || d.message.includes('凭证') || d.message.includes('鉴权'));
        setProbeAlert({
          type: isAuthWarning ? 'warning' : 'success',
          text: isAuthWarning
            ? `⚠️ ${d.message}`
            : (isZh
                ? `✨ 连通测试成功 (耗时: ${d.latency_ms}ms)！已自动匹配 [${d.type}] 引擎，获取到 ${d.models?.length || 0} 个在线模型并勾选对应协议。`
                : `✨ Probe passed (${d.latency_ms}ms)! Auto-matched [${d.type}] engine, discovered ${d.models?.length || 0} models and checked protocols.`),
        });
        showToast(
          isAuthWarning
            ? (isZh ? '服务已连通，但下游开启了凭证鉴权 (HTTP 401)，请输入 API Key 提取在线模型' : 'Connected, but upstream requires authentication (HTTP 401). Please enter API Key.')
            : (isZh ? `连通测试成功！已同步 ${d.models?.length || 0} 个模型` : `Connection passed! Synced ${d.models?.length || 0} models`),
          isAuthWarning ? 'warning' : 'success'
        );
      } else {
        setProbeAlert({
          type: 'error',
          text: `❌ ${isZh ? '探测失败: ' : 'Probe failed: '}${data.error || (isZh ? '无法连通指定上游服务，请检查网络连通性或服务端口' : 'Cannot connect to upstream endpoint, please check network or port')}`,
        });
        showToast(isZh ? '连通测试失败，请检查服务地址' : 'Connection test failed. Please verify endpoint URL', 'error');
      }
    } catch (e) {
      if (controller.signal.aborted || probeRef.current !== controller) return;
      setProbeAlert({
        type: 'error',
        text: `${isZh ? '探测网络异常: ' : 'Network probe error: '}${e.message}`,
      });
      showToast(isZh ? `网络请求异常: ${e.message}` : `Network request error: ${e.message}`, 'error');
    } finally {
      if (probeRef.current === controller) { probeRef.current = null; setProbing(false); }
    }
  }, [isZh, showToast, cancelProbe]);

  const handleProbeChannel = useCallback(() => {
    triggerProbe(newChannel.base_url, newChannel.api_key, newChannel.type);
  }, [triggerProbe, newChannel.base_url, newChannel.api_key, newChannel.type]);

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
  }, [channels, isZh, showToast]);

  const handleBatchTest = handleBatchPing;

  // Handle Channel/Provider Creation & Editing
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
  }, [submittingChannel, newChannel, editingChannelId, isZh, showToast, refresh, setNewChannel]);

  // Handle Channel Editing
  const handleEditChannel = useCallback((ch) => {
    setEditingChannelId(ch.id);
    setNewChannel({
      name: ch.name || '',
      type: ch.type || 'openai',
      base_url: ch.base_url || '',
      // The list endpoint returns the upstream credential in cleartext. Keep it out
      // of the form (and therefore out of the DOM and the console devtools tree):
      // an unchanged field must not round-trip the secret. `api_key_configured`
      // tells the form to say "already set — leave blank to keep it"; the update
      // handler preserves the stored key when the request omits it.
      api_key: '',
      api_key_configured: Boolean(ch.api_key),
      priority: ch.priority || 1,
      weight: ch.weight || 10,
      timeout_seconds: ch.timeout_seconds || 60,
      models_str: (ch.models || []).join(', '),
      mapping_rules: Object.entries(ch.model_mapping || {}).map(([from, to]) => ({ from, to })),
      protocols: ch.protocols || emptyChannel().protocols,
    });
    setProbeAlert(null);
    setShowChannelModal(true);
  }, [setNewChannel]);

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
  }, [isZh, showToast]);

  const handleBatchDeleteChannels = useCallback(async () => {
    if (selectedChannelIds.length === 0) return;
    if (!window.confirm(isZh ? `确认批量删除选中的 ${selectedChannelIds.length} 个服务商渠道？` : `Confirm delete selected ${selectedChannelIds.length} channels?`)) return;
    try {
      const res = await apiFetch('/api/v1/admin/channels/batch-delete', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids: selectedChannelIds })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(data.warning || data.message || (isZh ? '已批量删除服务商渠道' : 'Selected channels deleted'), data.warning ? 'warning' : 'success');
        setSelectedChannelIds([]);
        refresh();
      } else {
        showToast(data.error || (isZh ? '批量删除失败' : 'Failed to delete channels'), 'error');
      }
    } catch (e) {
      showToast((isZh ? '请求异常: ' : 'Request error: ') + e.message, 'error');
    }
  }, [selectedChannelIds, isZh, showToast, refresh]);

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
      const res = await apiFetch('/api/v1/admin/channels/batch-status', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids: targetIds, status })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
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
  }, [selectedChannelIds, isZh, showToast, refresh]);

  const openCreateChannel = useCallback(() => {
    setEditingChannelId(null);
    setProbeAlert(null);
    setNewChannel(emptyChannel());
    setShowChannelModal(true);
  }, [setNewChannel]);

  const closeChannelModal = useCallback(() => {
    cancelProbe();
    setShowChannelModal(false);
  }, [cancelProbe]);

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
  }, [setNewChannel]);

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
    openCreateChannel,
    closeChannelModal,
    reset,
  };
}
