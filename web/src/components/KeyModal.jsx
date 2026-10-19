import React from 'react';
import { X } from 'lucide-react';
import useModalA11y from '../hooks/useModalA11y';

export default function KeyModal({
  isOpen,
  onClose,
  newKey,
  setNewKey,
  pricingGroups = ['default'],
  adminUser,
  onSubmit,
  lang = 'zh',
  t
}) {
  useModalA11y(isOpen, onClose);

  if (!isOpen) return null;
  const isZh = lang === 'zh';

  return (
    <div
      className="fixed inset-0 bg-slate-900/60 flex items-center justify-center p-4 z-50 backdrop-blur-sm animate-in fade-in"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <form
        onSubmit={onSubmit}
        className="bg-white dark:bg-[#0F1420] border border-slate-200/70 dark:border-slate-800/80 rounded-2xl p-6 max-w-md w-full space-y-4 shadow-2xl animate-in zoom-in-95 duration-150"
      >
        <div className="flex items-center justify-between border-b border-slate-100 dark:border-slate-800/80 pb-3">
          <div>
            <h3 className="font-semibold text-base text-slate-900 dark:text-white tracking-tight">
              {isZh ? '新建 API 密钥' : 'Create API Key'}
            </h3>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-1.5 hover:bg-slate-100 dark:hover:bg-slate-800 rounded-lg text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 transition cursor-pointer"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        <div className="space-y-4 text-xs">
          <div>
            <label className="block font-semibold text-slate-700 dark:text-slate-300 mb-1">
              {isZh ? '密钥名称' : 'Key Name / Description'} <span className="text-rose-500">*</span>
            </label>
            <input
              required
              value={newKey.tenant_id}
              onChange={(e) => setNewKey({ ...newKey, tenant_id: e.target.value })}
              placeholder={isZh ? '如 个人开发 / Cursor / Claude Code' : 'e.g. Cursor, Claude Code, Dev Team'}
              className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-3 py-2 text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500 text-xs"
            />
          </div>

          <div>
            <label className="block font-semibold text-slate-700 dark:text-slate-300 mb-1 text-xs">
              {isZh ? '计费分组 (Pricing Group)' : 'Pricing Group'}
            </label>
            {adminUser?.role === 'admin' ? (
              <div className="space-y-2">
                <div className="flex flex-wrap items-center gap-1.5">
                  {pricingGroups.map(g => (
                    <button
                      key={g}
                      type="button"
                      onClick={() => setNewKey({ ...newKey, group_name: g })}
                      className={`px-3 py-1.5 rounded-lg text-xs font-medium border transition cursor-pointer ${
                        (newKey.group_name || 'default') === g
                          ? 'bg-slate-900 dark:bg-indigo-600 text-white border-slate-900 dark:border-indigo-600 font-semibold shadow-sm'
                          : 'bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-300 hover:bg-slate-50'
                      }`}
                    >
                      {g === 'default'
                        ? (isZh ? '默认组 (default)' : 'Default (default)')
                        : g === 'vip'
                        ? (isZh ? 'VIP组 (vip)' : 'VIP (vip)')
                        : g === 'enterprise'
                        ? (isZh ? '企业组 (enterprise)' : 'Enterprise (enterprise)')
                        : `${g} ${isZh ? '组' : 'Group'}`}
                    </button>
                  ))}
                </div>
                <input
                  type="text"
                  placeholder={isZh ? '或输入自定义计费分组标识' : 'or enter custom pricing group ID'}
                  value={newKey.group_name || ''}
                  onChange={(e) => setNewKey({ ...newKey, group_name: e.target.value.toLowerCase().replace(/[^a-z0-9_-]/g, '') })}
                  className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-3 py-1.5 font-mono text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500 text-xs"
                />
              </div>
            ) : (
              <div className="space-y-2">
                {adminUser?.group_name && adminUser.group_name !== 'default' ? (
                  <div className="grid grid-cols-2 gap-2">
                    <button
                      type="button"
                      onClick={() => setNewKey({ ...newKey, group_name: 'default' })}
                      className={`p-3 rounded-2xl border text-left transition cursor-pointer ${
                        (newKey.group_name || 'default') === 'default'
                          ? 'bg-indigo-50/70 dark:bg-indigo-950/40 border-indigo-500 text-indigo-950 dark:text-indigo-200 ring-1 ring-indigo-500'
                          : 'bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 hover:bg-slate-50'
                      }`}
                    >
                      <div className="text-xs font-bold">{isZh ? '默认基础组 (default)' : 'Default Group (default)'}</div>
                      <div className="text-[11px] text-slate-500 dark:text-slate-400 mt-0.5">
                        {isZh ? '标准定价，适合常规调用或测试环境' : 'Standard rate, ideal for general dev and testing'}
                      </div>
                    </button>
                    <button
                      type="button"
                      onClick={() => setNewKey({ ...newKey, group_name: adminUser.group_name })}
                      className={`p-3 rounded-2xl border text-left transition cursor-pointer ${
                        newKey.group_name === adminUser.group_name
                          ? 'bg-amber-50/70 dark:bg-amber-950/40 border-amber-500 text-amber-950 dark:text-amber-200 ring-1 ring-amber-500'
                          : 'bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 hover:bg-slate-50'
                      }`}
                    >
                      <div className="text-xs font-bold text-amber-600 dark:text-amber-400">
                        {adminUser.group_name.toUpperCase()} {isZh ? '专属保障组' : 'Tier Group'}
                      </div>
                      <div className="text-[11px] text-slate-500 dark:text-slate-400 mt-0.5">
                        {isZh ? '享受您账号签约的特权与优惠费率' : 'Enjoy your account negotiated discount rates'}
                      </div>
                    </button>
                  </div>
                ) : (
                  <div className="p-3 rounded-2xl bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 flex items-center justify-between">
                    <div>
                      <div className="text-xs font-semibold text-slate-800 dark:text-slate-200">{isZh ? '默认计费组 (Standard)' : 'Default Group (Standard)'}</div>
                      <div className="text-[11px] text-slate-400 mt-0.5">{isZh ? '该密钥调用将按通用标准费率结算扣费' : 'Usage will be billed under general standard rates'}</div>
                    </div>
                    <span className="px-2 py-0.5 rounded-lg text-[11px] font-semibold bg-indigo-50 text-indigo-700 border border-indigo-200">
                      {isZh ? '标准费率' : 'Standard'}
                    </span>
                  </div>
                )}
              </div>
            )}
          </div>

          <div>
            <div className="flex items-center justify-between mb-1">
              <label className="font-semibold text-slate-700 dark:text-slate-300">{isZh ? 'API 密钥' : 'API Key'}</label>
              <button
                type="button"
                onClick={() => setNewKey(prev => ({ ...prev, key: '' }))}
                className="text-[11px] text-indigo-600 dark:text-indigo-400 hover:underline flex items-center space-x-1 cursor-pointer"
              >
                <span>{isZh ? '由服务端安全生成' : 'Generate server-side'}</span>
              </button>
            </div>
            <input
              value={newKey.key}
              onChange={(e) => setNewKey({ ...newKey, key: e.target.value })}
              placeholder={isZh ? '留空自动生成' : 'Leave empty to auto-generate'}
              className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-3 py-2 text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500 font-mono text-xs"
            />
          </div>

          <div>
            <div className="flex items-center justify-between mb-1">
              <label className="font-semibold text-slate-700 dark:text-slate-300">
                {isZh ? '额度限制 (CNY, 0 为不限)' : 'Quota Limit (CNY, 0 for unlimited)'}
              </label>
              <div className="flex items-center space-x-1.5 text-[11px]">
                {[
                  { label: isZh ? '不限' : 'Unlimited', val: 0 },
                  { label: '¥10', val: 10 },
                  { label: '¥50', val: 50 },
                  { label: '¥100', val: 100 },
                ].map(p => (
                  <button
                    key={p.label}
                    type="button"
                    onClick={() => setNewKey(prev => ({ ...prev, budget: p.val }))}
                    className="text-indigo-600 dark:text-indigo-400 hover:underline px-1 cursor-pointer"
                  >
                    {p.label}
                  </button>
                ))}
              </div>
            </div>
            <input
              type="number"
              step="0.01"
              min="0"
              value={newKey.budget ?? 0}
              onChange={(e) => setNewKey({ ...newKey, budget: parseFloat(e.target.value) || 0 })}
              className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-3 py-2 text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500 font-mono text-xs"
            />
          </div>

          <div>
            <div className="flex items-center justify-between mb-1">
              <label className="font-semibold text-slate-700 dark:text-slate-300">
                {isZh ? '速率限制 (RPM, 0 为不限)' : 'Rate Limit (RPM, 0 for unlimited)'}
              </label>
              <div className="flex items-center space-x-1.5 text-[11px]">
                {[
                  { label: isZh ? '不限' : 'Unlimited', val: 0 },
                  { label: '60', val: 60 },
                  { label: '120', val: 120 },
                  { label: '600', val: 600 },
                ].map(p => (
                  <button
                    key={p.label}
                    type="button"
                    onClick={() => setNewKey(prev => ({ ...prev, rpm: p.val }))}
                    className="text-indigo-600 dark:text-indigo-400 hover:underline px-1 cursor-pointer"
                  >
                    {p.label}
                  </button>
                ))}
              </div>
            </div>
            <input
              type="number"
              value={newKey.rpm}
              onChange={(e) => setNewKey({ ...newKey, rpm: parseInt(e.target.value) || 0 })}
              className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-3 py-2 text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500 font-mono text-xs"
            />
          </div>
        </div>

        <div className="flex justify-end space-x-3 pt-4 border-t border-slate-100 dark:border-slate-800/80">
          <button
            type="button"
            onClick={onClose}
            className="px-4 py-2 text-xs text-slate-500 hover:text-slate-800 dark:hover:text-slate-200 font-medium transition cursor-pointer"
          >
            {isZh ? '取消' : 'Cancel'}
          </button>
          <button
            type="submit"
            className="px-4 py-2 bg-slate-900 dark:bg-indigo-600 hover:bg-slate-800 dark:hover:bg-indigo-500 text-white rounded-lg text-xs font-medium shadow-sm transition cursor-pointer"
          >
            {isZh ? '确认创建' : 'Create Key'}
          </button>
        </div>
      </form>
    </div>
  );
}
