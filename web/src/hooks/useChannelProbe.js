

import { useCallback } from 'react';
import { apiFetch } from '../api/client';
import { applyChannelProbeResult } from '../lib/channelProbeResult';

export default function useChannelProbe({
  cancelProbe, editingChannelId, isZh, newChannel, probeRef, setChannelDraft, setProbeAlert, setProbing,
  showToast
}) {
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
          channel_id: editingChannelId || undefined,
        }),
      });
      const data = await res.json();
      if (controller.signal.aborted || probeRef.current !== controller) return;
      applyChannelProbeResult(res, data, { setChannelDraft, setProbeAlert, showToast, isZh });
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
  }, [cancelProbe, probeRef, setProbing, setProbeAlert, showToast, isZh, editingChannelId, setChannelDraft]);

  const handleProbeChannel = useCallback(() => {
    triggerProbe(newChannel.base_url, newChannel.api_key, newChannel.type);
  }, [triggerProbe, newChannel.base_url, newChannel.api_key, newChannel.type]);
  return {
    handleProbeChannel
  };
}
