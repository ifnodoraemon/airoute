import { UserPlus, RefreshCw, AlertCircle, Eye, EyeOff, Sparkles } from 'lucide-react';
import DiscardChangesDialog from './DiscardChangesDialog';

export default function UserCreateModal({
  addModalError, addUserConfirming, allAvailableGroups, creating, discardAddUser, generateSecurePassword,
  handleCreateUser, isZh, keepEditingAddUser, lang, newBalance, newEmail, newGroupName, newPassword, newRole,
  newStatus, newUsername, requestCloseAddUser, setNewBalance, setNewEmail, setNewGroupName, setNewPassword,
  setNewRole, setNewStatus, setNewUsername, setShowNewPassPlain, showNewPassPlain
}) {
  return (
    <div
      className="fixed inset-0 bg-black/50 flex items-center justify-center p-4 z-50 backdrop-blur-xs animate-in fade-in"
      onClick={(e) => {
        if (e.target === e.currentTarget) requestCloseAddUser();
      }}
    >
      <div className="relative bg-white dark:bg-[#191B1F] border border-[#E4E6E9] dark:border-[#2C3038] rounded-xl p-6 max-w-md w-full shadow-xl space-y-4 animate-in zoom-in-95 duration-150">
        <div className="flex items-center justify-between border-b border-[#E4E6E9] dark:border-[#2C3038] pb-3">
          <div className="flex items-center space-x-2">
            <UserPlus className="w-5 h-5 text-[#355CFF] dark:text-[#91A7FF]" />
            <h3 className="font-semibold text-base text-[#202124] dark:text-[#F1F2F4]">{isZh ? '创建新用户账号' : 'Create User Account'}</h3>
          </div>
          <button
            onClick={() => requestCloseAddUser()}
            className="p-1 hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] rounded-lg text-[#656B75] hover:text-[#202124] dark:text-[#A1A7B0] dark:hover:text-[#F1F2F4] cursor-pointer"
          >
            ✕
          </button>
        </div>

        {addModalError && (
          <div className="p-3 rounded-lg bg-rose-50 dark:bg-rose-950/40 border border-rose-200 dark:border-rose-800 text-rose-800 dark:text-rose-300 text-xs flex items-center space-x-2">
            <AlertCircle className="w-4 h-4 shrink-0 text-rose-600 dark:text-rose-400" />
            <span>{addModalError}</span>
          </div>
        )}

        <form onSubmit={handleCreateUser} className="space-y-3">
          <div>
            <label className="block text-xs font-medium text-[#202124] dark:text-[#F1F2F4] mb-1">{isZh ? '登录用户名 *' : 'Username *'}</label>
            <input
              type="text"
              required
              value={newUsername}
              onChange={(e) => setNewUsername(e.target.value)}
              placeholder={isZh ? '至少 3 个字符' : 'At least 3 characters'}
              className="w-full ui-field text-xs"
            />
          </div>

          <div>
            <label className="block text-xs font-medium text-[#202124] dark:text-[#F1F2F4] mb-1">{isZh ? '绑定邮箱 (选填)' : 'Email (Optional)'}</label>
            <input
              type="email"
              value={newEmail}
              onChange={(e) => setNewEmail(e.target.value)}
              placeholder="user@example.com"
              className="w-full ui-field text-xs"
            />
          </div>

          <div>
            <div className="flex items-center justify-between mb-1">
              <label className="text-xs font-medium text-[#202124] dark:text-[#F1F2F4]">{isZh ? '登录密码 *' : 'Password *'}</label>
              <button
                type="button"
                onClick={() => setNewPassword(generateSecurePassword())}
                className="text-xs text-[#355CFF] dark:text-[#91A7FF] hover:underline flex items-center space-x-1 cursor-pointer"
              >
                <Sparkles className="w-3 h-3" />
                <span>{isZh ? '随机生成' : 'Generate'}</span>
              </button>
            </div>
            <div className="relative">
              <input
                type={showNewPassPlain ? 'text' : 'password'}
                required
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                placeholder={isZh ? '至少 6 个字符' : 'At least 6 characters'}
                className="w-full ui-field pl-3 pr-10 text-xs font-mono"
              />
              <button
                type="button"
                onClick={() => setShowNewPassPlain(!showNewPassPlain)}
                className="absolute right-3 top-2.5 text-[#656B75] hover:text-[#202124] dark:text-[#A1A7B0] dark:hover:text-[#F1F2F4] cursor-pointer"
              >
                {showNewPassPlain ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
              </button>
            </div>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs font-medium text-[#202124] dark:text-[#F1F2F4] mb-1">{isZh ? '权限角色' : 'Role'}</label>
              <select
                value={newRole}
                onChange={(e) => setNewRole(e.target.value)}
                className="w-full ui-field text-xs cursor-pointer"
              >
                <option value="user">{isZh ? '普通用户 (user)' : 'Regular User (user)'}</option>
                <option value="admin">{isZh ? '超级管理员 (admin)' : 'Administrator (admin)'}</option>
              </select>
            </div>

            <div>
              <label className="block text-xs font-medium text-[#202124] dark:text-[#F1F2F4] mb-1">{isZh ? '初始状态' : 'Initial Status'}</label>
              <select
                value={newStatus}
                onChange={(e) => setNewStatus(e.target.value)}
                className="w-full ui-field text-xs cursor-pointer"
              >
                <option value="active">{isZh ? '正常活跃 (active)' : 'Active (active)'}</option>
                <option value="locked">{isZh ? '直接锁定 (locked)' : 'Locked (locked)'}</option>
              </select>
            </div>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs font-medium text-[#202124] dark:text-[#F1F2F4] mb-1">{isZh ? '初始额度 (CNY)' : 'Initial Balance (CNY)'}</label>
              <input
                type="number"
                step="0.01"
                value={newBalance}
                onChange={(e) => setNewBalance(e.target.value)}
                placeholder="10.0"
                className="w-full ui-field font-mono text-xs"
              />
            </div>

            <div>
              <label className="block text-xs font-medium text-[#202124] dark:text-[#F1F2F4] mb-1">{isZh ? '账号保障分组 (Tier)' : 'Account Tier'}</label>
              <div className="space-y-1.5">
                <select
                  value={allAvailableGroups.includes(newGroupName) ? newGroupName : '__custom__'}
                  onChange={(e) => {
                    if (e.target.value !== '__custom__') {
                      setNewGroupName(e.target.value);
                    }
                  }}
                  className="w-full ui-field text-xs cursor-pointer"
                >
                  {allAvailableGroups.map(g => (
                    <option key={g} value={g}>
                      {g === 'default' ? (isZh ? '默认组 (default)' : 'Default (default)') : g === 'vip' ? (isZh ? 'VIP 组 (vip)' : 'VIP (vip)') : g === 'enterprise' ? (isZh ? '企业组 (enterprise)' : 'Enterprise (enterprise)') : `${g} ${isZh ? '组' : 'Group'}`}
                    </option>
                  ))}
                  <option value="__custom__">{isZh ? '自定义输入新分组...' : 'Custom tier...'}</option>
                </select>
                {(!allAvailableGroups.includes(newGroupName) || newGroupName === '') && (
                  <input
                    type="text"
                    placeholder={isZh ? '输入新保障分组标识 (如 partner / dev)' : 'Enter tier ID (e.g. partner / dev)'}
                    value={newGroupName}
                    onChange={(e) => setNewGroupName(e.target.value.toLowerCase().replace(/[^a-z0-9_-]/g, ''))}
                    className="w-full ui-field text-xs font-mono"
                  />
                )}
              </div>
            </div>
          </div>

          <div className="pt-2 flex items-center justify-end space-x-2">
            <button
              type="button"
              onClick={() => requestCloseAddUser()}
              className="ui-btn-secondary ui-btn-sm"
            >
              {isZh ? '取消' : 'Cancel'}
            </button>
            <button
              type="submit"
              disabled={creating}
              className="ui-btn-primary ui-btn-sm disabled:opacity-50"
            >
              {creating ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <span>{isZh ? '确认创建' : 'Create User'}</span>}
            </button>
          </div>
        </form>

        <DiscardChangesDialog
          open={addUserConfirming}
          onKeepEditing={keepEditingAddUser}
          onDiscard={discardAddUser}
          lang={lang}
          title={isZh ? '放弃未保存的用户信息？' : 'Discard this new user?'}
        />
      </div>
    </div>
  );
}
