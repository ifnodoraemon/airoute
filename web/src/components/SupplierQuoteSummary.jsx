import { quoteCurrency, quoteMode, quoteMoney } from '../lib/supplierQuotes';

export default function SupplierQuoteSummary({ quote, lang = 'zh' }) {
  const isZh = lang === 'zh';
  if (!quote)
    return (
      <span className="text-amber-700 dark:text-amber-300">
        {isZh ? '未配置采购报价' : 'Purchase quote not configured'}
      </span>
    );
  const currency = quoteCurrency(quote);
  const rate = (value) =>
    value == null ? (isZh ? '未报价' : 'Not quoted') : quoteMoney(value, currency);
  return (
    <div className="space-y-1 text-xs font-sans">
      <p className="font-medium">
        {currency} ·{' '}
        {quoteMode(quote) === 'request'
          ? isZh
            ? '每次请求'
            : 'per request'
          : isZh
            ? '每百万 Token'
            : 'per 1M tokens'}
        {quote.status === 'disabled' && (
          <span className="ml-2 text-amber-700 dark:text-amber-300">
            {isZh ? '已停用' : 'Disabled'}
          </span>
        )}
      </p>
      {quoteMode(quote) === 'request' ? (
        <p>{rate(quote.fixed_cost)}</p>
      ) : (
        <>
          <p>
            {isZh ? '输入 / 输出：' : 'Input / output: '}
            {rate(quote.input_cost)} / {rate(quote.output_cost)}
          </p>
          <p className="text-slate-500">
            {isZh ? '缓存读取：' : 'Cache read: '}
            {rate(quote.cache_read_cost)}
          </p>
          <p className="text-slate-500">
            {isZh ? '缓存写入：' : 'Cache write: '}
            {rate(quote.cache_write_cost)}
          </p>
        </>
      )}
    </div>
  );
}
