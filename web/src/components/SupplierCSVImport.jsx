import { useState } from 'react';
import { Upload } from 'lucide-react';
import useModalA11y from '../hooks/useModalA11y';
import { extractErrorMessage } from '../api/client';

export default function SupplierCSVImport({ adminFetch, onImported, showToast, lang = 'zh' }) {
  const isZh = lang === 'zh';
  const [preview, setPreview] = useState(null);
  const [busy, setBusy] = useState(false);
  const close = () => { if (!busy) setPreview(null); };
  useModalA11y(!!preview, close);

  const upload = async event => {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (!file || busy) return;
    setBusy(true);
    try {
      const text = await file.text();
      const res = await adminFetch('/api/v1/admin/supplier-costs/import?dry_run=1', {
        method: 'POST', headers: { 'Content-Type': 'text/csv' }, body: text,
      });
      const body = await res.json();
      const errors = body.data?.errors || [];
      if ((!res.ok || body.code !== 0) && !errors.length) throw new Error(extractErrorMessage(body, `HTTP ${res.status}`));
      setPreview({ text, errors, items: body.data?.items || [], valid: res.ok && body.code === 0 && body.data?.valid > 0 });
    } catch (err) {
      showToast?.(err.message, 'error');
    } finally {
      setBusy(false);
    }
  };

  const commit = async () => {
    if (!preview?.valid || busy) return;
    setBusy(true);
    try {
      const res = await adminFetch('/api/v1/admin/supplier-costs/import', {
        method: 'POST', headers: { 'Content-Type': 'text/csv' }, body: preview.text,
      });
      const body = await res.json();
      if (!res.ok || body.code !== 0) throw new Error(extractErrorMessage(body, `HTTP ${res.status}`));
      showToast?.(isZh ? `已导入 ${body.data?.imported ?? 0} 行` : `Imported ${body.data?.imported ?? 0} rows`, 'success');
      setPreview(null);
      await onImported?.();
    } catch (err) {
      showToast?.(err.message, 'error');
    } finally {
      setBusy(false);
    }
  };

  return <>
    <label className="ui-btn-secondary cursor-pointer">
      <Upload className="w-3.5 h-3.5" />
      <span>{busy ? (isZh ? '处理中...' : 'Processing...') : (isZh ? '导入 CSV' : 'Import CSV')}</span>
      <input type="file" accept=".csv,text/csv" onChange={upload} disabled={busy} className="hidden" aria-label={isZh ? '导入 CSV' : 'Import CSV'} />
    </label>
    {preview && <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/60 p-4">
      <section role="dialog" aria-modal="true" aria-labelledby="supplier-import-title" className="ui-card bg-white dark:bg-[#191B1F] w-full max-w-2xl p-5 space-y-4">
        <h3 id="supplier-import-title" className="font-semibold">{isZh ? '供应商单价导入预览' : 'Supplier price import preview'}</h3>
        {preview.errors.length ? <div role="alert" className="max-h-72 overflow-auto text-xs text-rose-600 space-y-1">
          {preview.errors.map((err, index) => <p key={index}>{isZh ? `第 ${err.row} 行：${err.error}` : `Row ${err.row}: ${err.error}`}</p>)}
        </div> : <>
          <p className="text-xs">{isZh ? `共 ${preview.items.length} 行，通过校验后确认写入；相同渠道和模型将覆盖原价。` : `${preview.items.length} valid rows. Confirm to import; matching channel/model prices will be overwritten.`}</p>
          <div className="max-h-72 overflow-auto text-xs">
            {preview.items.slice(0, 20).map((item, index) => <p key={index} className="py-1 font-mono">{item.channel} / {item.model} · {item.input_cost} / {item.output_cost} · {item.status}</p>)}
            {preview.items.length > 20 && <p>{isZh ? '预览仅展示前 20 行' : 'Preview shows the first 20 rows'}</p>}
          </div>
        </>}
        <div className="flex justify-end gap-2">
          <button className="ui-btn-secondary" onClick={close} disabled={busy}>{isZh ? '取消' : 'Cancel'}</button>
          <button className="ui-btn-primary" onClick={commit} disabled={busy || !preview.valid}>{isZh ? '确认导入' : 'Confirm import'}</button>
        </div>
      </section>
    </div>}
  </>;
}
