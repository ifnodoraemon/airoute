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

export function thinkingPolicyText(policy, isZh = true) {
  const mode = policy?.default_mode || 'auto';
  const labels = [];
  if (mode === 'off') labels.push(isZh ? '思考关闭' : 'Thinking off');
  if (mode === 'on') labels.push(isZh ? `思考 · ${effortName(policy.default_effort || '默认')}` : `Thinking · ${policy.default_effort || 'default'}`);
  if (policy?.allowed_modes != null) labels.push(isZh ? '开关受限' : 'Mode restricted');
  if (policy?.allowed_efforts != null) labels.push(isZh ? '档位受限' : 'Levels restricted');
  return labels.join(' · ');
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
