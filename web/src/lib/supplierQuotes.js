import { defaultRetailPrice, supplierQuoteKey, upstreamModelCatalog } from './modelPresentation';

export function quoteCurrency(quote) {
  return quote?.currency || 'CNY';
}
export function quoteMode(quote) {
  return quote?.billing_mode || 'token';
}
export function quoteMoney(value, currency = 'CNY') {
  if (value == null || value === '') return '—';
  const prefix = { CNY: '¥', USD: '$', EUR: '€', GBP: '£' }[currency] || `${currency} `;
  return `${prefix}${Number(value).toLocaleString('en-US', { minimumFractionDigits: 4, maximumFractionDigits: 9, useGrouping: false })}`;
}

export function procurementRows(channels, routes, quotes) {
  const catalog = upstreamModelCatalog(channels, routes);
  const byKey = new Map(catalog.map((row) => [row.key, row]));
  for (const quote of quotes) {
    const key = supplierQuoteKey(quote.channel, quote.model);
    byKey.set(key, {
      ...(byKey.get(key) || { key, channel: quote.channel, model: quote.model, routes: [] }),
      quote,
    });
  }
  return [...byKey.values()];
}

// Only compare a default retail rule for a public alias with this physical
// channel/model quote. Currency, billing basis and enablement must agree.
export function compareQuote(quote, prices, publicModel, reference) {
  const explicit = defaultRetailPrice(prices, publicModel);
  const retail = explicit || reference?.retail_price;
  if (!explicit && reference?.retail_price_source === 'default')
    return { retail, reason: 'unpriced' };
  if (!retail) return { retail: null, reason: 'unpriced' };
  if (!quote) return { retail, reason: 'missing' };
  if (quote.status === 'disabled') return { retail, reason: 'disabled' };
  if (quoteCurrency(quote) !== (retail.currency || 'CNY')) return { retail, reason: 'currency' };
  if (quoteMode(quote) === 'request') {
    // Retail may combine token charges and fixed charges, or bill images/seconds.
    // Only a flat per-request rule has the same basis as a request quote.
    if (
      ['image', 'audio', 'video', 'tts', 'stt'].includes(reference?.modality) ||
      Number(retail.prompt_price) !== 0 ||
      Number(retail.completion_price) !== 0 ||
      (retail.billing_unit && retail.billing_unit !== 'request')
    )
      return { retail, reason: 'basis' };
    return { retail, fixed: Number(retail.fixed_price || 0) - quote.fixed_cost };
  }
  if (Number(retail.fixed_price || 0) !== 0) return { retail, reason: 'basis' };
  return {
    retail,
    input: Number(retail.prompt_price) - quote.input_cost,
    output: Number(retail.completion_price) - quote.output_cost,
  };
}

export function openProcurement(setCurrentTab, channel = '', model = '') {
  const url = new URL(window.location.href);
  url.searchParams.set('pricing.view', 'suppliers');
  for (const [key, value] of Object.entries({ channel, model })) {
    if (value) url.searchParams.set(`pricing.${key}`, value);
    else url.searchParams.delete(`pricing.${key}`);
  }
  window.history.replaceState(window.history.state, '', url);
  setCurrentTab?.('pricing');
}
