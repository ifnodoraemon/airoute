import { Key, Trash2, Lock, Unlock, RefreshCw, Coins } from 'lucide-react';

export default function UserTable({
  adminUser, allAvailableGroups, filteredUsers, generateSecurePassword, handleDeleteUser, handleToggleStatus,
  handleUpdateGroup, handleUpdateRole, isZh, loading, setQuotaModalError, setQuotaMode, setQuotaTargetUser,
  setQuotaValue, setResetModalError, setResetOldPassword, setResetPassword, setResetTargetUser, users
}) {
  return (
    <div className="ui-card overflow-hidden">
      <div className="overflow-x-auto">
        <table className="w-full text-left border-collapse text-xs">
          <thead>
            <tr className="border-b border-[#E4E6E9] dark:border-[#2C3038] text-[#656B75] dark:text-[#A1A7B0] uppercase font-semibold bg-[#F0F1F2]/50 dark:bg-[#22252B]/50">
              <th className="ui-th">{isZh ? '用户标识与邮箱' : 'User & Email'}</th>
              <th className="ui-th">{isZh ? '角色' : 'Role'}</th>
              <th className="ui-th">{isZh ? '状态' : 'Status'}</th>
              <th className="ui-th">{isZh ? '钱包余额 / 额度' : 'Balance / Quota'}</th>
              <th className="ui-th" title={isZh ? '管理员为该用户分配的保障等级，用户创建 Key 时可享此等级或默认组' : 'Tier assigned by admin, user keys can enjoy this tier or default'}>{isZh ? '账号保障分组' : 'Account Tier'}</th>
              <th className="ui-th">{isZh ? '注册时间' : 'Registered At'}</th>
              <th className="ui-th text-right">{isZh ? '操作' : 'Actions'}</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[#E4E6E9]/60 dark:divide-[#2C3038]/60">
            {loading && users.length === 0 ? (
              <tr>
                <td colSpan="7" className="py-12 text-center ui-meta">
                  <RefreshCw className="w-5 h-5 animate-spin text-[#355CFF] dark:text-[#91A7FF] mx-auto mb-2" />
                  <span>{isZh ? '正在加载用户数据...' : 'Loading users...'}</span>
                </td>
              </tr>
            ) : filteredUsers.length === 0 ? (
              <tr>
                <td colSpan="7" className="py-12 text-center ui-meta">
                  {isZh ? '未找到匹配的用户账号' : 'No matching user accounts found'}
                </td>
              </tr>
            ) : (
              filteredUsers.map((u) => {
                const isCurrentAdmin = u.username === adminUser?.username;
                const isSysAdmin = u.username === 'admin';
                const isLocked = u.status === 'locked';

                return (
                  <tr key={u.id} className="ui-tr-hover">
                    <td className="ui-td">
                      <div className="flex items-center space-x-2.5">
                        <div className="w-7 h-7 rounded-lg bg-[#F0F1F2] dark:bg-[#22252B] border border-[#E4E6E9] dark:border-[#2C3038] flex items-center justify-center text-[#202124] dark:text-[#F1F2F4] font-semibold shrink-0 uppercase">
                          {u.username.charAt(0)}
                        </div>
                        <div>
                          <div className="font-semibold text-[#202124] dark:text-[#F1F2F4] flex items-center space-x-1.5">
                            <span>{u.username}</span>
                            {isCurrentAdmin && (
                              <span className="text-[10px] px-1.5 py-0.2 bg-[#EEF2FF] dark:bg-[#242D48] text-[#355CFF] dark:text-[#91A7FF] rounded font-semibold">
                                {isZh ? '当前登录' : 'Current'}
                              </span>
                            )}
                          </div>
                          <div className="ui-meta">
                            {u.email || (isZh ? '未绑定邮箱' : 'No email')}
                          </div>
                        </div>
                      </div>
                    </td>

                    <td className="ui-td">
                      <select
                        value={u.role || 'user'}
                        disabled={isSysAdmin || isCurrentAdmin}
                        onChange={(e) => handleUpdateRole(u, e.target.value)}
                        className={`border rounded-lg px-2 py-1 text-xs font-semibold focus:outline-none transition cursor-pointer ${
                          u.role === 'admin'
                            ? 'bg-purple-50 dark:bg-purple-950/50 text-purple-700 dark:text-purple-300 border-purple-200 dark:border-purple-800'
                            : 'bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 border-emerald-200 dark:border-emerald-800'
                        } disabled:opacity-75 disabled:cursor-not-allowed`}
                        title={isSysAdmin ? (isZh ? '系统默认管理员不可修改角色' : 'Cannot change role of system admin') : (isCurrentAdmin ? (isZh ? '当前登录账号不可降级自己' : 'Cannot demote current user') : (isZh ? '点击切换用户角色' : 'Switch role'))}
                      >
                        <option value="user">{isZh ? '普通用户 (user)' : 'Regular User (user)'}</option>
                        <option value="admin">{isZh ? '超级管理员 (admin)' : 'Administrator (admin)'}</option>
                      </select>
                    </td>

                    <td className="ui-td">
                      <span
                        className={`inline-flex items-center px-2 py-0.5 rounded-md text-[10px] font-semibold ${
                          isLocked
                            ? 'bg-rose-100 dark:bg-rose-950/60 text-rose-800 dark:text-rose-300'
                            : 'bg-emerald-100 dark:bg-emerald-950/60 text-emerald-800 dark:text-emerald-300'
                        }`}
                      >
                        {isLocked ? (isZh ? '已锁定 (禁止调用)' : 'Locked') : (isZh ? '正常使用' : 'Active')}
                      </span>
                    </td>

                    <td className="ui-td">
                      <div className="flex items-center space-x-2">
                        {u.role === 'admin' ? (
                          <span className="font-semibold text-[#355CFF] dark:text-[#91A7FF]">{isZh ? '¥ 无限额度' : '¥ Unlimited'}</span>
                        ) : (
                          <span
                            className={`font-mono font-semibold ${
                              u.balance <= 0 ? 'text-rose-600 dark:text-rose-400' : 'text-[#202124] dark:text-[#F1F2F4]'
                            }`}
                          >
                            ¥ {Number(u.balance || 0).toFixed(2)}
                          </span>
                        )}
                        <button
                          onClick={() => {
                            setQuotaTargetUser(u);
                            setQuotaMode('delta');
                            setQuotaValue('50');
                            setQuotaModalError('');
                          }}
                          className="p-1 hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] rounded text-[#656B75] hover:text-[#355CFF] dark:text-[#A1A7B0] dark:hover:text-[#91A7FF] transition"
                          title={isZh ? '调整用户额度' : 'Adjust balance'}
                        >
                          <Coins className="w-3.5 h-3.5" />
                        </button>
                      </div>
                    </td>

                    <td className="ui-td">
                      <select
                        value={u.group_name || 'default'}
                        onChange={(e) => {
                          if (e.target.value === '__custom__') {
                            const custom = window.prompt(isZh ? `请输入用户 [${u.username}] 的新保障分组标识 (如 partner / promo):` : `Enter new tier for [${u.username}]:`);
                            if (custom && custom.trim()) {
                              handleUpdateGroup(u, custom.trim().toLowerCase());
                            }
                          } else {
                            handleUpdateGroup(u, e.target.value);
                          }
                        }}
                        className="ui-field ui-field-sm cursor-pointer"
                      >
                        {allAvailableGroups.map(g => (
                          <option key={g} value={g}>
                            {g === 'default' ? (isZh ? '默认组 (default)' : 'Default (default)') : g === 'vip' ? (isZh ? 'VIP 组 (vip)' : 'VIP (vip)') : g === 'enterprise' ? (isZh ? '企业组 (enterprise)' : 'Enterprise (enterprise)') : `${g} ${isZh ? '组' : 'Group'}`}
                          </option>
                        ))}
                        <option value="__custom__">{isZh ? '+ 输入自定义分组...' : '+ Custom tier...'}</option>
                      </select>
                    </td>

                    <td className="ui-td ui-meta font-mono">
                      {u.created_at || '-'}
                    </td>

                    <td className="ui-td text-right space-x-1">
                      {/* Lock / Unlock Toggle */}
                      <button
                        onClick={() => handleToggleStatus(u)}
                        disabled={isSysAdmin}
                        className={`p-1.5 rounded-lg transition cursor-pointer ${
                          isLocked
                            ? 'bg-rose-50 dark:bg-rose-950/40 text-rose-600 dark:text-rose-400 hover:bg-rose-100 dark:hover:bg-rose-900/50'
                            : 'hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0] hover:text-rose-600 dark:hover:text-rose-400'
                        } disabled:opacity-30`}
                        title={isLocked ? (isZh ? '点击解锁账号' : 'Unlock account') : (isZh ? '点击锁定账号' : 'Lock account')}
                      >
                        {isLocked ? <Lock className="w-3.5 h-3.5" /> : <Unlock className="w-3.5 h-3.5" />}
                      </button>

                      {/* Reset Password */}
                      <button
                        onClick={() => {
                          setResetTargetUser(u);
                          setResetPassword(generateSecurePassword());
                          setResetOldPassword('');
                          setResetModalError('');
                        }}
                        className="p-1.5 hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] rounded-lg text-[#656B75] hover:text-[#355CFF] dark:text-[#A1A7B0] dark:hover:text-[#91A7FF] transition cursor-pointer"
                        title={isZh ? '重置登录密码' : 'Reset password'}
                      >
                        <Key className="w-3.5 h-3.5" />
                      </button>

                      {/* Delete User */}
                      <button
                        onClick={() => handleDeleteUser(u.username)}
                        disabled={isSysAdmin || isCurrentAdmin}
                        className="p-1.5 hover:bg-rose-50 dark:hover:bg-rose-950/40 rounded-lg text-[#656B75] hover:text-rose-600 dark:text-[#A1A7B0] dark:hover:text-rose-400 transition disabled:opacity-30 cursor-pointer"
                        title={isZh ? '删除用户' : 'Delete user'}
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </td>
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
