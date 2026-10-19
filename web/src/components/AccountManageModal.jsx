import React, { useState } from 'react';
import {
  X,
  Lock,
  User,
  Copy,
  Check,
  Shield,
  Wallet,
  Eye,
  EyeOff,
  Server,
  Sparkles
} from 'lucide-react';
import { getGatewayBaseUrl, copyToClipboard } from '../config';
import useModalA11y from '../hooks/useModalA11y';
import useFormDirty from '../hooks/useFormDirty';
import useUnsavedGuard from '../hooks/useUnsavedGuard';
import DiscardChangesDialog from './DiscardChangesDialog';

export default function AccountManageModal({
  isOpen,
  onClose,
  adminUser,
  adminToken,
  adminFetch,
  showToast,
  onPasswordChanged,
  initialTab = 'profile',
  lang = 'zh',
  t
}) {
  const isZh = lang === 'zh';

  const [activeTab, setActiveTab] = useState(initialTab || 'profile'); // 'profile' | 'password'

  // Change password state
  const [oldPassword, setOldPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [showOldPass, setShowOldPass] = useState(false);
  const [showNewPass, setShowNewPass] = useState(false);
  const [loading, setLoading] = useState(false);
  const [errorMsg, setErrorMsg] = useState('');
  const [copiedBaseUrl, setCopiedBaseUrl] = useState(false);

  // Only the password form holds user edits; the profile tab is read-only, so it
  // stays out of the draft and can never trip the guard.
  const isDirty = useFormDirty(isOpen, { oldPassword, newPassword, confirmPassword });
  const { confirming, requestClose, keepEditing, discard } = useUnsavedGuard({ isOpen, isDirty, onClose });

  // Escape goes through the guard as well (useModalA11y reads the callback via a ref).
  useModalA11y(isOpen, requestClose);

  // Sync tab and reset error on open. The password fields are also cleared on
  // close: useFormDirty snapshots the draft when the modal opens, so leftovers
  // from a previous session would otherwise look like unsaved edits right away.
  React.useEffect(() => {
    if (isOpen) {
      setActiveTab(initialTab || 'profile');
      setErrorMsg('');
    }
    setOldPassword('');
    setNewPassword('');
    setConfirmPassword('');
  }, [isOpen, initialTab]);

  const isAdmin = adminUser?.role === 'admin';
  const apiBaseUrl = getGatewayBaseUrl();

  const handleCopyBaseUrl = async () => {
    await copyToClipboard(apiBaseUrl);
    setCopiedBaseUrl(true);
    showToast(isZh ? '已复制网关 API Base URL 到剪贴板' : 'Gateway API Base URL copied to clipboard', 'success');
    setTimeout(() => setCopiedBaseUrl(false), 2000);
  };

  const handleChangePassword = async (e) => {
    e.preventDefault();
    setErrorMsg('');

    if (!oldPassword) {
      setErrorMsg(isZh ? '请输入当前密码' : 'Please enter current password');
      return;
    }
    if (!newPassword || newPassword.length < 6) {
      setErrorMsg(isZh ? '新密码长度不能少于 6 个字符' : 'New password must be at least 6 characters');
      return;
    }
    if (newPassword !== confirmPassword) {
      setErrorMsg(isZh ? '两次输入的新密码不一致' : 'New passwords do not match');
      return;
    }

    setLoading(true);
    try {
      const res = await adminFetch('/api/v1/user/password', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          old_password: oldPassword,
          new_password: newPassword
        })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(isZh ? '密码修改成功，请妥善保管' : 'Password changed successfully', 'success');
        setOldPassword('');
        setNewPassword('');
        setConfirmPassword('');
        if (onPasswordChanged) onPasswordChanged();
        // Fields are back to their opening state (and the baseline), so close
        // directly instead of asking the user to discard a saved draft.
        onClose();
      } else {
        setErrorMsg(data.error || (isZh ? '修改密码失败，请核对当前密码' : 'Failed to change password'));
      }
    } catch (err) {
      setErrorMsg(isZh ? '网络请求异常，请稍后重试' : 'Network error, please try again');
    } finally {
      setLoading(false);
    }
  };

  if (!isOpen) return null;

  return (
    <div
      className="fixed inset-0 bg-slate-900/50 dark:bg-slate-950/70 backdrop-blur-sm flex items-center justify-center p-4 z-50 animate-in fade-in duration-150"
      onClick={(e) => {
        if (e.target === e.currentTarget) requestClose();
      }}
    >
      <div className="relative bg-white dark:bg-[#0F1420] rounded-2xl border border-slate-200 dark:border-slate-800 shadow-2xl max-w-lg w-full overflow-hidden animate-in zoom-in-95 duration-150">
        {/* Modal Header */}
        <div className="px-6 py-5 border-b border-slate-100 dark:border-slate-800/80 flex items-center justify-between">
          <div className="flex items-center space-x-3">
            <div className="w-10 h-10 rounded-2xl bg-indigo-50 dark:bg-indigo-500/10 border border-indigo-100 dark:border-indigo-500/20 flex items-center justify-center text-indigo-600 dark:text-indigo-400 font-bold text-base uppercase">
              {adminUser?.username?.charAt(0) || 'U'}
            </div>
            <div>
              <h3 className="text-base font-bold text-slate-900 dark:text-slate-100 tracking-tight">{isZh ? '个人账号中心' : 'Account Center'}</h3>
              <p className="ui-meta">{isZh ? '管理个人资料与 API 接入凭证' : 'Manage profile & API credentials'}</p>
            </div>
          </div>
          <button
            onClick={requestClose}
            className="p-2 rounded-lg text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-800 transition cursor-pointer"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Tab Switcher */}
        <div className="flex items-center px-6 pt-4 border-b border-slate-100 dark:border-slate-800/80 space-x-6">
          <button
            onClick={() => { setActiveTab('profile'); setErrorMsg(''); }}
            className={`pb-3 text-xs font-semibold transition border-b-2 cursor-pointer flex items-center space-x-1.5 ${
              activeTab === 'profile'
                ? 'border-indigo-600 dark:border-indigo-400 text-indigo-600 dark:text-indigo-400'
                : 'border-transparent text-slate-500 dark:text-slate-400 hover:text-slate-800 dark:hover:text-slate-200'
            }`}
          >
            <User className="w-3.5 h-3.5" />
            <span>{isZh ? '账号概览' : 'Overview'}</span>
          </button>
          <button
            onClick={() => { setActiveTab('password'); setErrorMsg(''); }}
            className={`pb-3 text-xs font-semibold transition border-b-2 cursor-pointer flex items-center space-x-1.5 ${
              activeTab === 'password'
                ? 'border-indigo-600 dark:border-indigo-400 text-indigo-600 dark:text-indigo-400'
                : 'border-transparent text-slate-500 dark:text-slate-400 hover:text-slate-800 dark:hover:text-slate-200'
            }`}
          >
            <Lock className="w-3.5 h-3.5" />
            <span>{isZh ? '修改登录密码' : 'Change Password'}</span>
          </button>
        </div>

        {/* Modal Body */}
        <div className="p-6">
          {activeTab === 'profile' && (
            <div className="space-y-5">
              {/* Profile Details Grid */}
              <div className="grid grid-cols-2 gap-3">
                <div className="p-3.5 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-100 dark:border-slate-800">
                  <span className="ui-meta block mb-1 font-medium">{isZh ? '登录用户名' : 'Username'}</span>
                  <span className="text-sm font-bold text-slate-900 dark:text-slate-100 font-mono">{adminUser?.username || '—'}</span>
                </div>
                <div className="p-3.5 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-100 dark:border-slate-800">
                  <span className="ui-meta block mb-1 font-medium">{isZh ? '绑定邮箱' : 'Email'}</span>
                  <span className="text-sm font-semibold text-slate-900 dark:text-slate-100 truncate block">
                    {adminUser?.email || (isZh ? '未绑定' : 'Not set')}
                  </span>
                </div>
                <div className="p-3.5 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-100 dark:border-slate-800">
                  <span className="ui-meta block mb-1 font-medium">{isZh ? '角色身份' : 'Role'}</span>
                  <span className={`inline-flex items-center px-2 py-0.5 rounded-lg text-xs font-semibold ${
                    isAdmin
                      ? 'bg-purple-50 dark:bg-purple-950/50 text-purple-700 dark:text-purple-300 border border-purple-200 dark:border-purple-800'
                      : 'bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800'
                  }`}>
                    {isAdmin ? (isZh ? '超级管理员' : 'Administrator') : (isZh ? '普通用户' : 'Regular User')}
                  </span>
                </div>
                <div className="p-3.5 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-100 dark:border-slate-800">
                  <span className="ui-meta block mb-1 font-medium">{isZh ? '价格分组' : 'Pricing Tier'}</span>
                  <span className="inline-flex items-center px-2 py-0.5 rounded-lg text-xs font-semibold bg-amber-50 dark:bg-amber-950/50 text-amber-700 dark:text-amber-300 border border-amber-200 dark:border-amber-800 font-mono">
                    {adminUser?.group_name || 'default'}
                  </span>
                </div>
              </div>

              {/* Role Scope Notice */}
              <div className="p-3.5 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-200/80 dark:border-slate-800 flex items-start space-x-2.5 text-xs text-slate-600 dark:text-slate-300">
                <Shield className="w-4 h-4 text-indigo-600 dark:text-indigo-400 shrink-0 mt-0.5" />
                <div className="space-y-0.5">
                  <span className="font-bold text-slate-800 dark:text-slate-100 block text-xs">
                    {isAdmin ? (isZh ? '系统管理员权限 (System Administrator)' : 'System Administrator Privileges') : (isZh ? '研发成员权限 (Developer / Member)' : 'Developer / Member Privileges')}
                  </span>
                  <p className="ui-meta leading-relaxed">
                    {isAdmin
                      ? (isZh ? '具备全局管控权限：管理上游渠道与秘钥、模型路由拓扑、费率设定、组织成员与安全策略。' : 'Full access to manage upstream channels, keys, routing topology, pricing tiers, members and security policies.')
                      : (isZh ? '具备安全自服务权限：专属 API 密钥创建与限额管控、调用审计流水与在线调试。' : 'Self-service access to create API keys, set quotas, inspect audit logs, and test in playground.')}
                  </p>
                </div>
              </div>

              {/* Wallet Balance Banner */}
              <div className="p-4 rounded-xl bg-slate-900 dark:bg-slate-800/80 border border-transparent dark:border-slate-700 text-white flex items-center justify-between shadow-sm">
                <div className="flex items-center space-x-3">
                  <div className="w-9 h-9 rounded-xl bg-white/10 flex items-center justify-center text-emerald-400">
                    <Wallet className="w-5 h-5" />
                  </div>
                  <div>
                    <span className="text-xs text-slate-400 block">{isZh ? '账户可用额度' : 'Available Balance'}</span>
                    <span className="text-lg font-black tracking-tight text-white font-mono">
                      {isAdmin ? (isZh ? '无限额度' : 'Unlimited') : `¥ ${Number(adminUser?.balance || 0).toFixed(2)}`}
                    </span>
                  </div>
                </div>
                {isAdmin && (
                  <span className="text-xs px-2 py-0.5 rounded-full bg-indigo-500/30 text-indigo-300 font-medium">
                    {isZh ? '管理员豁免计费' : 'Admin Exemption'}
                  </span>
                )}
              </div>

              {/* Gateway API Base URL Card */}
              <div className="p-4 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-200/80 dark:border-slate-800 space-y-2">
                <div className="flex items-center justify-between">
                  <span className="text-xs font-bold text-slate-700 dark:text-slate-200 flex items-center space-x-1.5">
                    <Server className="w-3.5 h-3.5 text-indigo-600 dark:text-indigo-400" />
                    <span>{isZh ? '网关 API Base URL' : 'Gateway API Base URL'}</span>
                  </span>
                  <button
                    onClick={handleCopyBaseUrl}
                    className="text-xs text-indigo-600 dark:text-indigo-400 hover:text-indigo-800 dark:hover:text-indigo-300 font-semibold flex items-center space-x-1 transition cursor-pointer"
                  >
                    {copiedBaseUrl ? (
                      <>
                        <Check className="w-3.5 h-3.5 text-emerald-600 dark:text-emerald-400" />
                        <span className="text-emerald-600 dark:text-emerald-400">{isZh ? '已复制' : 'Copied'}</span>
                      </>
                    ) : (
                      <>
                        <Copy className="w-3.5 h-3.5" />
                        <span>{isZh ? '复制地址' : 'Copy URL'}</span>
                      </>
                    )}
                  </button>
                </div>
                <div className="ui-field font-mono break-all select-all">
                  {apiBaseUrl}
                </div>
                <p className="ui-meta leading-normal">
                  {isZh
                    ? '兼容 OpenAI 与 Anthropic 原生规范。复制填入客户端（如 Cursor、Claude Code、NextChat、Cherry Studio）即可直连。'
                    : 'Compatible with OpenAI & Anthropic standards. Paste into Cursor, Claude Code, NextChat, Cherry Studio, etc.'}
                </p>
              </div>
            </div>
          )}

          {activeTab === 'password' && (
            <form onSubmit={handleChangePassword} className="space-y-4">
              {errorMsg && (
                <div className="p-3 rounded-xl bg-rose-50 dark:bg-rose-950/50 border border-rose-200 dark:border-rose-900/60 text-rose-700 dark:text-rose-300 text-xs font-medium">
                  {errorMsg}
                </div>
              )}

              <div>
                <label className="ui-label">{isZh ? '当前旧密码' : 'Current Password'}</label>
                <div className="relative">
                  <input
                    type={showOldPass ? 'text' : 'password'}
                    required
                    value={oldPassword}
                    onChange={(e) => setOldPassword(e.target.value)}
                    placeholder={isZh ? '输入当前使用的登录密码' : 'Enter current password'}
                    className="ui-field pr-10"
                  />
                  <button
                    type="button"
                    onClick={() => setShowOldPass(!showOldPass)}
                    className="absolute right-2 top-1/2 -translate-y-1/2 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 cursor-pointer"
                  >
                    {showOldPass ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                  </button>
                </div>
              </div>

              <div>
                <label className="ui-label">{isZh ? '新密码' : 'New Password'}</label>
                <div className="relative">
                  <input
                    type={showNewPass ? 'text' : 'password'}
                    required
                    minLength={6}
                    value={newPassword}
                    onChange={(e) => setNewPassword(e.target.value)}
                    placeholder={isZh ? '至少 6 个字符' : 'At least 6 characters'}
                    className="ui-field pr-10"
                  />
                  <button
                    type="button"
                    onClick={() => setShowNewPass(!showNewPass)}
                    className="absolute right-2 top-1/2 -translate-y-1/2 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 cursor-pointer"
                  >
                    {showNewPass ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                  </button>
                </div>
              </div>

              <div>
                <label className="ui-label">{isZh ? '确认新密码' : 'Confirm New Password'}</label>
                <input
                  type="password"
                  required
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                  placeholder={isZh ? '再次输入新密码' : 'Re-enter new password'}
                  className="ui-field"
                />
              </div>

              <div className="pt-2 flex justify-end space-x-3">
                <button
                  type="button"
                  onClick={requestClose}
                  className="ui-btn-ghost"
                >
                  {isZh ? '取消' : 'Cancel'}
                </button>
                <button
                  type="submit"
                  disabled={loading}
                  className="ui-btn-primary"
                >
                  {loading ? (isZh ? '正在更新...' : 'Saving...') : (isZh ? '保存新密码' : 'Save New Password')}
                </button>
              </div>
            </form>
          )}
        </div>

        <DiscardChangesDialog
          open={confirming}
          onKeepEditing={keepEditing}
          onDiscard={discard}
          lang={lang}
        />
      </div>
    </div>
  );
}
