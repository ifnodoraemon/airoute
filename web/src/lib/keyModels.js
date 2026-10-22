// Match the gateway's exact names, global wildcard, and namespace wildcards.
export function availableKeyModels(key, models = []) {
  if (!key || (key.status && key.status !== 'active')) return [];
  const allowed = key.allowed_models || [];
  return models.filter(model => allowed.length === 0 || allowed.some(rule =>
    rule === '*' || rule === model || (rule.endsWith('/*') && model.startsWith(rule.slice(0, -1)))
  ));
}
