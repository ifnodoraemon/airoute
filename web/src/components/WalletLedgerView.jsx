import { useCallback, useEffect, useState } from 'react';
import { Download, RefreshCw, ReceiptText, Search } from 'lucide-react';

const TYPE_LABELS = {
  zh: { opening: '期初', recharge: '充值', charge: '扣费', refund: '退款', adjustment: '调整', redemption: '兑换' },
  en: { opening: 'Opening', recharge: 'Recharge', charge: 'Charge', refund: 'Refund', adjustment: 'Adjustment', redemption: 'Redemption' },
};

const TYPE_BADGES = {
  recharge: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300',
  charge: 'bg-red-50 text-red-700 dark:bg-red-950/40 dark:text-red-300',
  refund: 'bg-sky-50 text-sky-700 dark:bg-sky-950/40 dark:text-sky-300',
  adjustment: 'bg-amber-50 text-amber-700 dark:bg-amber-950/40 dark:text-amber-300',
  redemption: 'bg-indigo-50 text-indigo-700 dark:bg-indigo-950/40 dark:text-indigo-300',
  opening: 'bg-[#F0F1F2] text-[#656B75] dark:bg-[#22252B] dark:text-[#A1A7B0]',
};

function money(value) {
  const number = Number(value || 0);
  return `${number < 0 ? '-' : ''}¥${Math.abs(number).toFixed(4)}`;
}

function formatTime(value) {
  if (!value) return '';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString('zh-CN');
}

// WalletLedgerView is the admin view over wallet_transactions. Rows are
// append-only; the filter is a user id because the ledger is keyed by account.
export default function WalletLedgerView({ adminFetch, showToast, lang = 'zh' }) {
  const isZh = lang === 'zh';
  const labels = TYPE_LABELS[isZh ? 'zh' : 'en'];
  const [rows, setRows] = useState([]);
  const [userId, setUserId] = useState('');
  const [loading, setLoading] = useState(false);
  const [exporting, setExporting] = useState(false);

  const notify = useCallback((message, type = 'success') => {
    if (showToast) showToast(message, type);
  }, [showToast]);

  const queryString = useCallback((limit = 100) => {
    const params = new URLSearchParams({ limit: String(limit) });
    if (userId.trim()) params.set('user_id', userId.trim());
    return params.toString();
  }, [userId]);

  const load = useCallback(async () => {
    if (!adminFetch) return;
    setLoading(true);
    try {
      const res = await adminFetch(`/api/v1/admin/wallet/transactions?${queryString()}`);
      const body = await res.json().catch(() => ({}));
      if (!res.ok || body.code !== 0) throw new Error(body.error || `HTTP ${res.status}`);
      setRows(Array.isArray(body.data) ? body.data : []);
    } catch (err) {
      notify(err.message, 'error');
    } finally {
      setLoading(false);
    }
  }, [adminFetch, queryString, notify]);

  useEffect(() => {
    load();
  }, [load]);

  const exportCSV = async () => {
    if (!adminFetch) return;
    setExporting(true);
    try {
      const res = await adminFetch(`/api/v1/admin/wallet/transactions/export?${queryString(10000)}`);
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new Error(body.error || `HTTP ${res.status}`);
      }
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = `wallet-transactions-${new Date().toISOString().slice(0, 10)}.csv`;
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(url);
      notify(isZh ? '导出已开始' : 'Export started');
    } catch (err) {
      notify(err.message, 'error');
    } finally {
      setExporting(false);
    }
  };

  const inputClass = 'px-2.5 py-1.5 rounded-lg border border-[#E4E6E9] dark:border-[#2C3038] bg-white dark:bg-[#191B1F] text-xs text-[#202124] dark:text-[#F1F2F4]';

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center space-x-2">
          <ReceiptText className="w-5 h-5 text-[#355CFF] dark:text-[#91A7FF]" />
          <div>
            <h2 className="text-sm font-semibold text-[#202124] dark:text-[#F1F2F4]">{isZh ? '钱包资金流水' : 'Wallet Ledger'}</h2>
            <p className="text-[11px] text-[#656B75] dark:text-[#A1A7B0]">
              {isZh ? '只追加的余额变动记录：期初、充值、扣费、退款、调整与兑换。' : 'Append-only balance movements: opening, recharge, charge, refund, adjustment, redemption.'}
            </p>
          </div>
        </div>
        <div className="flex items-center space-x-2">
          <button type="button" onClick={load} disabled={loading} className="inline-flex items-center space-x-1.5 px-2.5 py-1.5 rounded-lg border border-[#E4E6E9] dark:border-[#2C3038] text-xs text-[#656B75] dark:text-[#A1A7B0] disabled:opacity-50">
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
            <span>{isZh ? '刷新' : 'Refresh'}</span>
          </button>
          <button type="button" onClick={exportCSV} disabled={exporting} className="inline-flex items-center space-x-1.5 px-2.5 py-1.5 rounded-lg border border-[#E4E6E9] dark:border-[#2C3038] text-xs text-[#656B75] dark:text-[#A1A7B0] disabled:opacity-50">
            <Download className="w-3.5 h-3.5" />
            <span>{isZh ? '导出 CSV' : 'Export CSV'}</span>
          </button>
        </div>
      </div>

      <form onSubmit={(event) => { event.preventDefault(); load(); }} className="flex items-center space-x-2 p-3 rounded-xl border border-[#E4E6E9] dark:border-[#2C3038] bg-white dark:bg-[#191B1F]">
        <Search className="w-4 h-4 text-[#656B75] dark:text-[#A1A7B0]" />
        <input value={userId} onChange={(event) => setUserId(event.target.value)} className={inputClass} placeholder={isZh ? '用户 ID（留空为全部）' : 'User ID (empty for all)'} aria-label={isZh ? '用户 ID' : 'User ID'} />
        <button type="submit" className="px-2.5 py-1.5 rounded-lg bg-[#355CFF] hover:bg-[#2A4AE0] text-white text-xs font-medium">{isZh ? '查询' : 'Search'}</button>
      </form>

      <div className="rounded-xl border border-[#E4E6E9] dark:border-[#2C3038] bg-white dark:bg-[#191B1F] overflow-x-auto">
        <table className="w-full text-xs">
          <thead>
            <tr className="text-left text-[10px] uppercase tracking-wide text-[#656B75] dark:text-[#A1A7B0] border-b border-[#E4E6E9] dark:border-[#2C3038]">
              <th className="px-3 py-2">{isZh ? '时间' : 'Time'}</th>
              <th className="px-3 py-2">{isZh ? '用户' : 'User'}</th>
              <th className="px-3 py-2">{isZh ? '类型' : 'Type'}</th>
              <th className="px-3 py-2 text-right">{isZh ? '变动' : 'Delta'}</th>
              <th className="px-3 py-2 text-right">{isZh ? '余额' : 'Balance'}</th>
              <th className="px-3 py-2">{isZh ? '说明 / 引用' : 'Reason / ref'}</th>
              <th className="px-3 py-2">{isZh ? '操作员' : 'Operator'}</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => {
              const positive = Number(row.delta || 0) >= 0;
              return (
                <tr key={row.id} className="border-b border-[#F0F1F2] dark:border-[#22252B]">
                  <td className="px-3 py-2 whitespace-nowrap text-[#656B75] dark:text-[#A1A7B0]">{formatTime(row.created_at)}</td>
                  <td className="px-3 py-2">
                    <span className="text-[#202124] dark:text-[#F1F2F4]">{row.username || `#${row.user_id}`}</span>
                    <span className="ml-1 font-mono text-[10px] text-[#656B75] dark:text-[#A1A7B0]">#{row.user_id}</span>
                  </td>
                  <td className="px-3 py-2">
                    <span className={`px-1.5 py-0.5 rounded text-[10px] ${TYPE_BADGES[row.type] || TYPE_BADGES.opening}`}>{labels[row.type] || row.type}</span>
                  </td>
                  <td className={`px-3 py-2 text-right font-mono ${positive ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-600 dark:text-red-400'}`}>{positive ? '+' : ''}{money(row.delta)}</td>
                  <td className="px-3 py-2 text-right font-mono text-[#202124] dark:text-[#F1F2F4]">{money(row.balance_after)}</td>
                  <td className="px-3 py-2 text-[#656B75] dark:text-[#A1A7B0]">
                    <span>{row.reason || '-'}</span>
                    {row.ref_id && <span className="ml-1 font-mono text-[10px]">{row.ref_id}</span>}
                  </td>
                  <td className="px-3 py-2 text-[#656B75] dark:text-[#A1A7B0]">{row.operator || '-'}</td>
                </tr>
              );
            })}
            {rows.length === 0 && (
              <tr><td colSpan={7} className="px-3 py-8 text-center text-[11px] text-[#656B75] dark:text-[#A1A7B0]">{isZh ? '暂无流水' : 'No ledger rows yet'}</td></tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
