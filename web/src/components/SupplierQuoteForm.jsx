import { useEffect, useRef, useState } from 'react';
import { quoteCurrency, quoteMode } from '../lib/supplierQuotes';

export default function SupplierQuoteForm({
  initial = {},
  channels = [],
  catalog = [],
  adminFetch,
  onSaved,
  onCancel,
  showToast,
  lang = 'zh',
}) {
  const isZh = lang === 'zh';
  const formRef = useRef(null);
  useEffect(() => { formRef.current?.scrollIntoView?.({ block: 'start' }); }, []);
  const [form, setForm] = useState(() => ({
    channel: initial.channel || '',
    model: initial.model || '',
    currency: quoteCurrency(initial),
    billing_mode: quoteMode(initial),
    input_cost: initial.input_cost ?? '',
    output_cost: initial.output_cost ?? '',
    cache_read_cost: initial.cache_read_cost ?? '',
    cache_write_cost: initial.cache_write_cost ?? '',
    fixed_cost: initial.fixed_cost ?? '',
    status: initial.status || 'active',
  }));
  const [busy, setBusy] = useState(false);
  const update = (key, value) => setForm((prev) => ({ ...prev, [key]: value }));
  const save = async (event) => {
    event.preventDefault();
    setBusy(true);
    try {
      const payload = {
        channel: form.channel.trim(),
        model: form.model.trim(),
        currency: form.currency.trim().toUpperCase(),
        billing_mode: form.billing_mode,
        status: form.status,
      };
      for (const key of form.billing_mode === 'request'
        ? ['fixed_cost']
        : ['input_cost', 'output_cost', 'cache_read_cost', 'cache_write_cost']) {
        if (form[key] !== '') payload[key] = Number(form[key]);
      }
      const res = await adminFetch('/api/v1/admin/supplier-costs', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      const body = await res.json();
      if (!res.ok || body.code !== 0) throw new Error(body.error || `HTTP ${res.status}`);
      showToast?.(isZh ? '采购报价已保存' : 'Purchase quote saved', 'success');
      await onSaved?.();
    } catch (err) {
      showToast?.(err.message, 'error');
    } finally {
      setBusy(false);
    }
  };
  const numberField = (key, zh, en, required = false) => (
    <label className="block text-xs space-y-1">
      {isZh ? zh : en}
      <input
        aria-label={isZh ? zh : en}
        className="ui-field"
        type="number"
        min="0"
        max="1000000000"
        step="0.000000001"
        required={required}
        value={form[key]}
        onChange={(e) => update(key, e.target.value)}
        placeholder={required ? '' : isZh ? '留空表示未报价' : 'Blank = not quoted'}
      />
    </label>
  );
  return (
    <form
      ref={formRef}
      onSubmit={save}
      className="rounded-xl border border-indigo-200 dark:border-indigo-900 bg-indigo-50/30 dark:bg-indigo-950/10 p-4 space-y-3"
    >
      <h3 className="text-sm font-semibold">
        {isZh ? '编辑渠道采购报价' : 'Edit channel purchase quote'}
      </h3>
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <label className="block text-xs space-y-1">
          {isZh ? '渠道' : 'Channel'}
          {channels.length > 0 ? (
            <select
              aria-label={isZh ? '报价渠道' : 'Quote channel'}
              className="ui-field"
              required
              value={form.channel}
              onChange={(e) => setForm((prev) => ({ ...prev, channel: e.target.value, model: '' }))}
            >
              <option value="">{isZh ? '选择渠道' : 'Select channel'}</option>
              {[
                ...new Set([
                  ...channels.map((row) => row.name),
                  ...(initial.channel ? [initial.channel] : []),
                ]),
              ].map((name) => (
                <option key={name}>{name}</option>
              ))}
            </select>
          ) : (
            <input
              aria-label={isZh ? '报价渠道' : 'Quote channel'}
              className="ui-field"
              required
              value={form.channel}
              onChange={(e) => update('channel', e.target.value)}
            />
          )}
        </label>
        <label className="block text-xs space-y-1">
          {isZh ? '实际上游模型' : 'Actual upstream model'}
          <input
            aria-label={isZh ? '实际上游模型' : 'Actual upstream model'}
            className="ui-field"
            required
            list="supplier-upstream-models"
            value={form.model}
            onChange={(e) => update('model', e.target.value)}
          />
          <datalist id="supplier-upstream-models">
            {catalog
              .filter((row) => row.channel === form.channel)
              .map((row) => (
                <option key={row.model} value={row.model} />
              ))}
          </datalist>
        </label>
        <label className="block text-xs space-y-1">
          {isZh ? '报价币种' : 'Quote currency'}
          <input
            aria-label={isZh ? '报价币种' : 'Quote currency'}
            className="ui-field uppercase"
            list="supplier-currencies"
            pattern="[A-Za-z]{3}"
            maxLength={3}
            required
            value={form.currency}
            onChange={(e) => update('currency', e.target.value.toUpperCase())}
          />
          <datalist id="supplier-currencies">
            {['CNY', 'USD', 'EUR', 'GBP', 'JPY', 'HKD'].map((value) => (
              <option key={value}>{value}</option>
            ))}
          </datalist>
        </label>
        <label className="block text-xs space-y-1">
          {isZh ? '采购计费方式' : 'Purchase billing basis'}
          <select
            aria-label={isZh ? '采购计费方式' : 'Purchase billing basis'}
            className="ui-field"
            value={form.billing_mode}
            onChange={(e) => update('billing_mode', e.target.value)}
          >
            <option value="token">{isZh ? '按百万 Token' : 'Per 1M tokens'}</option>
            <option value="request">{isZh ? '按次请求' : 'Per request'}</option>
          </select>
        </label>
      </div>
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {form.billing_mode === 'request' ? (
          numberField('fixed_cost', '每次请求采购价', 'Purchase price per request', true)
        ) : (
          <>
            {numberField('input_cost', '输入采购价', 'Input purchase price', true)}
            {numberField('output_cost', '输出采购价', 'Output purchase price', true)}
            {numberField('cache_read_cost', '缓存读取采购价', 'Cache read purchase price')}
            {numberField('cache_write_cost', '缓存写入采购价', 'Cache write purchase price')}
          </>
        )}
      </div>
      <p className="text-xs text-slate-500">
        {isZh
          ? `以上报价单位：${form.currency || '—'} / ${form.billing_mode === 'request' ? '次请求' : '百万 Token'}。缓存报价未知可留空；输入、输出或按次价为必填，已确认免费才填写 0。采购报价只作参考，不改变客户扣费。`
          : `Rates: ${form.currency} / ${form.billing_mode === 'request' ? 'request' : '1M tokens'}. Blank means unknown; enter 0 only for confirmed free rates. Quotes do not change customer billing.`}
      </p>
      <div className="flex flex-wrap justify-end items-center gap-2">
        <label className="text-xs">
          {isZh ? '报价状态' : 'Quote status'}{' '}
          <select
            aria-label={isZh ? '报价状态' : 'Quote status'}
            className="ui-field inline-block w-auto"
            value={form.status}
            onChange={(e) => update('status', e.target.value)}
          >
            <option value="active">{isZh ? '启用' : 'Active'}</option>
            <option value="disabled">{isZh ? '停用' : 'Disabled'}</option>
          </select>
        </label>
        <button type="button" className="ui-btn-secondary" disabled={busy} onClick={onCancel}>
          {isZh ? '取消' : 'Cancel'}
        </button>
        <button className="ui-btn-primary" disabled={busy}>
          {busy ? (isZh ? '保存中…' : 'Saving…') : isZh ? '保存采购报价' : 'Save purchase quote'}
        </button>
      </div>
    </form>
  );
}
