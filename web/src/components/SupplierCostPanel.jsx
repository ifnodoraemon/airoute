import { useCallback, useEffect, useMemo, useState } from 'react';
import { Download, Plus, RefreshCw, Trash2 } from 'lucide-react';
import SupplierCSVImport from './SupplierCSVImport';
import SupplierQuoteForm from './SupplierQuoteForm';
import SupplierQuoteSummary from './SupplierQuoteSummary';
import { compareQuote, procurementRows, quoteCurrency, quoteMoney } from '../lib/supplierQuotes';

export default function SupplierCostPanel({
  adminFetch,
  showToast,
  lang = 'zh',
  prices = [],
  channels = [],
  modelRoutes = [],
  focusChannel = '',
  focusModel = '',
  onClearTarget,
}) {
  const isZh = lang === 'zh';
  const [quotes, setQuotes] = useState([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState('');
  const [busy, setBusy] = useState(false);
  const [editing, setEditing] = useState(null);
  const [search, setSearch] = useState('');
  const load = useCallback(async () => {
    setLoading(true);
    setLoadError('');
    try {
      const res = await adminFetch('/api/v1/admin/supplier-costs');
      const body = await res.json();
      if (!res.ok || body.code !== 0) throw new Error(body.error || `HTTP ${res.status}`);
      setQuotes(Array.isArray(body.data) ? body.data : []);
    } catch (err) {
      setLoadError(err.message);
      showToast?.(err.message, 'error');
    } finally {
      setLoading(false);
    }
  }, [adminFetch, showToast]);
  useEffect(() => {
    load();
  }, [load]);
  const catalog = useMemo(
    () => procurementRows(channels, modelRoutes, quotes),
    [channels, modelRoutes, quotes]
  );
  const visible = catalog.filter(
    (row) =>
      (!focusChannel || row.channel === focusChannel) &&
      `${row.channel} ${row.model} ${row.routes.map((route) => route.model).join(' ')}`
        .toLowerCase()
        .includes(search.toLowerCase())
  );
  const selected =
    editing ||
    (focusModel
      ? quotes.find((row) => row.channel === focusChannel && row.model === focusModel) || {
          channel: focusChannel,
          model: focusModel,
        }
      : null);
  const clear = () => {
    setEditing(null);
    onClearTarget?.();
  };
  const remove = async (row) => {
    if (
      !window.confirm(
        isZh
          ? `删除 ${row.channel} / ${row.model} 的采购报价？`
          : `Delete purchase quote for ${row.channel} / ${row.model}?`
      )
    )
      return;
    setBusy(true);
    try {
      const res = await adminFetch(`/api/v1/admin/supplier-costs/${row.quote.id}`, {
        method: 'DELETE',
      });
      const body = await res.json();
      if (!res.ok || body.code !== 0) throw new Error(body.error || `HTTP ${res.status}`);
      showToast?.(isZh ? '采购报价已删除' : 'Purchase quote deleted', 'success');
      clear();
      await load();
    } catch (err) {
      showToast?.(err.message, 'error');
    } finally {
      setBusy(false);
    }
  };
  const exportCSV = async () => {
    try {
      const res = await adminFetch('/api/v1/admin/supplier-costs/export');
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const url = URL.createObjectURL(await res.blob());
      const link = document.createElement('a');
      link.href = url;
      link.download = 'supplier-costs.csv';
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(url);
    } catch (err) {
      showToast?.(err.message, 'error');
    }
  };
  const comparison = (quote, model) =>
    compareQuote(
      quote,
      prices,
      model,
      modelRoutes.find((route) => route.model === model)
    );
  const spread = (quote, model) => {
    const result = comparison(quote, model);
    const reasons = {
      unpriced: isZh ? '未配置默认售价' : 'No default retail rate',
      missing: isZh ? '未报价' : 'Not quoted',
      disabled: isZh ? '报价已停用' : 'Quote disabled',
      currency: isZh ? '币种不同，不换算' : 'Different currencies; no conversion',
      basis: isZh ? '计费口径不同' : 'Different billing basis',
    };
    if (result.reason) return <span className="text-slate-500">{reasons[result.reason]}</span>;
    return (result.fixed != null ? [result.fixed] : [result.input, result.output]).map(
      (value, index) => (
        <span
          key={index}
          className={`block font-mono ${value < 0 ? 'text-rose-600 dark:text-rose-400 font-semibold' : ''}`}
        >
          {quoteMoney(value, quoteCurrency(quote))}
        </span>
      )
    );
  };
  return (
    <section className="ui-card p-4 space-y-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold">
            {isZh ? '渠道采购报价' : 'Channel purchase quotes'}
          </h2>
          <p className="mt-1 text-xs text-slate-500">
            {isZh
              ? `按渠道 + 实际上游模型登记。${catalog.filter((row) => row.quote).length} 项已报价，${catalog.filter((row) => !row.quote).length} 项未报价。`
              : `Quotes by channel + actual upstream model. ${catalog.filter((row) => row.quote).length} configured; ${catalog.filter((row) => !row.quote).length} missing.`}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <SupplierCSVImport
            adminFetch={adminFetch}
            onImported={load}
            showToast={showToast}
            lang={lang}
          />
          <button className="ui-btn-secondary" onClick={exportCSV}>
            <Download className="w-3.5 h-3.5" />
            {isZh ? '导出 CSV' : 'Export CSV'}
          </button>
          <button
            className="ui-btn-secondary"
            onClick={load}
            disabled={loading}
            aria-label={isZh ? '刷新采购报价' : 'Refresh purchase quotes'}
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
          </button>
          <button className="ui-btn-primary" onClick={() => setEditing({ channel: focusChannel })}>
            <Plus className="w-3.5 h-3.5" />
            {isZh ? '新增采购报价' : 'Add purchase quote'}
          </button>
        </div>
      </div>
      <p className="text-xs text-slate-500 leading-relaxed">
        {isZh
          ? '采购报价是供应商合同的参考单价；对外售价按公开路由配置。下方仅比较同币种、同口径的默认售价与采购价，未计入分组、时段、缓存等折扣，价差不代表实际利润。'
          : 'Purchase quotes are contracted reference rates. Retail rates belong to public routes. Spreads compare default rates with the same currency and billing basis, before group, schedule or cache discounts; they are not actual profit.'}
      </p>
      {selected && !loading && !loadError && (
        <SupplierQuoteForm
          key={JSON.stringify([selected.channel, selected.model, selected.id])}
          initial={selected}
          channels={channels}
          catalog={catalog}
          adminFetch={adminFetch}
          showToast={showToast}
          lang={lang}
          onCancel={clear}
          onSaved={async () => {
            clear();
            await load();
          }}
        />
      )}
      <div className="flex flex-wrap items-center gap-2">
        <input
          className="ui-field sm:max-w-sm"
          aria-label={isZh ? '搜索采购报价' : 'Search purchase quotes'}
          placeholder={
            isZh ? '搜索渠道、上游模型或公开路由' : 'Search channel, upstream model or route'
          }
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        {focusChannel && (
          <button className="ui-btn-secondary" onClick={clear}>
            {focusChannel} · {isZh ? '清除筛选' : 'Clear filter'}
          </button>
        )}
      </div>
      {loadError ? (
        <p role="alert" className="text-sm text-rose-600">
          {loadError}
        </p>
      ) : loading ? (
        <p className="text-sm text-slate-500">
          {isZh ? '正在加载采购报价…' : 'Loading purchase quotes…'}
        </p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-xs text-left">
            <thead>
              <tr className="border-b dark:border-slate-700 text-slate-500">
                <th className="ui-th">
                  {isZh ? '渠道 / 实际上游模型' : 'Channel / actual upstream model'}
                </th>
                <th className="ui-th">{isZh ? '采购报价' : 'Purchase quote'}</th>
                <th className="ui-th">
                  {isZh ? '公开路由 / 对外默认售价' : 'Public route / default retail price'}
                </th>
                <th className="ui-th">
                  {isZh ? '基准价差（售 − 采）' : 'Baseline spread (retail − purchase)'}
                </th>
                <th className="ui-th text-right">{isZh ? '操作' : 'Actions'}</th>
              </tr>
            </thead>
            <tbody>
              {visible.map((row) => (
                <tr key={row.key} className="border-b dark:border-slate-800 align-top">
                  <td className="ui-td">
                    <p className="font-medium">{row.channel}</p>
                    <p className="mt-1 font-mono break-all">{row.model}</p>
                  </td>
                  <td className="ui-td">
                    <SupplierQuoteSummary quote={row.quote} lang={lang} />
                  </td>
                  <td className="ui-td space-y-3">
                    {row.routes.length ? (
                      row.routes.map((binding) => {
                        const retail = comparison(row.quote, binding.model).retail;
                        return (
                          <div key={binding.model}>
                            <p className="font-mono">{binding.model}</p>
                            {binding.retail_price_source === 'wildcard' && (
                              <p className="text-slate-500">
                                {isZh ? '继承规则：' : 'Inherited rule: '}
                                {binding.retail_price?.model}
                              </p>
                            )}
                            {binding.retail_price_source === 'default' && (
                              <p className="text-amber-700">
                                {isZh
                                  ? '系统兜底价，尚未配置售价'
                                  : 'System fallback; retail rate unconfigured'}
                              </p>
                            )}
                            {retail ? (
                              <>
                                <p className="text-slate-500">
                                  {retail.currency || 'CNY'} ·{' '}
                                  {isZh
                                    ? '输入 / 输出，每百万 Token'
                                    : 'Input / output per 1M tokens'}
                                </p>
                                <p>
                                  {quoteMoney(retail.prompt_price, retail.currency)} /{' '}
                                  {quoteMoney(retail.completion_price, retail.currency)}
                                </p>
                                {Number(retail.fixed_price) > 0 && (
                                  <p>
                                    {isZh ? '另收固定价：' : 'Additional fixed rate: '}
                                    {quoteMoney(retail.fixed_price, retail.currency)}
                                  </p>
                                )}
                              </>
                            ) : (
                              <p className="text-slate-500">
                                {isZh ? '未配置默认售价' : 'No default retail rate'}
                              </p>
                            )}
                          </div>
                        );
                      })
                    ) : (
                      <span className="text-slate-500">
                        {isZh ? '尚未绑定公开路由' : 'No public route binding'}
                      </span>
                    )}
                  </td>
                  <td className="ui-td space-y-3">
                    {row.routes.length
                      ? row.routes.map((binding) => (
                          <div key={binding.model}>
                            <p className="font-mono mb-1">{binding.model}</p>
                            {spread(row.quote, binding.model)}
                          </div>
                        ))
                      : '—'}
                  </td>
                  <td className="ui-td text-right">
                    <button
                      className="ui-btn-secondary ui-btn-sm"
                      onClick={() =>
                        setEditing(row.quote || { channel: row.channel, model: row.model })
                      }
                    >
                      {row.quote
                        ? isZh
                          ? '编辑报价'
                          : 'Edit quote'
                        : isZh
                          ? '补充报价'
                          : 'Add quote'}
                    </button>
                    {row.quote && (
                      <button
                        className="p-2 text-slate-500 hover:text-rose-600"
                        aria-label={`${isZh ? '删除报价' : 'Delete quote'} ${row.channel} ${row.model}`}
                        disabled={busy}
                        onClick={() => remove(row)}
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    )}
                  </td>
                </tr>
              ))}
              {visible.length === 0 && (
                <tr>
                  <td colSpan={5} className="py-8 text-center text-slate-500">
                    {isZh
                      ? '没有匹配的渠道模型；可新增合同报价，或先配置渠道和路由。'
                      : 'No matching channel models. Add a quote, or configure channels and routes.'}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      )}
      <p className="text-xs text-slate-400">
        {isZh
          ? 'CSV 支持 currency、billing_mode（token / request）、cache_read_cost、cache_write_cost、fixed_cost；兼容原有人民币 Token 报价文件。'
          : 'CSV supports currency, billing_mode (token / request), cache_read_cost, cache_write_cost and fixed_cost. Legacy CNY token files remain supported.'}
      </p>
    </section>
  );
}
