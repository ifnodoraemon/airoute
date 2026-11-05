const EFFORT_NAMES = {
  minimal: '极少',
  low: '低',
  medium: '中',
  high: '高',
  xhigh: '很高',
  max: '最高',
};

export function effortName(value, isZh = true) {
  return isZh ? EFFORT_NAMES[value] || value : value;
}

export function thinkingCapabilityText(capability, isZh = true) {
  if (!capability?.known) return isZh ? '思考能力待确认' : 'Thinking capability unconfirmed';
  if (capability.on && capability.off) return isZh ? '支持开启 / 关闭' : 'Can enable / disable';
  if (capability.on) return isZh ? '支持思考，不能关闭' : 'Thinking supported; cannot disable';
  if (capability.off) return isZh ? '不支持思考模式' : 'No thinking mode';
  return isZh ? '开关能力待确认' : 'Toggle support unconfirmed';
}

export function thinkingPolicyText(policy, isZh = true) {
  const mode = policy?.default_mode || 'auto';
  let text =
    mode === 'auto'
      ? isZh
        ? '默认由上游决定'
        : 'Default follows upstream'
      : mode === 'off'
        ? isZh
          ? '默认关闭思考'
          : 'Thinking off by default'
        : isZh
          ? `默认开启${policy.default_effort ? ` · ${effortName(policy.default_effort)}强度` : ' · 使用模型默认强度'}`
          : `Thinking on · ${policy.default_effort || 'model default effort'}`;
  if (policy?.allowed_modes != null) {
    const labels = {
      auto: isZh ? '上游默认' : 'upstream default',
      on: isZh ? '开启' : 'on',
      off: isZh ? '关闭' : 'off',
    };
    text +=
      (isZh ? '；允许：' : '; allowed: ') +
      (policy.allowed_modes.map((value) => labels[value] || value).join(' / ') ||
        (isZh ? '无' : 'none'));
  }
  if (policy?.allowed_efforts != null)
    text +=
      (isZh ? '；强度范围：' : '; effort range: ') +
      (policy.allowed_efforts.map((value) => effortName(value, isZh)).join(' / ') ||
        (isZh ? '仅模型默认' : 'model default only'));
  return text;
}

export function supplierQuoteKey(channel, model) {
  return JSON.stringify([channel || '', model || '']);
}

export function defaultRetailPrice(prices, model) {
  return (
    (prices || []).find(
      (row) => row.model === model && (!row.group_name || row.group_name === 'default')
    ) || null
  );
}

// Public routes and physical models are different identities. Join procurement
// quotes by the provider's actual mapped model, then expose every public alias.
export function upstreamModelCatalog(channels = [], routes = []) {
  const entries = new Map();
  const add = (channel, model, binding) => {
    if (!channel || !model || model.includes('*')) return;
    const key = supplierQuoteKey(channel.name, model);
    if (!entries.has(key))
      entries.set(key, { key, channel: channel.name, channel_id: channel.id, model, routes: [] });
    const entry = entries.get(key);
    if (binding && !entry.routes.some((row) => row.model === binding.model))
      entry.routes.push(binding);
  };
  for (const channel of channels) {
    const actualModels = channel.upstream_models || [
      ...(channel.models || []).map((model) => channel.model_mapping?.[model] || model),
      ...Object.values(channel.model_mapping || {}),
      ...Object.keys(channel.model_capabilities || {}),
    ];
    for (const model of actualModels) add(channel, model);
  }
  for (const route of routes) {
    for (const provider of route.providers || []) {
      const channel = channels.find((row) => row.id === provider.channel_id) || {
        id: provider.channel_id,
        name: provider.channel_name,
      };
      add(channel, provider.mapped_model || route.model, {
        model: route.model,
        modality: route.modality,
        thinking: provider.thinking,
        retail_price: route.retail_price,
        retail_price_source: route.retail_price_source,
      });
    }
  }
  return [...entries.values()];
}
