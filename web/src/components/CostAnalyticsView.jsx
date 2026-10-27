import { useCallback, useEffect, useState } from 'react';
import { Calculator, RefreshCw } from 'lucide-react';
import SupplierCostPanel from './SupplierCostPanel';

function money(value) {
  return `¥${Number(value || 0).toFixed(4)}`;
}

const GROUP_TABS = [
  { key: 'org', zh: '组织', en: 'Organization' },
  { key: 'channel', zh: '通道', en: 'Channel' },
  { key: 'model', zh: '模型', en: 'Model' },
];

// CostAnalyticsView shows revenue vs supplier cost (margin) and the price list
// that drives cost_basis_units. Unknown-cost requests stay visible: a margin
// that silently assumed zero cost would be fiction.
export default function CostAnalyticsView({ adminFetch, showToast, lang = 'zh' }) {
  const isZh = lang === 'zh';
  const [overview, setOverview] = useState(null);
  const [groups, setGroups] = useState([]);
  const [groupBy, setGroupBy] = useState('org');
  const [loading, setLoading] = useState(false);

  const notify = useCallback((message, type = 'success') => {
    if (showToast) showToast(message, type);
  }, [showToast]);

  const load = useCallback(async () => {
    if (!adminFetch) return;
    setLoading(true);
    const results = await Promise.allSettled([
      adminFetch('/api/v1/admin/cost/overview'),
      adminFetch(`/api/v1/admin/cost/breakdown?group=${groupBy}`),
    ]);
    const [overviewResult, groupsResult] = results;
    if (overviewResult.status === 'fulfilled' && overviewResult.value.ok) {
      const body = await overviewResult.value.json().catch(() => ({}));
      if (body.code === 0) setOverview(body.data || {});
    }
    if (groupsResult.status === 'fulfilled' && groupsResult.value.ok) {
      const body = await groupsResult.value.json().catch(() => ({}));
      if (body.code === 0) setGroups(Array.isArray(body.data) ? body.data : []);
    }
    const failure = results.find((result) => result.status === 'rejected');
    if (failure) notify(failure.reason?.message || String(failure.reason), 'error');
    setLoading(false);
  }, [adminFetch, groupBy, notify]);

  useEffect(() => { load(); }, [load]);

  const cardClass = 'rounded-xl border border-[#E4E6E9] dark:border-[#2C3038] bg-white dark:bg-[#191B1F] p-4';
  const total = (key) => groups.reduce((sum, group) => sum + Number(group[key] || 0), 0);

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center space-x-2">
          <Calculator className="w-5 h-5 text-[#355CFF] dark:text-[#91A7FF]" />
          <div>
            <h2 className="text-sm font-semibold text-[#202124] dark:text-[#F1F2F4]">{isZh ? '成本与毛利' : 'Cost & Margin'}</h2>
            <p className="text-[11px] text-[#656B75] dark:text-[#A1A7B0]">
              {isZh ? '营收来自售价，成本来自供应商价目；未定价请求单独计数。' : 'Revenue is list price, cost is supplier price; unpriced requests are counted separately.'}
            </p>
          </div>
        </div>
        <button type="button" onClick={load} disabled={loading} className="inline-flex items-center space-x-1.5 px-2.5 py-1.5 rounded-lg border border-[#E4E6E9] dark:border-[#2C3038] text-xs text-[#656B75] dark:text-[#A1A7B0]">
          <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
          <span>{isZh ? '刷新' : 'Refresh'}</span>
        </button>
      </div>

      <div className="grid grid-cols-2 lg:grid-cols-5 gap-3">
        <div className={cardClass}>
          <div className="text-[11px] text-[#656B75] dark:text-[#A1A7B0]">{isZh ? '营收' : 'Revenue'}</div>
          <div className="mt-1 text-xl font-semibold">{money(overview?.revenue)}</div>
        </div>
        <div className={cardClass}>
          <div className="text-[11px] text-[#656B75] dark:text-[#A1A7B0]">{isZh ? '供应商成本' : 'Supplier cost'}</div>
          <div className="mt-1 text-xl font-semibold">{money(overview?.cost)}</div>
        </div>
        <div className={cardClass}>
          <div className="text-[11px] text-[#656B75] dark:text-[#A1A7B0]">{isZh ? '毛利' : 'Gross margin'}</div>
          <div className={`mt-1 text-xl font-semibold ${Number(overview?.margin || 0) >= 0 ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-600 dark:text-red-400'}`}>{money(overview?.margin)}</div>
        </div>
        <div className={cardClass}>
          <div className="text-[11px] text-[#656B75] dark:text-[#A1A7B0]">{isZh ? '毛利率' : 'Margin %'}</div>
          <div className="mt-1 text-xl font-semibold">{Number(overview?.margin_percent || 0).toFixed(2)}%</div>
        </div>
        <div className={cardClass}>
          <div className="text-[11px] text-[#656B75] dark:text-[#A1A7B0]">{isZh ? '未定价请求' : 'Unpriced requests'}</div>
          <div className="mt-1 text-xl font-semibold text-amber-600 dark:text-amber-400">{Number(overview?.unknown_cost_requests || 0).toLocaleString()}</div>
          <div className="text-[10px] text-[#656B75] dark:text-[#A1A7B0]">{isZh ? `共 ${Number(overview?.requests || 0).toLocaleString()} 次请求` : `${Number(overview?.requests || 0).toLocaleString()} requests`}</div>
        </div>
      </div>

      <div className="rounded-xl border border-[#E4E6E9] dark:border-[#2C3038] bg-white dark:bg-[#191B1F] p-3">
        <div className="flex items-center justify-between mb-2">
          <div className="flex items-center space-x-1.5">
            {GROUP_TABS.map((tab) => (
              <button
                key={tab.key}
                type="button"
                onClick={() => setGroupBy(tab.key)}
                className={`px-2.5 py-1 rounded-lg text-[11px] ${groupBy === tab.key ? 'bg-[#EEF2FF] text-[#355CFF] dark:bg-[#242D48] dark:text-[#91A7FF]' : 'text-[#656B75] dark:text-[#A1A7B0] hover:bg-[#F7F8FA] dark:hover:bg-[#202329]'}`}
              >
                {isZh ? tab.zh : tab.en}
              </button>
            ))}
          </div>
          <div className="text-[10px] text-[#656B75] dark:text-[#A1A7B0]">
            {isZh ? `分组营收 ${money(total('revenue'))} / 毛利 ${money(total('margin'))}` : `Group revenue ${money(total('revenue'))} / margin ${money(total('margin'))}`}
          </div>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="text-left text-[10px] uppercase tracking-wide text-[#656B75] dark:text-[#A1A7B0] border-b border-[#E4E6E9] dark:border-[#2C3038]">
                <th className="py-2 pr-3">{isZh ? '分组' : 'Group'}</th>
                <th className="py-2 pr-3 text-right">{isZh ? '请求' : 'Requests'}</th>
                <th className="py-2 pr-3 text-right">{isZh ? '营收' : 'Revenue'}</th>
                <th className="py-2 pr-3 text-right">{isZh ? '成本' : 'Cost'}</th>
                <th className="py-2 pr-3 text-right">{isZh ? '毛利' : 'Margin'}</th>
                <th className="py-2 pr-3 text-right">{isZh ? '毛利率' : 'Margin %'}</th>
                <th className="py-2 text-right">{isZh ? '已定价' : 'Priced'}</th>
              </tr>
            </thead>
            <tbody>
              {groups.map((group) => (
                <tr key={group.key || group.label} className="border-b border-[#F0F1F2] dark:border-[#22252B]">
                  <td className="py-2 pr-3">{group.label || group.key || '未归属'}</td>
                  <td className="py-2 pr-3 text-right">{Number(group.requests || 0).toLocaleString()}</td>
                  <td className="py-2 pr-3 text-right font-mono">{money(group.revenue)}</td>
                  <td className="py-2 pr-3 text-right font-mono">{money(group.cost)}</td>
                  <td className={`py-2 pr-3 text-right font-mono ${Number(group.margin || 0) >= 0 ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-600 dark:text-red-400'}`}>{money(group.margin)}</td>
                  <td className="py-2 pr-3 text-right">{Number(group.margin_percent || 0).toFixed(1)}%</td>
                  <td className="py-2 text-right">{Number(group.known_cost_requests || 0).toLocaleString()}</td>
                </tr>
              ))}
              {groups.length === 0 && (
                <tr><td colSpan={7} className="py-6 text-center text-[11px] text-[#656B75] dark:text-[#A1A7B0]">{isZh ? '暂无数据' : 'No data yet'}</td></tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      <SupplierCostPanel adminFetch={adminFetch} showToast={showToast} lang={lang} />
    </div>
  );
}
