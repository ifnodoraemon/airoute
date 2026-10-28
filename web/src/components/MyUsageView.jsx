import { useCallback, useEffect, useMemo, useState } from 'react';
import { Activity, Coins, Key, RefreshCw, Wallet, Zap } from 'lucide-react';
import { keyDisplayRef } from '../lib/apiKey';

function formatNumber(value) {
  const number = Number(value || 0);
  return number.toLocaleString('zh-CN');
}

function formatMoney(value) {
  return `¥${Number(value || 0).toFixed(4)}`;
}

// MyUsageView is the self-service dashboard for regular accounts. It only reads
// endpoints the user is allowed to see: the stats endpoints are scoped to the
// caller's own keys on the server, and /user/* is the account's own wallet and
// credentials. No admin endpoint is called here.
export default function MyUsageView({ adminFetch, showToast, lang = 'zh' }) {
  const isZh = lang === 'zh';
  const [overview, setOverview] = useState(null);
  const [wallet, setWallet] = useState(null);
  const [keys, setKeys] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const request = useCallback(async (path) => {
    const res = await adminFetch(path);
    const body = await res.json().catch(() => ({}));
    if (!res.ok || body.code !== 0) {
      throw new Error(body.error || `HTTP ${res.status}`);
    }
    return body.data;
  }, [adminFetch]);

  const load = useCallback(async () => {
    if (!adminFetch) return;
    setLoading(true);
    setError('');
    const results = await Promise.allSettled([
      request('/api/v1/admin/stats/overview'),
      request('/api/v1/user/wallet'),
      request('/api/v1/user/keys'),
    ]);
    const [overviewResult, walletResult, keysResult] = results;
    if (overviewResult.status === 'fulfilled') setOverview(overviewResult.value || {});
    if (walletResult.status === 'fulfilled') setWallet(walletResult.value || {});
    if (keysResult.status === 'fulfilled') setKeys(Array.isArray(keysResult.value) ? keysResult.value : []);

    const failure = results.find((result) => result.status === 'rejected');
    if (failure) {
      const message = failure.reason?.message || String(failure.reason);
      setError(message);
      if (showToast) showToast(message, 'error');
    }
    setLoading(false);
  }, [adminFetch, request, showToast]);

  useEffect(() => {
    load();
  }, [load]);

  const totals = useMemo(() => ({
    requests: overview?.total_requests ?? 0,
    tokens: overview?.total_tokens ?? 0,
    cost: overview?.total_cost ?? 0,
    saved: overview?.saved_cost ?? 0,
    balance: wallet?.balance ?? 0,
    activeKeys: (keys || []).filter((key) => key.status === 'active').length,
  }), [overview, wallet, keys]);

  const cardClass = 'rounded-xl border border-[#E4E6E9] dark:border-[#2C3038] bg-white dark:bg-[#191B1F] p-4';

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-sm font-semibold text-[#202124] dark:text-[#F1F2F4]">{isZh ? '我的用量' : 'My Usage'}</h2>
          <p className="text-[11px] text-[#656B75] dark:text-[#A1A7B0]">
            {isZh ? '仅统计你本人密钥的调用；配额水位来自密钥的预算与速率限制。' : 'Only your own keys are counted. Quota waterlines come from each key budget and rate limit.'}
          </p>
        </div>
        <button type="button" onClick={load} disabled={loading} className="inline-flex items-center space-x-1.5 px-2.5 py-1.5 rounded-lg border border-[#E4E6E9] dark:border-[#2C3038] text-xs text-[#656B75] dark:text-[#A1A7B0] disabled:opacity-50">
          <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
          <span>{isZh ? '刷新' : 'Refresh'}</span>
        </button>
      </div>

      {error && (
        <div className="rounded-lg border border-amber-200 bg-amber-50 dark:border-amber-900/40 dark:bg-amber-950/30 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
          {isZh ? '部分数据加载失败：' : 'Some data failed to load: '}{error}
        </div>
      )}

      <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
        <div className={cardClass}>
          <div className="flex items-center space-x-1.5 text-[11px] text-[#656B75] dark:text-[#A1A7B0]"><Activity className="w-3.5 h-3.5" /><span>{isZh ? '请求数' : 'Requests'}</span></div>
          <div className="mt-1 text-xl font-semibold text-[#202124] dark:text-[#F1F2F4]">{formatNumber(totals.requests)}</div>
        </div>
        <div className={cardClass}>
          <div className="flex items-center space-x-1.5 text-[11px] text-[#656B75] dark:text-[#A1A7B0]"><Zap className="w-3.5 h-3.5" /><span>{isZh ? 'Tokens' : 'Tokens'}</span></div>
          <div className="mt-1 text-xl font-semibold text-[#202124] dark:text-[#F1F2F4]">{formatNumber(totals.tokens)}</div>
        </div>
        <div className={cardClass}>
          <div className="flex items-center space-x-1.5 text-[11px] text-[#656B75] dark:text-[#A1A7B0]"><Coins className="w-3.5 h-3.5" /><span>{isZh ? '累计费用' : 'Total cost'}</span></div>
          <div className="mt-1 text-xl font-semibold text-[#202124] dark:text-[#F1F2F4]">{formatMoney(totals.cost)}</div>
          <div className="text-[10px] text-emerald-600 dark:text-emerald-400">{isZh ? '已节省 ' : 'Saved '}{formatMoney(totals.saved)}</div>
        </div>
        <div className={cardClass}>
          <div className="flex items-center space-x-1.5 text-[11px] text-[#656B75] dark:text-[#A1A7B0]"><Wallet className="w-3.5 h-3.5" /><span>{isZh ? '钱包余额' : 'Wallet balance'}</span></div>
          <div className="mt-1 text-xl font-semibold text-[#202124] dark:text-[#F1F2F4]">{formatMoney(totals.balance)}</div>
          <div className="text-[10px] text-[#656B75] dark:text-[#A1A7B0]">{isZh ? '活跃密钥 ' : 'Active keys '}{totals.activeKeys}</div>
        </div>
      </div>

      <div className={cardClass}>
        <div className="flex items-center space-x-1.5 text-xs font-semibold text-[#202124] dark:text-[#F1F2F4] mb-2">
          <Key className="w-3.5 h-3.5" />
          <span>{isZh ? '密钥与配额水位' : 'Keys and quota waterlines'}</span>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="text-left text-[10px] uppercase tracking-wide text-[#656B75] dark:text-[#A1A7B0] border-b border-[#E4E6E9] dark:border-[#2C3038]">
                <th className="py-2 pr-3">{isZh ? '密钥' : 'Key'}</th>
                <th className="py-2 pr-3">{isZh ? '状态' : 'Status'}</th>
                <th className="py-2 pr-3">{isZh ? '预算水位' : 'Budget'}</th>
                <th className="py-2 pr-3">RPM</th>
                <th className="py-2">{isZh ? '已用 / 限额 (TPM)' : 'Used / limit (TPM)'}</th>
              </tr>
            </thead>
            <tbody>
              {keys.map((key) => {
                const budget = Number(key.budget || 0);
                const used = Number(key.used_cost || 0);
                const percent = budget > 0 ? Math.min(100, (used / budget) * 100) : 0;
                return (
                  <tr key={key.id} className="border-b border-[#F0F1F2] dark:border-[#22252B]">
                    <td className="py-2 pr-3 font-mono text-[11px] text-[#202124] dark:text-[#F1F2F4]">{keyDisplayRef(key)}</td>
                    <td className="py-2 pr-3">
                      <span className={`px-1.5 py-0.5 rounded text-[10px] ${key.status === 'active' ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300' : 'bg-amber-50 text-amber-700 dark:bg-amber-950/40 dark:text-amber-300'}`}>{key.status}</span>
                    </td>
                    <td className="py-2 pr-3 min-w-[140px]">
                      {budget > 0 ? (
                        <>
                          <div className="flex items-center justify-between text-[10px] text-[#656B75] dark:text-[#A1A7B0]"><span>{formatMoney(used)}</span><span>{formatMoney(budget)}</span></div>
                          <div className="h-1.5 mt-1 rounded-full bg-[#F0F1F2] dark:bg-[#22252B] overflow-hidden">
                            <div className={`h-full ${percent >= 100 ? 'bg-red-500' : percent >= 80 ? 'bg-amber-500' : 'bg-[#355CFF]'}`} style={{ width: `${percent}%` }} />
                          </div>
                        </>
                      ) : (
                        <span className="text-[10px] text-[#656B75] dark:text-[#A1A7B0]">{isZh ? '不限' : 'Unlimited'}</span>
                      )}
                    </td>
                    <td className="py-2 pr-3 font-mono text-[11px]">{key.rpm > 0 ? key.rpm : '∞'}</td>
                    <td className="py-2 font-mono text-[11px]">{key.tpm > 0 ? `${formatNumber(key.used_tokens)} / ${formatNumber(key.tpm)}` : '∞'}</td>
                  </tr>
                );
              })}
              {keys.length === 0 && (
                <tr><td colSpan={5} className="py-6 text-center text-[11px] text-[#656B75] dark:text-[#A1A7B0]">{isZh ? '还没有密钥' : 'No keys yet'}</td></tr>
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
