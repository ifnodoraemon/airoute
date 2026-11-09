import { applyThinkingMetadata } from './channelThinking';

export function applyChannelProbeResult(response, data, { setChannelDraft, setProbeAlert, showToast, isZh }) {
  const probeData = data && data.data;
  const unreachable = Boolean(probeData) && probeData.reachable === false;
  if (response.ok && data.code === 0 && probeData && !unreachable) {
    setChannelDraft(previous => channelDraftFromProbe(previous, probeData));
    notifyProbeSuccess(probeData, setProbeAlert, showToast, isZh);
  } else {
    notifyProbeFailure(data, probeData, unreachable, setProbeAlert, showToast, isZh);
  }
}

function channelDraftFromProbe(prev, d) {
  const shouldUpdateName = !prev.name || prev.name.includes('upstream') || prev.name.includes('cluster') || prev.name.includes('instance') || prev.name.includes('official') || prev.name.includes('direct');
  const shouldUpdateBaseUrl = d.suggested_base_url && (!prev.base_url.includes('/v1-openai') && !prev.base_url.includes('/v1'));
  return {
    ...prev,
    base_url: shouldUpdateBaseUrl ? d.suggested_base_url : prev.base_url,
    type: d.type || prev.type,
    name: shouldUpdateName ? (d.suggested_name || prev.name) : prev.name,
    models_str: (d.models && d.models.length > 0) ? d.models.join(', ') : prev.models_str,
    protocols: (d.protocols && d.protocols.length > 0) ? d.protocols : prev.protocols,
    detected_thinking_capabilities: d.thinking_capabilities || {},
    model_capabilities: applyThinkingMetadata(prev.model_capabilities, d.thinking_capabilities),
  };
}

function notifyProbeSuccess(d, setProbeAlert, showToast, isZh) {
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
}

function notifyProbeFailure(data, probeData, unreachable, setProbeAlert, showToast, isZh) {
  // No auto-fill on failure: the discovered models/protocols came from a
  // probe that never established a connection.
  const failureText = data.error
    || (probeData && probeData.message)
    || (unreachable
      ? (isZh ? '上游服务不可达：所有探测路径均在传输层失败' : 'Upstream unreachable: every probe path failed at the transport level')
      : (isZh ? '无法连通指定上游服务，请检查网络连通性或服务端口' : 'Cannot connect to upstream endpoint, please check network or port'));
  setProbeAlert({
    type: 'error',
    text: `❌ ${isZh ? '探测失败: ' : 'Probe failed: '}${failureText}`,
  });
  showToast(
    unreachable
      ? (isZh ? '探测未连通上游服务，协议与模型未自动匹配' : 'Probe did not reach the upstream; protocols and models were not auto-filled')
      : (isZh ? '连通测试失败，请检查服务地址' : 'Connection test failed. Please verify endpoint URL'),
    'error'
  );
}
