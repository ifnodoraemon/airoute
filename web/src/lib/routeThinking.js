import { THINKING_EFFORTS, selectableThinkingEfforts } from './thinking';

const ORDER = THINKING_EFFORTS;

// Read only server-resolved facts. Public aliases never supply capability facts.
export function routeThinkingAvailability(providers, channels, publicModel) {
  const bindings = providers.map((provider) => {
    const channel = channels.find((item) => item.id === provider.channel_id);
    const actual = provider.mapped_model || publicModel;
    return {
      ...provider,
      thinking: channel?.thinking_capabilities?.[actual] || provider.thinking,
      enabled: !!channel && (!channel.status || channel.status === 'active'),
    };
  });
  const enabled = bindings.filter((item) => item.enabled);
  const on = enabled.filter((item) => item.thinking?.known && item.thinking.on);
  const off = enabled.filter((item) => item.thinking?.known && item.thinking.off);
  const efforts = [...new Set(on.flatMap((item) => selectableThinkingEfforts(item.thinking.efforts)))].sort(
    (a, b) => (ORDER.includes(a) ? ORDER.indexOf(a) : ORDER.length) - (ORDER.includes(b) ? ORDER.indexOf(b) : ORDER.length) || a.localeCompare(b)
  );
  return {
    bindings,
    total: enabled.length,
    on,
    off,
    efforts,
    unknown: enabled.filter((item) => !item.thinking?.known).length,
    supporting: (effort) => effort === 'off' ? off : effort === 'on' ? on : on.filter((item) => item.thinking.efforts?.includes(effort)),
  };
}

export function routeThinkingError(policy, availability, isZh = true) {
  const mode = policy.default_mode || 'auto';
  const allowed = policy.allowed_efforts;
  const unavailable =
    (mode === 'on' && !availability.on.length) || (mode === 'off' && !availability.off.length);
  if (unavailable)
    return isZh
      ? '所选设置无可用渠道。'
      : 'No enabled channel supports the default thinking mode.';
  if (policy.allowed_modes != null && !policy.allowed_modes.includes(mode))
    return isZh
      ? '默认设置超出路由允许范围。'
      : 'The default mode must be allowed by the route.';
  const invalidMode = policy.allowed_modes?.some(
    (value) =>
      (value === 'on' && !availability.on.length) || (value === 'off' && !availability.off.length)
  );
  if (invalidMode)
    return isZh
      ? '请移除不支持的模式限制。'
      : 'The mode restriction contains an unsupported option. Clear or adjust it.';
  if (allowed != null && mode === 'auto')
    return isZh
      ? '请先选择默认档位或关闭思考。'
      : 'Choose a default thinking mode before restricting effort.';
  if (
    policy.default_effort &&
    (!availability.efforts.includes(policy.default_effort) ||
      (allowed != null && !allowed.includes(policy.default_effort)))
  )
    return isZh
      ? '默认档位不可用，请重新选择。'
      : 'Choose a supported and allowed default effort.';
  if (allowed?.some((value) => !availability.efforts.includes(value)))
    return isZh
      ? '请移除不支持的档位。'
      : 'Remove unsupported efforts from the allowed range.';
  if (mode === 'on' && allowed?.length === 0)
    return isZh
      ? '请保留至少一个档位，或取消限制。'
      : 'Allow at least one effort, or clear the restriction to use the model default.';
  return '';
}
