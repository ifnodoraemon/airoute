import { useCallback, useEffect, useMemo, useState } from 'react';
import { Gauge, Plus, RefreshCw, Trash2, Pencil, X } from 'lucide-react';

const EMPTY_FORM = {
  scope_type: 'org',
  scope_id: '',
  period: 'monthly',
  amount: '',
  action: 'notify',
  warn_percent: 80,
  status: 'active',
};

const PERIOD_LABELS = {
  zh: { daily: '每日', monthly: '每月', total: '总额' },
  en: { daily: 'Daily', monthly: 'Monthly', total: 'Total' },
};

function money(value) {
  return `¥${Number(value || 0).toFixed(4)}`;
}

// BudgetsView manages the three-level budget rules. Scope ids are resolved to
// readable names from the organization/user/key lists; the server remains the
// authority on which scopes exist.
export default function BudgetsView({ adminFetch, showToast, lang = 'zh' }) {
  const isZh = lang === 'zh';
  const periods = PERIOD_LABELS[isZh ? 'zh' : 'en'];
  const [budgets, setBudgets] = useState([]);
  const [orgs, setOrgs] = useState([]);
  const [users, setUsers] = useState([]);
  const [keys, setKeys] = useState([]);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [form, setForm] = useState(EMPTY_FORM);
  const [editingId, setEditingId] = useState(null);

  const request = useCallback(async (path, options) => {
    const res = await adminFetch(path, options);
    const body = await res.json().catch(() => ({}));
    if (!res.ok || body.code !== 0) {
      throw new Error(body.error || `HTTP ${res.status}`);
    }
    return body.data;
  }, [adminFetch]);

  const notify = useCallback((message, type = 'success') => {
    if (showToast) showToast(message, type);
  }, [showToast]);

  const load = useCallback(async () => {
    if (!adminFetch) return;
    setLoading(true);
    const results = await Promise.allSettled([
      request('/api/v1/admin/budgets'),
      request('/api/v1/admin/orgs'),
      request('/api/v1/admin/users'),
      request('/api/v1/admin/keys'),
    ]);
    const [budgetResult, orgResult, userResult, keyResult] = results;
    if (budgetResult.status === 'fulfilled') setBudgets(Array.isArray(budgetResult.value) ? budgetResult.value : []);
    if (orgResult.status === 'fulfilled') setOrgs(Array.isArray(orgResult.value) ? orgResult.value : []);
    if (userResult.status === 'fulfilled') setUsers(Array.isArray(userResult.value) ? userResult.value : []);
    if (keyResult.status === 'fulfilled') setKeys(Array.isArray(keyResult.value) ? keyResult.value : []);
    const failure = results.find((result) => result.status === 'rejected');
    if (failure) notify(failure.reason?.message || String(failure.reason), 'error');
    setLoading(false);
  }, [adminFetch, request, notify]);

  useEffect(() => {
    load();
  }, [load]);

  const scopeOptions = useMemo(() => {
    if (form.scope_type === 'org') return orgs.map((org) => ({ id: org.id, label: `${org.name} (${org.code})` }));
    if (form.scope_type === 'user') return users.map((user) => ({ id: user.id, label: user.username || `#${user.id}` }));
    return keys.map((key) => ({ id: key.id, label: key.key_fingerprint || key.key || `#${key.id}` }));
  }, [form.scope_type, orgs, users, keys]);

  const scopeName = useCallback((budget) => {
    if (budget.scope_type === 'org') {
      const org = orgs.find((item) => item.id === budget.scope_id);
      return org ? org.name : `org#${budget.scope_id}`;
    }
    if (budget.scope_type === 'user') {
      const user = users.find((item) => item.id === budget.scope_id);
      return user ? user.username : `user#${budget.scope_id}`;
    }
    const key = keys.find((item) => item.id === budget.scope_id);
    return key ? (key.key_fingerprint || key.key) : `key#${budget.scope_id}`;
  }, [orgs, users, keys]);

  const resetForm = () => {
    setEditingId(null);
    setForm(EMPTY_FORM);
  };

  const save = async (event) => {
    event.preventDefault();
    if (!adminFetch) return;
    setBusy(true);
    try {
      const payload = {
        scope_type: form.scope_type,
        scope_id: Number(form.scope_id),
        period: form.period,
        amount: Number(form.amount),
        action: form.action,
        warn_percent: Number(form.warn_percent),
        status: form.status || 'active',
      };
      if (editingId) {
        await request(`/api/v1/admin/budgets/${editingId}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ amount: payload.amount, action: payload.action, warn_percent: payload.warn_percent, status: payload.status }),
        });
        notify(isZh ? '预算已更新' : 'Budget updated');
      } else {
        await request('/api/v1/admin/budgets', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        });
        notify(isZh ? '预算已创建' : 'Budget created');
      }
      resetForm();
      await load();
    } catch (err) {
      notify(err.message, 'error');
    } finally {
      setBusy(false);
    }
  };

  const edit = (budget) => {
    setEditingId(budget.id);
    setForm({
      scope_type: budget.scope_type,
      scope_id: String(budget.scope_id),
      period: budget.period,
      amount: String(budget.amount),
      action: budget.action,
      warn_percent: budget.warn_percent,
      status: budget.status,
    });
  };

  const remove = async (budget) => {
    setBusy(true);
    try {
      await request(`/api/v1/admin/budgets/${budget.id}`, { method: 'DELETE' });
      notify(isZh ? '预算已删除' : 'Budget deleted');
      if (editingId === budget.id) resetForm();
      await load();
    } catch (err) {
      notify(err.message, 'error');
    } finally {
      setBusy(false);
    }
  };

  const inputClass = 'w-full px-2.5 py-1.5 rounded-lg border border-[#E4E6E9] dark:border-[#2C3038] bg-white dark:bg-[#191B1F] text-xs text-[#202124] dark:text-[#F1F2F4]';
  const buttonClass = 'inline-flex items-center space-x-1.5 px-2.5 py-1.5 rounded-lg bg-[#355CFF] hover:bg-[#2A4AE0] text-white text-xs font-medium transition disabled:opacity-50';

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center space-x-2">
          <Gauge className="w-5 h-5 text-[#355CFF] dark:text-[#91A7FF]" />
          <div>
            <h2 className="text-sm font-semibold text-[#202124] dark:text-[#F1F2F4]">{isZh ? '预算与告警' : 'Budgets & Alerts'}</h2>
            <p className="text-[11px] text-[#656B75] dark:text-[#A1A7B0]">
              {isZh ? '支持组织/用户/密钥三级预算；阈值触发通知，耗尽可停用。' : 'Organization/user/key budgets; thresholds notify, exhaustion can disable.'}
            </p>
          </div>
        </div>
        <button type="button" onClick={load} disabled={loading} className="inline-flex items-center space-x-1.5 px-2.5 py-1.5 rounded-lg border border-[#E4E6E9] dark:border-[#2C3038] text-xs text-[#656B75] dark:text-[#A1A7B0] disabled:opacity-50">
          <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
          <span>{isZh ? '刷新' : 'Refresh'}</span>
        </button>
      </div>

      <form onSubmit={save} className="grid grid-cols-2 md:grid-cols-7 gap-2 p-3 rounded-xl border border-[#E4E6E9] dark:border-[#2C3038] bg-white dark:bg-[#191B1F]">
        <select value={form.scope_type} disabled={!!editingId} onChange={(e) => setForm((prev) => ({ ...prev, scope_type: e.target.value, scope_id: '' }))} className={inputClass} aria-label={isZh ? '范围类型' : 'Scope type'}>
          <option value="org">{isZh ? '组织' : 'Organization'}</option>
          <option value="user">{isZh ? '用户' : 'User'}</option>
          <option value="key">{isZh ? '密钥' : 'Key'}</option>
        </select>
        <select value={form.scope_id} disabled={!!editingId} onChange={(e) => setForm((prev) => ({ ...prev, scope_id: e.target.value }))} className={inputClass} aria-label={isZh ? '范围' : 'Scope'}>
          <option value="">{isZh ? '选择范围' : 'Select scope'}</option>
          {scopeOptions.map((option) => <option key={option.id} value={option.id}>{option.label}</option>)}
        </select>
        <select value={form.period} disabled={!!editingId} onChange={(e) => setForm((prev) => ({ ...prev, period: e.target.value }))} className={inputClass} aria-label={isZh ? '周期' : 'Period'}>
          <option value="daily">{periods.daily}</option>
          <option value="monthly">{periods.monthly}</option>
          <option value="total">{periods.total}</option>
        </select>
        <input type="number" min="0" step="0.01" value={form.amount} onChange={(e) => setForm((prev) => ({ ...prev, amount: e.target.value }))} className={inputClass} placeholder={isZh ? '金额 (CNY)' : 'Amount (CNY)'} aria-label={isZh ? '金额' : 'Amount'} />
        <select value={form.action} onChange={(e) => setForm((prev) => ({ ...prev, action: e.target.value }))} className={inputClass} aria-label={isZh ? '动作' : 'Action'}>
          <option value="notify">{isZh ? '仅通知' : 'Notify'}</option>
          <option value="disable">{isZh ? '停用' : 'Disable'}</option>
        </select>
        <input type="number" min="0" max="100" value={form.warn_percent} onChange={(e) => setForm((prev) => ({ ...prev, warn_percent: e.target.value }))} className={inputClass} aria-label={isZh ? '告警阈值' : 'Warn percent'} />
        <div className="flex items-center space-x-1.5">
          <button type="submit" disabled={busy || !form.scope_id || !form.amount} className={buttonClass}>
            <Plus className="w-3.5 h-3.5" />
            <span>{editingId ? (isZh ? '保存' : 'Save') : (isZh ? '创建' : 'Create')}</span>
          </button>
          {editingId && (
            <button type="button" onClick={resetForm} className="p-1.5 rounded-lg border border-[#E4E6E9] dark:border-[#2C3038] text-[#656B75]" title={isZh ? '取消' : 'Cancel'}>
              <X className="w-3.5 h-3.5" />
            </button>
          )}
        </div>
      </form>

      <div className="rounded-xl border border-[#E4E6E9] dark:border-[#2C3038] bg-white dark:bg-[#191B1F] overflow-x-auto">
        <table className="w-full text-xs">
          <thead>
            <tr className="text-left text-[10px] uppercase tracking-wide text-[#656B75] dark:text-[#A1A7B0] border-b border-[#E4E6E9] dark:border-[#2C3038]">
              <th className="px-3 py-2">{isZh ? '范围' : 'Scope'}</th>
              <th className="px-3 py-2">{isZh ? '周期' : 'Period'}</th>
              <th className="px-3 py-2">{isZh ? '金额 / 已用' : 'Amount / used'}</th>
              <th className="px-3 py-2">{isZh ? '水位' : 'Waterline'}</th>
              <th className="px-3 py-2">{isZh ? '动作' : 'Action'}</th>
              <th className="px-3 py-2">{isZh ? '状态' : 'Status'}</th>
              <th className="px-3 py-2 text-right">{isZh ? '操作' : 'Actions'}</th>
            </tr>
          </thead>
          <tbody>
            {budgets.map((budget) => {
              const percent = Math.max(0, Math.min(150, Number(budget.percent || 0)));
              return (
                <tr key={budget.id} className="border-b border-[#F0F1F2] dark:border-[#22252B]">
                  <td className="px-3 py-2">
                    <span className="text-[#202124] dark:text-[#F1F2F4]">{scopeName(budget)}</span>
                    <span className="ml-2 text-[10px] text-[#656B75] dark:text-[#A1A7B0]">{budget.scope_type}</span>
                  </td>
                  <td className="px-3 py-2">{periods[budget.period] || budget.period}</td>
                  <td className="px-3 py-2 font-mono text-[11px]">{money(budget.used)} / {money(budget.amount)}</td>
                  <td className="px-3 py-2 min-w-[120px]">
                    <div className="flex items-center justify-between text-[10px] text-[#656B75] dark:text-[#A1A7B0]"><span>{Number(budget.percent || 0).toFixed(1)}%</span></div>
                    <div className="h-1.5 mt-1 rounded-full bg-[#F0F1F2] dark:bg-[#22252B] overflow-hidden">
                      <div className={`h-full ${percent >= 100 ? 'bg-red-500' : percent >= (budget.warn_percent || 80) ? 'bg-amber-500' : 'bg-[#355CFF]'}`} style={{ width: `${Math.min(100, percent)}%` }} />
                    </div>
                  </td>
                  <td className="px-3 py-2">
                    <span className={`px-1.5 py-0.5 rounded text-[10px] ${budget.action === 'disable' ? 'bg-red-50 text-red-700 dark:bg-red-950/40 dark:text-red-300' : 'bg-sky-50 text-sky-700 dark:bg-sky-950/40 dark:text-sky-300'}`}>{budget.action}</span>
                  </td>
                  <td className="px-3 py-2">
                    <span className={`px-1.5 py-0.5 rounded text-[10px] ${budget.status === 'active' ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300' : 'bg-[#F0F1F2] text-[#656B75] dark:bg-[#22252B] dark:text-[#A1A7B0]'}`}>{budget.status}</span>
                  </td>
                  <td className="px-3 py-2 text-right">
                    <button type="button" onClick={() => edit(budget)} className="p-1 text-[#656B75] hover:text-[#355CFF]" title={isZh ? '编辑' : 'Edit'}><Pencil className="w-3.5 h-3.5" /></button>
                    <button type="button" onClick={() => remove(budget)} disabled={busy} className="p-1 text-[#656B75] hover:text-red-500" title={isZh ? '删除' : 'Delete'}><Trash2 className="w-3.5 h-3.5" /></button>
                  </td>
                </tr>
              );
            })}
            {budgets.length === 0 && (
              <tr><td colSpan={7} className="px-3 py-8 text-center text-[11px] text-[#656B75] dark:text-[#A1A7B0]">{isZh ? '暂无预算' : 'No budgets yet'}</td></tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
