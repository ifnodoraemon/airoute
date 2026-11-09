import { Key, RefreshCw, Eye, EyeOff, Sparkles } from 'lucide-react';
import DiscardChangesDialog from './DiscardChangesDialog';

export default function UserPasswordModal({
  discardReset, generateSecurePassword, handleResetPassword, isSelfPasswordReset, isZh, keepEditingReset,
  lang, requestCloseReset, resetConfirming, resetModalError, resetOldPassword, resetPassword, resetTargetUser,
  resetting, setResetOldPassword, setResetPassword, setShowResetOldPassPlain, setShowResetPassPlain,
  showResetOldPassPlain, showResetPassPlain
}) {
  return (
    <div
      className="fixed inset-0 bg-black/50 flex items-center justify-center p-4 z-50 backdrop-blur-xs animate-in fade-in"
      onClick={(e) => {
        if (e.target === e.currentTarget) requestCloseReset();
      }}
    >
      <div className="relative bg-white dark:bg-[#191B1F] border border-[#E4E6E9] dark:border-[#2C3038] rounded-xl p-6 max-w-sm w-full shadow-xl space-y-4 animate-in zoom-in-95 duration-150">
        <div className="flex items-center justify-between border-b border-[#E4E6E9] dark:border-[#2C3038] pb-3">
          <div className="flex items-center space-x-2">
            <Key className="w-5 h-5 text-[#355CFF] dark:text-[#91A7FF]" />
            <h3 className="font-semibold text-base text-[#202124] dark:text-[#F1F2F4]">{isZh ? '重置用户密码' : 'Reset User Password'}</h3>
          </div>
          <button
            onClick={() => requestCloseReset()}
            className="p-1 hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] rounded-lg text-[#656B75] hover:text-[#202124] dark:text-[#A1A7B0] dark:hover:text-[#F1F2F4] cursor-pointer"
          >
            ✕
          </button>
        </div>

        <p className="text-xs text-[#656B75] dark:text-[#A1A7B0]">
          {isZh ? '正在重置用户 ' : 'Resetting password for user '}
          <span className="font-semibold text-[#202124] dark:text-[#F1F2F4]">[{resetTargetUser.username}]</span>
          {isZh ? ' 的登录密码。' : '.'}
        </p>
        {isSelfPasswordReset && (
          <p className="text-xs text-amber-700 dark:text-amber-300 rounded-lg border border-amber-500/30 bg-amber-500/10 px-2.5 py-2">
            {isZh
              ? '这是您自己的账号：重置本人密码需要先验证当前登录密码。'
              : 'This is your own account: resetting it requires your current password.'}
          </p>
        )}

        {resetModalError && (
          <div className="p-2.5 rounded-lg bg-rose-50 dark:bg-rose-950/40 border border-rose-200 dark:border-rose-800 text-rose-800 dark:text-rose-300 text-xs">
            {resetModalError}
          </div>
        )}

        <form onSubmit={handleResetPassword} className="space-y-3">
          {isSelfPasswordReset && (
            <div>
              <label className="text-xs font-medium text-[#202124] dark:text-[#F1F2F4]">
                {isZh ? '当前登录密码 *' : 'Current Password *'}
              </label>
              <div className="relative mt-1">
                <input
                  type={showResetOldPassPlain ? 'text' : 'password'}
                  required
                  value={resetOldPassword}
                  onChange={(e) => setResetOldPassword(e.target.value)}
                  placeholder={isZh ? '请输入当前登录密码' : 'Enter your current password'}
                  autoComplete="current-password"
                  className="w-full ui-field pl-3 pr-10 text-xs font-mono"
                />
                <button
                  type="button"
                  onClick={() => setShowResetOldPassPlain(!showResetOldPassPlain)}
                  className="absolute right-3 top-2.5 text-[#656B75] hover:text-[#202124] dark:text-[#A1A7B0] dark:hover:text-[#F1F2F4] cursor-pointer"
                >
                  {showResetOldPassPlain ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                </button>
              </div>
            </div>
          )}
          <div>
            <div className="flex items-center justify-between mb-1">
              <label className="text-xs font-medium text-[#202124] dark:text-[#F1F2F4]">{isZh ? '新密码 *' : 'New Password *'}</label>
              <button
                type="button"
                onClick={() => setResetPassword(generateSecurePassword())}
                className="text-xs text-[#355CFF] dark:text-[#91A7FF] hover:underline flex items-center space-x-1 cursor-pointer"
              >
                <Sparkles className="w-3 h-3" />
                <span>{isZh ? '随机生成' : 'Generate'}</span>
              </button>
            </div>
            <div className="relative">
              <input
                type={showResetPassPlain ? 'text' : 'password'}
                required
                value={resetPassword}
                onChange={(e) => setResetPassword(e.target.value)}
                placeholder={isZh ? '至少 6 个字符' : 'At least 6 characters'}
                className="w-full ui-field pl-3 pr-10 text-xs font-mono"
              />
              <button
                type="button"
                onClick={() => setShowResetPassPlain(!showResetPassPlain)}
                className="absolute right-3 top-2.5 text-[#656B75] hover:text-[#202124] dark:text-[#A1A7B0] dark:hover:text-[#F1F2F4] cursor-pointer"
              >
                {showResetPassPlain ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
              </button>
            </div>
          </div>

          <div className="pt-2 flex items-center justify-end space-x-2">
            <button
              type="button"
              onClick={() => requestCloseReset()}
              className="ui-btn-secondary ui-btn-sm"
            >
              {isZh ? '取消' : 'Cancel'}
            </button>
            <button
              type="submit"
              disabled={resetting}
              className="ui-btn-primary ui-btn-sm disabled:opacity-50"
            >
              {resetting ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <span>{isZh ? '确认重置' : 'Reset Password'}</span>}
            </button>
          </div>
        </form>

        <DiscardChangesDialog
          open={resetConfirming}
          onKeepEditing={keepEditingReset}
          onDiscard={discardReset}
          lang={lang}
          title={isZh ? '放弃未保存的密码？' : 'Discard this password?'}
        />
      </div>
    </div>
  );
}
