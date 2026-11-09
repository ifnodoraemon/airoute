import { selectableThinkingEfforts } from './thinking';

// Explicit model and wildcard declarations always take priority over discovery.
export function applyThinkingMetadata(config = {}, capabilities = {}) {
  const next = { ...config };
  for (const [name, capability] of Object.entries(capabilities)) {
    if (capability.source !== 'metadata' || !capability.known || next[name]?.thinking || next['*']?.thinking) continue;
    next[name] = { ...next[name], thinking: { can_disable: capability.off, efforts: selectableThinkingEfforts(capability.efforts) } };
  }
  return next;
}
