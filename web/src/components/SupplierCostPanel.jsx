import { useCallback, useEffect, useMemo, useState } from 'react';
import { Download, Plus, RefreshCw, Trash2, Pencil } from 'lucide-react';
import SupplierCSVImport from './SupplierCSVImport';

function money(value) {
  return `¥${Number(value || 0).toLocaleString('en-US', { minimumFractionDigits: 4, maximumFractionDigits: 9, useGrouping: false })}`;
}

// A spread is our selling price minus the supplier price. Negative means we sell
// below what the channel charges us, so it is the one value that must stand out.
function spreadClass(value) {
  return value < 0 ? 'text-rose-600 dark:text-rose-400 font-semibold' : '';
}

const EMPTY = { channel: '', model: '', input_cost: '', output_cost: '', status: 'active' };

// SupplierCostPanel manages the contracted per-million-token supplier prices and
// shows them next to our own selling prices. This is a pure price-list
// comparison: our price (loaded from /api/v1/admin/pricing) and the supplier
// price are stored and edited separately, so the panel no longer writes any cost
// field onto usage rows — it no longer derives cost_basis_units, it only pairs
// the two price lists and shows the spread (ours - supplier) per model.
export default function SupplierCostPanel({ adminFetch, showToast, lang = 'zh', prices = [] }) {
  const isZh = lang === 'zh';
  const [rows, setRows] = useState([]);
  const [form, setForm] = useState(EMPTY);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);

  const notify = useCallback((message, type = 'success') => {
    if (showToast) showToast(message, type);
  }, [showToast]);

  // Our selling prices indexed by model. A model may carry several group tiers;
  // the default tier is the reference price, and only when it is absent does the
  // first row for that model stand in for it.
  const ourPrices = useMemo(() => {
    const byModel = new Map();
    for (const price of prices || []) {
      if (!price || !price.model) continue;
      const current = byModel.get(price.model);
      const isDefault = (price.group_name || 'default') === 'default';
      if (!current || (isDefault && (current.group_name || 'default') !== 'default')) {
        byModel.set(price.model, price);
      }
    }
    return byModel;
  }, [prices]);

  const load = useCallback(async () => {
    if (!adminFetch) return;
    setLoading(true);
    try {
      const res = await adminFetch('/api/v1/admin/supplier-costs');
      const body = await res.json().catch(() => ({}));
      if (!res.ok || body.code !== 0) throw new Error(body.error || `HTTP ${res.status}`);
      setRows(Array.isArray(body.data) ? body.data : []);
    } catch (err) {
      notify(err.message, 'error');
    } finally {
      setLoading(false);
    }
  }, [adminFetch, notify]);

  useEffect(() => { load(); }, [load]);

  const save = async (event) => {
    event.preventDefault();
    setBusy(true);
    try {
      const res = await adminFetch('/api/v1/admin/supplier-costs', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          channel: form.channel.trim(),
          model: form.model.trim(),
          input_cost: Number(form.input_cost),
          output_cost: Number(form.output_cost),
          status: form.status,
        }),
      });
      const body = await res.json().catch(() => ({}));
      if (!res.ok || body.code !== 0) throw new Error(body.error || `HTTP ${res.status}`);
      notify(isZh ? '供应商单价已保存' : 'Supplier price saved');
      setForm(EMPTY);
      await load();
    } catch (err) {
      notify(err.message, 'error');
    } finally {
      setBusy(false);
    }
  };

  const remove = async (row) => {
    setBusy(true);
    try {
      const res = await adminFetch(`/api/v1/admin/supplier-costs/${row.id}`, { method: 'DELETE' });
      const body = await res.json().catch(() => ({}));
      if (!res.ok || body.code !== 0) throw new Error(body.error || `HTTP ${res.status}`);
      notify(isZh ? '供应商单价已删除' : 'Supplier price deleted');
      await load();
    } catch (err) {
      notify(err.message, 'error');
    } finally {
      setBusy(false);
    }
  };

  const exportCSV = async () => {
    try {
      const res = await adminFetch('/api/v1/admin/supplier-costs/export');
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new Error(body.error || `HTTP ${res.status}`);
      }
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = 'supplier-costs.csv';
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(url);
    } catch (err) {
      notify(err.message, 'error');
    }
  };

  const comparisonRows = useMemo(() => {
    const configured = new Set(rows.map(row => row.model));
    const missing = [...ourPrices.keys()].filter(model => !configured.has(model) && !model.includes('*'));
    return [...rows, ...missing.map(model => ({ id: `missing:${model}`, channel: '—', model, unconfigured: true }))];
  }, [rows, ourPrices]);

  const inputClass = 'w-full px-2.5 py-1.5 rounded-lg border border-[#E4E6E9] dark:border-[#2C3038] bg-white dark:bg-[#191B1F] text-xs text-[#202124] dark:text-[#F1F2F4]';

  return (
    <div className="rounded-xl border border-[#E4E6E9] dark:border-[#2C3038] bg-white dark:bg-[#191B1F] p-3 space-y-3">
      <div className="flex items-center justify-between">
        <div>
          <div className="text-xs font-semibold text-[#202124] dark:text-[#F1F2F4]">{isZh ? '供应商单价' : 'Supplier prices'}</div>
          <div className="text-[10px] text-[#656B75] dark:text-[#A1A7B0]">
            {isZh
              ? '单位：CNY / 百万 tokens；按通道名 + 模型匹配，并与我们的售价分开存储，此处仅作并排对照。'
              : 'CNY per 1M tokens, keyed by channel name + model. Stored separately from our selling prices; the table only compares the two.'}
          </div>
        </div>
        <div className="flex items-center space-x-1.5">
          <SupplierCSVImport adminFetch={adminFetch} onImported={load} showToast={showToast} lang={lang} />
          <button type="button" onClick={exportCSV} className="inline-flex items-center space-x-1 px-2 py-1.5 rounded-lg border border-[#E4E6E9] dark:border-[#2C3038] text-[11px] text-[#656B75] dark:text-[#A1A7B0]">
            <Download className="w-3.5 h-3.5" />
            <span>{isZh ? '导出 CSV' : 'Export CSV'}</span>
          </button>
          <button type="button" onClick={load} disabled={loading} className="p-1.5 rounded-lg border border-[#E4E6E9] dark:border-[#2C3038] text-[#656B75]">
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
          </button>
        </div>
      </div>

      <form onSubmit={save} className="grid grid-cols-2 md:grid-cols-6 gap-2">
        <input value={form.channel} onChange={(e) => setForm((prev) => ({ ...prev, channel: e.target.value }))} className={inputClass} placeholder={isZh ? '通道名' : 'Channel'} aria-label={isZh ? '通道名' : 'Channel'} />
        <input value={form.model} onChange={(e) => setForm((prev) => ({ ...prev, model: e.target.value }))} className={inputClass} placeholder={isZh ? '模型' : 'Model'} aria-label={isZh ? '模型' : 'Model'} />
        <input type="number" min="0" step="any" value={form.input_cost} onChange={(e) => setForm((prev) => ({ ...prev, input_cost: e.target.value }))} className={inputClass} placeholder={isZh ? '供应商输入价' : 'Supplier input price'} aria-label={isZh ? '供应商输入价' : 'Supplier input price'} />
        <input type="number" min="0" step="any" value={form.output_cost} onChange={(e) => setForm((prev) => ({ ...prev, output_cost: e.target.value }))} className={inputClass} placeholder={isZh ? '供应商输出价' : 'Supplier output price'} aria-label={isZh ? '供应商输出价' : 'Supplier output price'} />
        <select value={form.status} onChange={e => setForm({ ...form, status: e.target.value })} className={inputClass} aria-label={isZh ? '单价状态' : 'Price status'}>
          <option value="active">{isZh ? '启用' : 'Active'}</option><option value="disabled">{isZh ? '停用' : 'Disabled'}</option>
        </select>
        <button type="submit" disabled={busy || !form.channel.trim() || !form.model.trim() || form.input_cost === '' || form.output_cost === ''} className="inline-flex items-center justify-center space-x-1.5 px-2.5 py-1.5 rounded-lg bg-[#355CFF] hover:bg-[#2A4AE0] text-white text-xs font-medium disabled:opacity-50">
          <Plus className="w-3.5 h-3.5" />
          <span>{isZh ? '保存' : 'Save'}</span>
        </button>
      </form>

      <div className="overflow-x-auto">
        <table className="w-full text-xs">
          <thead>
            <tr className="text-left text-[10px] uppercase tracking-wide text-[#656B75] dark:text-[#A1A7B0] border-b border-[#E4E6E9] dark:border-[#2C3038]">
              <th className="py-2 pr-3">{isZh ? '通道' : 'Channel'}</th>
              <th className="py-2 pr-3">{isZh ? '模型' : 'Model'}</th>
              <th className="py-2 pr-3 text-right">{isZh ? '我们售价 (输入/输出)' : 'Our price (in/out)'}</th>
              <th className="py-2 pr-3 text-right">{isZh ? '供应商单价 (输入/输出)' : 'Supplier price (in/out)'}</th>
              <th className="py-2 pr-3 text-right">{isZh ? '价差 (我们-供应商)' : 'Spread (ours - supplier)'}</th>
              <th className="py-2 text-right">{isZh ? '操作' : ''}</th>
            </tr>
          </thead>
          <tbody>
            {comparisonRows.map((row) => {
              const ours = ourPrices.get(row.model) || null;
              const ourInput = ours ? Number(ours.prompt_price || 0) : null;
              const ourOutput = ours ? Number(ours.completion_price || 0) : null;
              const supplierInput = Number(row.input_cost || 0);
              const supplierOutput = Number(row.output_cost || 0);
              const inputSpread = ours && !row.unconfigured ? ourInput - supplierInput : null;
              const outputSpread = ours && !row.unconfigured ? ourOutput - supplierOutput : null;

              return (
                <tr key={row.id} className="border-b border-[#F0F1F2] dark:border-[#22252B]">
                  <td className="py-2 pr-3">{row.channel}</td>
                  <td className="py-2 pr-3 font-mono text-[11px]">{row.model}</td>
                  <td className="py-2 pr-3 text-right font-mono">
                    {ours ? (
                      <div className="space-y-0.5">
                        <div>{money(ourInput)}</div>
                        <div>{money(ourOutput)}</div>
                      </div>
                    ) : (
                      <span className="text-[#656B75] dark:text-[#A1A7B0]">{isZh ? '未定价' : 'Unpriced'}</span>
                    )}
                  </td>
                  <td className="py-2 pr-3 text-right font-mono">
                    {row.unconfigured ? <span>{isZh ? '未配置' : 'Not configured'}</span> : <div className="space-y-0.5">
                      <div>{money(supplierInput)}</div><div>{money(supplierOutput)}</div>
                      {row.status === 'disabled' && <span>{isZh ? '已停用' : 'Disabled'}</span>}
                    </div>}
                  </td>
                  <td className="py-2 pr-3 text-right font-mono">
                    {ours && !row.unconfigured ? (
                      <div className="space-y-0.5">
                        <div className={spreadClass(inputSpread)}>{money(inputSpread)}</div>
                        <div className={spreadClass(outputSpread)}>{money(outputSpread)}</div>
                      </div>
                    ) : (
                      <span className="text-[#656B75] dark:text-[#A1A7B0]">—</span>
                    )}
                  </td>
                  <td className="py-2 text-right">
                    {!row.unconfigured && <><button type="button" onClick={() => setForm({ channel: row.channel, model: row.model, input_cost: String(row.input_cost), output_cost: String(row.output_cost), status: row.status || 'active' })} title={isZh ? '编辑' : 'Edit'} className="p-1"><Pencil className="w-3.5 h-3.5" /></button>
                    <button type="button" onClick={() => remove(row)} disabled={busy} className="p-1 text-[#656B75] hover:text-red-500" title={isZh ? '删除' : 'Delete'}>
                      <Trash2 className="w-3.5 h-3.5" />
                    </button></>}
                  </td>
                </tr>
              );
            })}
            {comparisonRows.length === 0 && (
              <tr><td colSpan={6} className="py-6 text-center text-[11px] text-[#656B75] dark:text-[#A1A7B0]">{isZh ? '尚未配置供应商单价' : 'No supplier prices yet'}</td></tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
