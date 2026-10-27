import { useCallback, useEffect, useState } from 'react';
import { Download, Plus, RefreshCw, Trash2, Upload } from 'lucide-react';

function money(value) {
  return `¥${Number(value || 0).toFixed(4)}`;
}

const EMPTY = { channel: '', model: '', input_cost: '', output_cost: '' };

// SupplierCostPanel manages the contracted per-million-token prices. These rows
// are the only source of cost_basis_units on new usage rows; an unknown pair
// stores NULL rather than zero.
export default function SupplierCostPanel({ adminFetch, showToast, lang = 'zh' }) {
  const isZh = lang === 'zh';
  const [rows, setRows] = useState([]);
  const [form, setForm] = useState(EMPTY);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [importing, setImporting] = useState(false);

  const notify = useCallback((message, type = 'success') => {
    if (showToast) showToast(message, type);
  }, [showToast]);

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
        }),
      });
      const body = await res.json().catch(() => ({}));
      if (!res.ok || body.code !== 0) throw new Error(body.error || `HTTP ${res.status}`);
      notify(isZh ? '价目已保存' : 'Price saved');
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
      notify(isZh ? '价目已删除' : 'Price deleted');
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

  const importCSV = async (event) => {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (!file) return;
    setImporting(true);
    try {
      const text = await file.text();
      const res = await adminFetch('/api/v1/admin/supplier-costs/import', {
        method: 'POST',
        headers: { 'Content-Type': 'text/csv' },
        body: text,
      });
      const body = await res.json().catch(() => ({}));
      if (!res.ok || body.code !== 0) throw new Error(body.error || `HTTP ${res.status}`);
      notify(isZh ? `已导入 ${body.data?.imported ?? 0} 行` : `Imported ${body.data?.imported ?? 0} rows`);
      await load();
    } catch (err) {
      notify(err.message, 'error');
    } finally {
      setImporting(false);
    }
  };

  const inputClass = 'w-full px-2.5 py-1.5 rounded-lg border border-[#E4E6E9] dark:border-[#2C3038] bg-white dark:bg-[#191B1F] text-xs text-[#202124] dark:text-[#F1F2F4]';

  return (
    <div className="rounded-xl border border-[#E4E6E9] dark:border-[#2C3038] bg-white dark:bg-[#191B1F] p-3 space-y-3">
      <div className="flex items-center justify-between">
        <div>
          <div className="text-xs font-semibold text-[#202124] dark:text-[#F1F2F4]">{isZh ? '供应商价目（成本）' : 'Supplier prices (cost)'}</div>
          <div className="text-[10px] text-[#656B75] dark:text-[#A1A7B0]">{isZh ? '单位：CNY / 百万 tokens；匹配通道名 + 模型。' : 'CNY per 1M tokens, keyed by channel name + model.'}</div>
        </div>
        <div className="flex items-center space-x-1.5">
          <label className="inline-flex items-center space-x-1 px-2 py-1.5 rounded-lg border border-[#E4E6E9] dark:border-[#2C3038] text-[11px] text-[#656B75] dark:text-[#A1A7B0] cursor-pointer">
            <Upload className="w-3.5 h-3.5" />
            <span>{isZh ? '导入 CSV' : 'Import CSV'}</span>
            <input type="file" accept=".csv,text/csv" onChange={importCSV} disabled={importing} className="hidden" aria-label={isZh ? '导入 CSV' : 'Import CSV'} />
          </label>
          <button type="button" onClick={exportCSV} className="inline-flex items-center space-x-1 px-2 py-1.5 rounded-lg border border-[#E4E6E9] dark:border-[#2C3038] text-[11px] text-[#656B75] dark:text-[#A1A7B0]">
            <Download className="w-3.5 h-3.5" />
            <span>{isZh ? '导出 CSV' : 'Export CSV'}</span>
          </button>
          <button type="button" onClick={load} disabled={loading} className="p-1.5 rounded-lg border border-[#E4E6E9] dark:border-[#2C3038] text-[#656B75]">
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
          </button>
        </div>
      </div>

      <form onSubmit={save} className="grid grid-cols-2 md:grid-cols-5 gap-2">
        <input value={form.channel} onChange={(e) => setForm((prev) => ({ ...prev, channel: e.target.value }))} className={inputClass} placeholder={isZh ? '通道名' : 'Channel'} aria-label={isZh ? '通道名' : 'Channel'} />
        <input value={form.model} onChange={(e) => setForm((prev) => ({ ...prev, model: e.target.value }))} className={inputClass} placeholder={isZh ? '模型' : 'Model'} aria-label={isZh ? '模型' : 'Model'} />
        <input type="number" min="0" step="0.01" value={form.input_cost} onChange={(e) => setForm((prev) => ({ ...prev, input_cost: e.target.value }))} className={inputClass} placeholder={isZh ? '输入成本' : 'Input cost'} aria-label={isZh ? '输入成本' : 'Input cost'} />
        <input type="number" min="0" step="0.01" value={form.output_cost} onChange={(e) => setForm((prev) => ({ ...prev, output_cost: e.target.value }))} className={inputClass} placeholder={isZh ? '输出成本' : 'Output cost'} aria-label={isZh ? '输出成本' : 'Output cost'} />
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
              <th className="py-2 pr-3 text-right">{isZh ? '输入/百万' : 'Input/1M'}</th>
              <th className="py-2 pr-3 text-right">{isZh ? '输出/百万' : 'Output/1M'}</th>
              <th className="py-2 pr-3">{isZh ? '状态' : 'Status'}</th>
              <th className="py-2 text-right">{isZh ? '操作' : ''}</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.id} className="border-b border-[#F0F1F2] dark:border-[#22252B]">
                <td className="py-2 pr-3">{row.channel}</td>
                <td className="py-2 pr-3 font-mono text-[11px]">{row.model}</td>
                <td className="py-2 pr-3 text-right font-mono">{money(row.input_cost)}</td>
                <td className="py-2 pr-3 text-right font-mono">{money(row.output_cost)}</td>
                <td className="py-2 pr-3">{row.status}</td>
                <td className="py-2 text-right">
                  <button type="button" onClick={() => remove(row)} disabled={busy} className="p-1 text-[#656B75] hover:text-red-500" title={isZh ? '删除' : 'Delete'}>
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                </td>
              </tr>
            ))}
            {rows.length === 0 && (
              <tr><td colSpan={6} className="py-6 text-center text-[11px] text-[#656B75] dark:text-[#A1A7B0]">{isZh ? '尚未配置供应商成本' : 'No supplier prices yet'}</td></tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
