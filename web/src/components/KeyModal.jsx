import { X, Loader2 } from 'lucide-react';
import KeyPricingGroup from './KeyPricingGroup';
import KeyQuotaFields from './KeyQuotaFields';
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

          {createdKey.keyId && (
            <div className="text-xs space-y-0.5">
              <div>
                <span className="text-[#656B75] dark:text-[#A1A7B0]">{t?.keyIdLabel || copy('密钥 ID', 'Key ID')}: </span>
                <code className="font-mono select-all">{createdKey.keyId}</code>
              </div>
              <p className="text-[11px] leading-relaxed text-[#656B75] dark:text-[#A1A7B0]">
                {t?.keyIdHint || copy(
                  '密钥 ID 仅用于在日志、账单和工单中识别密钥，不能用于鉴权。',
                  'The key ID identifies the key in logs, billing and support tickets; it cannot authenticate a call.',
                )}
              </p>
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

          <KeyPricingGroup newKey={newKey} setNewKey={setNewKey} pricingGroups={pricingGroups}
            groupMultipliers={groupMultipliers} adminUser={adminUser} isZh={isZh} />

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

          {/* Protocol format validation level. Empty means "follow the gateway
              default", which is what the resolver does with no key-level value.
              Only the admin create endpoint accepts this field. */}
          {isAdmin && (
            <div>
              <label className="ui-label">{isZh ? '格式校验级别' : 'Format validation'}</label>
              <select
                value={newKey.format_validation || ''}
                onChange={(e) => setNewKey({ ...newKey, format_validation: e.target.value })}
                className="ui-field cursor-pointer"
              >
                <option value="">{isZh ? '跟随全局（默认）' : 'Follow gateway default'}</option>
                <option value="off">{isZh ? '关闭 (off)' : 'Off'}</option>
                <option value="lenient">{isZh ? '宽松纠错 (lenient)' : 'Lenient'}</option>
                <option value="strict">{isZh ? '严格拦截 (strict)' : 'Strict'}</option>
              </select>
              <p className="ui-meta-sm mt-1">
                {isZh
                  ? '按协议校验该密钥的请求体格式；留空时沿用网关全局设置。'
                  : 'Validates request bodies sent with this key; empty follows the gateway-wide setting.'}
              </p>
            </div>
          )}

          <KeyQuotaFields newKey={newKey} setNewKey={setNewKey} isAdmin={isAdmin} isZh={isZh} t={t} />
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
