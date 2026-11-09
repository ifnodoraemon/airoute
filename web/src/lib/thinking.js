export const THINKING_EFFORTS = ['low', 'medium', 'high', 'xhigh', 'max', 'ultra'];

export function validThinkingEffort(value) {
  return /^[a-z][a-z0-9_-]{0,31}$/.test(value) && !['none', 'auto', 'on', 'off', 'adaptive', 'minimal'].includes(value);
}

export function selectableThinkingEfforts(efforts) {
  return (efforts || []).filter(validThinkingEffort);
}

export function reasoningTokens(usage) {
  const count = usage?.completion_tokens_details?.reasoning_tokens ?? usage?.reasoning_tokens ?? usage?.thoughtsTokenCount;
  return Number.isFinite(count) && count >= 0 ? count : null;
}

export function thinkingChoices(capability, tools = false) {
  return tools ? (capability?.tools || {}) : (capability || {});
}

export function thinkingSelection(value) {
  return value?.mode === 'on' && value.effort ? value.effort : value?.mode || 'auto';
}

export function thinkingFromSelection(value) {
  return ['auto', 'on', 'off'].includes(value)
    ? { mode: value, effort: '' }
    : { mode: 'on', effort: value };
}

export function thinkingOptions(capability, value) {
  const efforts = selectableThinkingEfforts(capability?.efforts);
  return ['auto', ...(capability?.off ? ['off'] : []),
    ...(capability?.on && (!efforts.length || (value?.mode === 'on' && !value.effort)) ? ['on'] : []), ...efforts];
}

export function thinkingOptionLabel(value, isZh = true) {
  const labels = isZh
    ? { auto: '默认', on: '开启', off: '关闭', native: '上游原生参数', low: '低', medium: '中', high: '高', xhigh: '很高', max: '最高', ultra: '超高（ultra）' }
    : { auto: 'Default', on: 'On', off: 'Off', native: 'Upstream parameters' };
  return labels[value] || value;
}

export function thinkingControl(capability, value) {
  const mode = value?.mode || 'auto';
  // Retain explicit requirements after a hot reload. The UI marks unavailable
  // choices and the server rejects them; no stale choice silently becomes auto.
  const control = { mode };
  if (value?.effort) control.effort = value.effort;
  if (capability?.summary) control.include_summary = value?.include_summary !== false;
  return control;
}

export function thinkingRequest(capability, value) {
  const control = thinkingControl(capability, value);
  return control.mode !== 'auto' || control.include_summary != null ? { thinking_control: control } : {};
}

export function thinkingResult(frame, previous = {}) {
  const choice = frame?.choices?.[0];
  const result = { ...previous, ...(frame?.airoute_thinking || {}) };
  const count = reasoningTokens(frame?.usage);
  if (count != null) result.reasoning_tokens = count;
  if (choice?.finish_reason) result.finish_reason = choice.finish_reason;
  return result;
}

// Only protocol history returned by the provider is replayed. UI diagnostics,
// usage and display-only reasoning are kept out of ordinary conversation text.
export function assistantHistory(message, keepReasoning = false, targetModel) {
  const result = { role: message.role, content: message.content };
  if (message.role === 'assistant') {
    if (message.extra_content && (!targetModel || !message.model || message.model===targetModel)) result.extra_content = message.extra_content;
    if (keepReasoning && (message.reasoning_content || message.reasoning)) result.reasoning_content = message.reasoning_content || message.reasoning;
  }
  return result;
}

export function minimumThinkingOutput(capability, value) {
  const mode=value?.mode || 'auto';
  if (mode!=='on' && !(mode==='auto' && capability?.policy?.default_mode==='on')) return 0;
  const effort=value?.effort || capability?.policy?.default_effort || 'default';
  return capability?.minimum_output_tokens?.[effort] || capability?.minimum_output_tokens?.default || 0;
}

export function thinkingOutputLimit(capability, value, defaultLimit) {
  const minimum=minimumThinkingOutput(capability,value);
  return minimum>0 ? Math.max(defaultLimit,minimum+1024) : defaultLimit;
}
