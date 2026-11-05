const ORDER = ['minimal', 'low', 'medium', 'high', 'xhigh', 'max'];

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
  const efforts = [...new Set(on.flatMap((item) => item.thinking.efforts || []))].sort(
    (a, b) => ORDER.indexOf(a) - ORDER.indexOf(b)
  );
  return {
    bindings,
    total: enabled.length,
    on,
    off,
    efforts,
    unknown: enabled.filter((item) => !item.thinking?.known).length,
    supporting: (effort) => on.filter((item) => item.thinking.efforts?.includes(effort)),
  };
}

export function routeThinkingError(policy, availability, isZh = true) {
  const mode = policy.default_mode || 'auto';
  const allowed = policy.allowed_efforts;
  const unavailable =
    (mode === 'on' && !availability.on.length) || (mode === 'off' && !availability.off.length);
  if (unavailable)
    return isZh
      ? '当前没有已启用渠道支持所选默认思考模式。'
      : 'No enabled channel supports the default thinking mode.';
  if (policy.allowed_modes != null && !policy.allowed_modes.includes(mode))
    return isZh
      ? '默认模式必须在路由允许的范围内。'
      : 'The default mode must be allowed by the route.';
  const invalidMode = policy.allowed_modes?.some(
    (value) =>
      (value === 'on' && !availability.on.length) || (value === 'off' && !availability.off.length)
  );
  if (invalidMode)
    return isZh
      ? '模式限制包含当前上游不支持的选项，请取消或调整模式限制。'
      : 'The mode restriction contains an unsupported option. Clear or adjust it.';
  if (allowed != null && mode === 'auto')
    return isZh
      ? '请先选择默认思考模式，再限制强度范围。'
      : 'Choose a default thinking mode before restricting effort.';
  if (
    policy.default_effort &&
    (!availability.efforts.includes(policy.default_effort) ||
      (allowed != null && !allowed.includes(policy.default_effort)))
  )
    return isZh
      ? '默认强度当前不可用，请选择上游支持且路由允许的档位。'
      : 'Choose a supported and allowed default effort.';
  if (allowed?.some((value) => !availability.efforts.includes(value)))
    return isZh
      ? '允许范围中有当前上游不支持的档位，请取消这些选项。'
      : 'Remove unsupported efforts from the allowed range.';
  if (mode === 'on' && allowed?.length === 0)
    return isZh
      ? '开启思考需要至少一个允许的档位，或取消强度限制以使用模型默认。'
      : 'Allow at least one effort, or clear the restriction to use the model default.';
  return '';
}
