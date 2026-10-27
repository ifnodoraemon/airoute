import { X, Loader2 } from 'lucide-react';
import {
  SELF_SERVICE_DEFAULT_RPM,
  SELF_SERVICE_MAX_RPM,
  SELF_SERVICE_DEFAULT_TPM,
  SELF_SERVICE_MAX_TPM,
} from '../hooks/useKeyActions';
import useModalA11y from '../hooks/useModalA11y';
import useFormDirty from '../hooks/useFormDirty';
import useUnsavedGuard from '../hooks/useUnsavedGuard';
import DiscardChangesDialog from './DiscardChangesDialog';

export default function KeyModal({
  isOpen,
  onClose,
  newKey,
  setNewKey,
  pricingGroups = ['default'],
  groupMultipliers = {},
  adminUser,
  onSubmit,
  submitting = false,
  createdKey = null,
  onCopy,
  lang = 'zh',
  t
}) {
  // Dismissing the dialog must not silently drop a half-filled key form.
  const isDirty = useFormDirty(isOpen, newKey);
  const { confirming, requestClose, keepEditing, discard } = useUnsavedGuard({ isOpen, isDirty, onClose });
  const guardedClose = () => {
    // The creation succeeded: the form draft is gone and the only thing on screen is
    // the one-time credential, which must not trigger a "discard changes" prompt.
    if (createdKey) {
      onClose?.();
      return;
    }
    // A create request is in flight. The plaintext credential exists only in that
    // response, so dismissing now would drop it for good: ignore the attempt and
    // let the response reveal the key. The request always settles (success or
    // failure), so this never traps the user permanently.
    if (submitting) return;
    requestClose();
  };
  useModalA11y(isOpen, guardedClose);

  if (!isOpen) return null;
  const isZh = lang === 'zh';
  const isAdmin = adminUser?.role === 'admin';
  const copy = (zh, en) => (isZh ? zh : en);

  // The plaintext credential is only ever available in the creation response.
  if (createdKey) {
    return (
      <div
        className="fixed inset-0 bg-slate-900/60 flex items-center justify-center p-4 z-50 backdrop-blur-sm animate-in fade-in"
        onClick={(e) => {
          if (e.target === e.currentTarget) guardedClose();
        }}
      >
        <div className="bg-white dark:bg-[#191B1F] border border-[#E4E6E9] dark:border-[#2C3038] rounded-2xl p-6 max-w-md w-full space-y-4 shadow-2xl animate-in zoom-in-95 duration-150">
          <div className="border-b border-[#E4E6E9] dark:border-[#2C3038] pb-3">
            <h3 className="font-semibold text-base text-[#202124] dark:text-[#F1F2F4] tracking-tight">
              {t?.keyCreatedTitle || copy('密钥已创建', 'Key created')}
            </h3>
          </div>

          <p role="alert" className="text-xs leading-relaxed rounded-xl border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-amber-900 dark:text-amber-200">
            {t?.keyCreatedOnceWarning || copy(
              '这是该密钥唯一一次显示明文，请立即复制并妥善保存；关闭后无法再次查看。',
              'This is the only time the plaintext key is shown. Copy and store it now; it cannot be viewed again.',
            )}
          </p>

          <div>
            <label className="ui-label">{copy('API 密钥（仅此一次显示）', 'API key (shown once)')}</label>
            <div className="flex items-start space-x-2">
              <code data-testid="created-key" className="flex-1 break-all rounded-lg bg-[#F0F1F2] dark:bg-[#22252B] border border-[#E4E6E9] dark:border-[#2C3038] px-3 py-2 font-mono text-xs select-all">
                {createdKey.key}
              </code>
              <button
                type="button"
                onClick={() => onCopy?.(createdKey.key)}
                className="ui-btn-secondary ui-btn-sm shrink-0"
              >
                {copy('复制', 'Copy')}
              </button>
            </div>
          </div>

          {createdKey.fingerprint && (
            <div className="text-xs">
              <span className="text-[#656B75] dark:text-[#A1A7B0]">{t?.keyFingerprintLabel || copy('密钥指纹', 'Key fingerprint')}: </span>
              <code className="font-mono select-all">{createdKey.fingerprint}</code>
            </div>
          )}

          <div className="ui-meta-sm space-y-1">
            <div>{copy('归属租户', 'Tenant')}: <span className="font-mono">{createdKey.tenant}</span></div>
            <div>RPM: <span className="font-mono">{createdKey.rpm}</span> · TPM: <span className="font-mono">{createdKey.tpm}</span></div>
            {createdKey.quotaAdjusted && (
              <div className="text-amber-600 dark:text-amber-400">
                {t?.keyQuotaClamped || copy('已按自助密钥上限调整速率/Token 配额', 'Rate/token quotas were adjusted to the self-service limits')}
              </div>
            )}
          </div>

          <div className="flex justify-end pt-2 border-t border-[#E4E6E9] dark:border-[#2C3038]">
            <button type="button" onClick={guardedClose} className="ui-btn-primary">
              {t?.keyDismissCreated || copy('我已保存，关闭', 'Saved — close')}
            </button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div
      className="fixed inset-0 bg-slate-900/60 flex items-center justify-center p-4 z-50 backdrop-blur-sm animate-in fade-in"
      onClick={(e) => {
        if (e.target === e.currentTarget) guardedClose();
      }}
    >
      <form
        onSubmit={onSubmit}
        aria-busy={submitting}
        className="bg-white dark:bg-[#191B1F] border border-[#E4E6E9] dark:border-[#2C3038] rounded-2xl p-6 max-w-md w-full space-y-4 shadow-2xl animate-in zoom-in-95 duration-150"
      >
        <div className="flex items-center justify-between border-b border-[#E4E6E9] dark:border-[#2C3038] pb-3">
          <div>
            <h3 className="font-semibold text-base text-[#202124] dark:text-[#F1F2F4] tracking-tight">
              {isZh ? '新建 API 密钥' : 'Create API Key'}
            </h3>
          </div>
          <button
            type="button"
            onClick={guardedClose}
            className="ui-icon-btn"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        <div className="space-y-4 text-xs">
          {isAdmin ? (
            <div>
              <label className="ui-label">
                {t?.keyTenantLabel || (isZh ? '租户标识' : 'Tenant identifier')} <span className="text-rose-500">*</span>
              </label>
              <input
                required
                value={newKey.tenant_id}
                onChange={(e) => setNewKey({ ...newKey, tenant_id: e.target.value })}
                placeholder={isZh ? '如 个人开发 / Cursor / Claude Code' : 'e.g. Cursor, Claude Code, Dev Team'}
                className="ui-field"
              />
              <p className="ui-meta-sm mt-1">
                {t?.keyTenantAdminHint || (isZh
                  ? '管理员可指定该密钥归属的租户标识，将用于用量与账单归属。'
                  : 'Administrators may set which tenant the key belongs to; usage and billing are attributed to it.')}
              </p>
            </div>
          ) : (
            <div>
              <label className="ui-label">
                {t?.keyTenantServerAssigned || (isZh ? '归属租户（由服务器分配）' : 'Tenant (assigned by the server)')}
              </label>
              <div className="ui-field bg-[#F0F1F2] dark:bg-[#22252B] font-mono flex items-center">
                {adminUser?.username || adminUser?.tenant_id || '—'}
              </div>
              <p className="ui-meta-sm mt-1">
                {t?.keyTenantServerAssignedHint || (isZh
                  ? '个人密钥自动归属您的账号，无需也无法自行填写。'
                  : 'A personal key always belongs to your account; this cannot be chosen.')}
              </p>
            </div>
          )}

          <div>
            <label className="ui-label text-xs">
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
                          ? 'bg-[#355CFF] text-white border-[#355CFF] font-semibold shadow-xs'
                          : 'bg-white dark:bg-[#191B1F] border-[#E4E6E9] dark:border-[#2C3038] text-[#202124] dark:text-[#F1F2F4] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B]'
                      }`}
                    >
                      {(() => {
                        const mult = groupMultipliers?.[g] ?? (g === 'vip' ? 0.8 : g === 'enterprise' ? 1.5 : 1.0);
                        if (g === 'default') return isZh ? `默认组 (1.0x 官方基准)` : `Default (1.0x)`;
                        if (g === 'vip') return isZh ? `VIP组 (${mult}x · ${Math.round(mult * 10)}折)` : `VIP (${mult}x)`;
                        if (g === 'enterprise') return isZh ? `企业组 (${mult}x 保障)` : `Enterprise (${mult}x)`;
                        return `${g} (${mult}x)`;
                      })()}
                    </button>
                  ))}
                </div>
                <input
                  type="text"
                  placeholder={isZh ? '或输入自定义计费分组标识' : 'or enter custom pricing group ID'}
                  value={newKey.group_name || ''}
                  onChange={(e) => setNewKey({ ...newKey, group_name: e.target.value.toLowerCase().replace(/[^a-z0-9_-]/g, '') })}
                  className="ui-field ui-field-sm font-mono"
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
                          ? 'bg-[#EEF2FF] dark:bg-[#242D48] border-[#355CFF] text-[#202124] dark:text-[#F1F2F4] ring-1 ring-[#355CFF]'
                          : 'bg-white dark:bg-[#191B1F] border-[#E4E6E9] dark:border-[#2C3038] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B]'
                      }`}
                    >
                      <div className="text-xs font-bold">{isZh ? '默认基础组 (default)' : 'Default Group (default)'}</div>
                      <div className="ui-meta mt-0.5">
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
                        {adminUser.group_name.toUpperCase()} {isZh ? `专属保障组 (${groupMultipliers?.[adminUser.group_name] ?? 0.8}x)` : `Tier Group (${groupMultipliers?.[adminUser.group_name] ?? 0.8}x)`}
                      </div>
                      <div className="ui-meta mt-0.5">
                        {isZh ? '享受您账号签约的特权与优惠费率' : 'Enjoy your account negotiated discount rates'}
                      </div>
                    </button>
                  </div>
                ) : (
                  <div className="p-3 rounded-2xl bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 flex items-center justify-between">
                    <div>
                      <div className="text-xs font-semibold text-slate-800 dark:text-slate-200">{isZh ? '默认计费组 (Standard)' : 'Default Group (Standard)'}</div>
                      <div className="ui-meta mt-0.5">{isZh ? '该密钥调用将按通用标准费率结算扣费' : 'Usage will be billed under general standard rates'}</div>
                    </div>
                    <span className="px-2 py-0.5 rounded-lg text-xs font-semibold bg-[#EEF2FF] dark:bg-[#242D48] text-[#355CFF] dark:text-[#91A7FF] border border-[#355CFF]/20 dark:border-[#91A7FF]/30">
                      {isZh ? '标准费率' : 'Standard'}
                    </span>
                  </div>
                )}
              </div>
            )}
          </div>

          {isAdmin ? (
            <div>
              <div className="flex items-center justify-between mb-1">
                <label className="font-semibold text-[#202124] dark:text-[#F1F2F4]">{isZh ? 'API 密钥' : 'API Key'}</label>
                <button
                  type="button"
                  onClick={() => setNewKey(prev => ({ ...prev, key: '' }))}
                  className="text-xs text-[#355CFF] dark:text-[#91A7FF] hover:underline flex items-center space-x-1 cursor-pointer"
                >
                  <span>{isZh ? '由服务端安全生成' : 'Generate server-side'}</span>
                </button>
              </div>
              <input
                value={newKey.key}
                onChange={(e) => setNewKey({ ...newKey, key: e.target.value })}
                placeholder={isZh ? '留空自动生成' : 'Leave empty to auto-generate'}
                className="ui-field font-mono"
              />
            </div>
          ) : (
            <p className="ui-meta-sm rounded-lg border border-[#E4E6E9] dark:border-[#2C3038] bg-[#F0F1F2]/60 dark:bg-[#22252B]/60 px-3 py-2">
              {t?.keySecretUnavailable || (isZh
                ? '密钥明文仅在创建时显示一次，创建后请立即复制保存。'
                : 'The plaintext key is shown once, at creation — copy it before closing.')}
            </p>
          )}

          <div>
            <div className="flex items-center justify-between mb-1">
              <label className="font-semibold text-[#202124] dark:text-[#F1F2F4]">
                {isZh ? '额度限制 (CNY, 0 为不限)' : 'Quota Limit (CNY, 0 for unlimited)'}
              </label>
              <div className="flex items-center space-x-1.5 text-xs">
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
                    className="text-[#355CFF] dark:text-[#91A7FF] hover:underline px-1 cursor-pointer"
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
              className="ui-field font-mono"
            />
          </div>

          <div>
            <div className="flex items-center justify-between mb-1">
              <label className="font-semibold text-[#202124] dark:text-[#F1F2F4]">
                {isAdmin
                  ? (t?.keyRpmLabel || (isZh ? '速率上限 (RPM)' : 'Rate limit (RPM)'))
                  : `${t?.keyRpmLabel || (isZh ? '速率上限 (RPM)' : 'Rate limit (RPM)')} (${SELF_SERVICE_DEFAULT_RPM}–${SELF_SERVICE_MAX_RPM})`}
              </label>
              <div className="flex items-center space-x-1.5 text-xs">
                {(isAdmin
                  ? [{ label: isZh ? '不限' : 'Unlimited', val: 0 }, { label: '60', val: 60 }, { label: '120', val: 120 }, { label: '600', val: 600 }]
                  // Self-service keys cannot be unlimited: 0 would silently become
                  // the default, so the presets only offer real allowances.
                  : [{ label: '60', val: 60 }, { label: '120', val: 120 }, { label: '600', val: 600 }, { label: '6000', val: 6000 }]
                ).map(p => (
                  <button
                    key={p.label}
                    type="button"
                    onClick={() => setNewKey(prev => ({ ...prev, rpm: p.val }))}
                    className="text-[#355CFF] dark:text-[#91A7FF] hover:underline px-1 cursor-pointer"
                  >
                    {p.label}
                  </button>
                ))}
              </div>
            </div>
            <input
              type="number"
              min={isAdmin ? 0 : 1}
              max={isAdmin ? undefined : SELF_SERVICE_MAX_RPM}
              step="1"
              value={newKey.rpm ?? 0}
              onChange={(e) => setNewKey(prev => ({ ...prev, rpm: parseInt(e.target.value, 10) || 0 }))}
              className="ui-field font-mono"
            />
            <p className="ui-meta-sm mt-1">
              {isAdmin
                ? (t?.keyRpmAdminHint || (isZh ? '0 表示不限速（仅管理员可创建不限速密钥）。' : '0 means unlimited — only an administrator can create an unlimited key.'))
                : (t?.keyRpmSelfServiceHint || (isZh
                  ? '默认 60 RPM，自助创建最高 6000 RPM；如需不限速请联系管理员。'
                  : 'Defaults to 60 RPM; self-service keys cap at 6000 RPM. Contact an administrator for unlimited.'))}
            </p>
          </div>

          <div>
            <div className="flex items-center justify-between mb-1">
              <label className="font-semibold text-[#202124] dark:text-[#F1F2F4]">
                {isAdmin
                  ? (t?.keyTpmLabel || (isZh ? 'Token 上限 (TPM)' : 'Token limit (TPM)'))
                  : `${t?.keyTpmLabel || (isZh ? 'Token 上限 (TPM)' : 'Token limit (TPM)')} (${SELF_SERVICE_DEFAULT_TPM}–${SELF_SERVICE_MAX_TPM})`}
              </label>
              <div className="flex items-center space-x-1.5 text-xs">
                {(isAdmin
                  ? [{ label: isZh ? '不限' : 'Unlimited', val: 0 }, { label: '100K', val: 100000 }, { label: '1M', val: 1000000 }]
                  : [{ label: '100K', val: 100000 }, { label: '1M', val: 1000000 }, { label: '10M', val: SELF_SERVICE_MAX_TPM }]
                ).map(p => (
                  <button
                    key={p.label}
                    type="button"
                    onClick={() => setNewKey(prev => ({ ...prev, tpm: p.val }))}
                    className="text-[#355CFF] dark:text-[#91A7FF] hover:underline px-1 cursor-pointer"
                  >
                    {p.label}
                  </button>
                ))}
              </div>
            </div>
            <input
              type="number"
              min={isAdmin ? 0 : 1}
              max={isAdmin ? undefined : SELF_SERVICE_MAX_TPM}
              step="1"
              value={newKey.tpm ?? 0}
              onChange={(e) => setNewKey(prev => ({ ...prev, tpm: parseInt(e.target.value, 10) || 0 }))}
              className="ui-field font-mono"
            />
            <p className="ui-meta-sm mt-1">
              {isAdmin
                ? (t?.keyTpmAdminHint || (isZh ? '0 表示不限量（仅管理员可创建不限量密钥）。' : '0 means unlimited — only an administrator can create an unlimited key.'))
                : (t?.keyTpmSelfServiceHint || (isZh
                  ? '默认 100,000 TPM，自助创建最高 10,000,000 TPM；如需更高请联系管理员。'
                  : 'Defaults to 100,000 TPM; self-service keys cap at 10,000,000 TPM. Contact an administrator for more.'))}
            </p>
          </div>
        </div>

        <div className="flex items-center justify-between gap-3 pt-4 border-t border-[#E4E6E9] dark:border-[#2C3038]">
          <span className="ui-meta-sm truncate">
            {isDirty && !submitting ? (isZh ? '有未保存的修改' : 'Unsaved changes') : ''}
          </span>
          <div className="flex items-center space-x-3 shrink-0">
            <button
              type="button"
              onClick={guardedClose}
              className="ui-btn-ghost"
            >
              {isZh ? '取消' : 'Cancel'}
            </button>
            <button
              type="submit"
              disabled={submitting}
              className="ui-btn-primary"
            >
              {submitting ? (
                <>
                  <Loader2 className="w-3.5 h-3.5 animate-spin" />
                  <span>{isZh ? '创建中…' : 'Creating…'}</span>
                </>
              ) : (
                <span>{isZh ? '确认创建' : 'Create Key'}</span>
              )}
            </button>
          </div>
        </div>
      </form>

      <DiscardChangesDialog
        open={confirming}
        onKeepEditing={keepEditing}
        onDiscard={discard}
        lang={lang}
        title={isZh ? '放弃未保存的密钥配置？' : 'Discard this key draft?'}
      />
    </div>
  );
}
