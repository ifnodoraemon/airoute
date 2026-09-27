import React, { useState, useEffect } from 'react';
import {
  X,
  Lock,
  User,
  Users,
  AlertCircle,
  RefreshCw,
  CheckCircle2,
  Trash2,
  Plus,
  Shield,
  Key,
  Eye,
  EyeOff
} from 'lucide-react';

export default function AccountManageModal({
  isOpen,
  onClose,
  adminUser,
  adminToken,
  adminFetch,
  showToast,
  onPasswordChanged
}) {
  if (!isOpen) return null;

  const [activeTab, setActiveTab] = useState('accounts'); // 'accounts' | 'password'

  // Users list state
  const [users, setUsers] = useState([]);
  const [usersLoading, setUsersLoading] = useState(false);
  const [showAddUser, setShowAddUser] = useState(false);
  const [newUsername, setNewUsername] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [newRole, setNewRole] = useState('operator');
  const [creatingUser, setCreatingUser] = useState(false);

  // Admin reset another user's password
  const [resetTargetUser, setResetTargetUser] = useState(null);
  const [resetNewPass, setResetNewPass] = useState('');
  const [resetting, setResetting] = useState(false);

  // Change own password state
  const [oldPassword, setOldPassword] = useState('');
  const [newPass, setNewPass] = useState('');
  const [confirmPass, setConfirmPass] = useState('');
  const [showOldPass, setShowOldPass] = useState(false);
  const [showNewPass, setShowNewPass] = useState(false);
  const [passLoading, setPassLoading] = useState(false);
  const [passError, setPassError] = useState('');
  const [passSuccess, setPassSuccess] = useState(false);

  const fetchUsers = async () => {
    setUsersLoading(true);
    try {
      const res = await adminFetch('/api/v1/admin/users');
      const data = await res.json();
      if (res.ok && data.code === 0) {
        setUsers(data.data || []);
      }
    } catch (err) {
      console.error('Fetch users error:', err);
    } finally {
      setUsersLoading(false);
    }
  };

  useEffect(() => {
    if (isOpen) {
      fetchUsers();
      setPassError('');
      setPassSuccess(false);
      setOldPassword('');
      setNewPass('');
      setConfirmPass('');
      setResetTargetUser(null);
    }
  }, [isOpen]);

  const handleCreateUser = async (e) => {
    e.preventDefault();
    if (!newUsername.trim() || !newPassword.trim()) {
      showToast('用户名和密码不能为空', 'warning');
      return;
    }
    setCreatingUser(true);
    try {
      const res = await adminFetch('/api/v1/admin/users', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          username: newUsername.trim(),
          password: newPassword,
          role: newRole
        })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(`已创建账号: ${newUsername}`, 'success');
        setNewUsername('');
        setNewPassword('');
        setShowAddUser(false);
        fetchUsers();
      } else {
        showToast(data.error || '创建账号失败', 'error');
      }
    } catch (err) {
      showToast('网络请求异常: ' + err.message, 'error');
    } finally {
      setCreatingUser(false);
    }
  };

  const handleDeleteUser = async (username) => {
    if (!window.confirm(`确定删除账号 [${username}] 吗？`)) {
      return;
    }
    try {
      const res = await adminFetch(`/api/v1/admin/users/${encodeURIComponent(username)}`, {
        method: 'DELETE'
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(`账号 [${username}] 已删除`, 'success');
        fetchUsers();
      } else {
        showToast(data.error || '删除失败', 'warning');
      }
    } catch (err) {
      showToast('请求异常: ' + err.message, 'error');
    }
  };

  const handleResetPassword = async (e) => {
    e.preventDefault();
    if (!resetNewPass || resetNewPass.length < 6) {
      showToast('新密码长度不能少于 6 个字符', 'warning');
      return;
    }
    setResetting(true);
    try {
      const res = await adminFetch(`/api/v1/admin/users/${encodeURIComponent(resetTargetUser)}/password`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ new_password: resetNewPass })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(`已成功重置账号 [${resetTargetUser}] 的密码`, 'success');
        setResetTargetUser(null);
        setResetNewPass('');
      } else {
        showToast(data.error || '重置密码失败', 'error');
      }
    } catch (err) {
      showToast('网络请求异常: ' + err.message, 'error');
    } finally {
      setResetting(false);
    }
  };

  const handleChangePassword = async (e) => {
    e.preventDefault();
    setPassError('');

    if (newPass !== confirmPass) {
      setPassError('两次输入的新密码不一致');
      return;
    }
    if (newPass.length < 6) {
      setPassError('新密码长度不能少于 6 个字符');
      return;
    }

    setPassLoading(true);
    try {
      const res = await adminFetch('/api/v1/admin/auth/password', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          old_password: oldPassword,
          new_password: newPass
        })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        setPassSuccess(true);
        showToast('密码修改成功，请牢记新密码', 'success');
        if (onPasswordChanged) onPasswordChanged();
        setTimeout(() => {
          onClose();
        }, 1200);
      } else {
        setPassError(data.error || '修改密码失败');
      }
    } catch (err) {
      setPassError('请求异常: ' + err.message);
    } finally {
      setPassLoading(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-slate-900/60 flex items-center justify-center p-4 z-50 backdrop-blur-sm animate-in fade-in">
      <div className="bg-white border border-slate-200 rounded-3xl p-6 sm:p-8 max-w-xl w-full shadow-2xl space-y-5 animate-in zoom-in-95 duration-150 max-h-[85vh] overflow-y-auto">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-slate-100 pb-3">
          <div className="flex items-center space-x-2.5">
            <div className="w-10 h-10 rounded-2xl bg-indigo-50 border border-indigo-200 flex items-center justify-center text-indigo-600 font-bold">
              <Shield className="w-5 h-5" />
            </div>
            <div>
              <h3 className="font-bold text-base text-slate-900 tracking-tight">
                账号与安全管理
              </h3>
              <p className="text-xs text-slate-500">
                当前登录: <strong className="text-slate-800">{adminUser?.username || 'admin'}</strong> ({adminUser?.role === 'admin' ? '超级管理员' : '操作员'})
              </p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 hover:bg-slate-100 rounded-xl text-slate-400 hover:text-slate-600 transition cursor-pointer"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Tab switcher */}
        <div className="flex items-center space-x-2 bg-slate-100 p-1 rounded-2xl border border-slate-200/80 text-xs">
          <button
            type="button"
            onClick={() => setActiveTab('accounts')}
            className={`flex-1 py-1.5 rounded-xl font-semibold transition flex items-center justify-center space-x-1.5 cursor-pointer ${
              activeTab === 'accounts'
                ? 'bg-white text-indigo-600 shadow-xs'
                : 'text-slate-600 hover:text-slate-900'
            }`}
          >
            <Users className="w-3.5 h-3.5" />
            <span>账号列表与角色 ({users.length})</span>
          </button>
          <button
            type="button"
            onClick={() => setActiveTab('password')}
            className={`flex-1 py-1.5 rounded-xl font-semibold transition flex items-center justify-center space-x-1.5 cursor-pointer ${
              activeTab === 'password'
                ? 'bg-white text-indigo-600 shadow-xs'
                : 'text-slate-600 hover:text-slate-900'
            }`}
          >
            <Lock className="w-3.5 h-3.5" />
            <span>修改当前密码</span>
          </button>
        </div>

        {/* Tab 1: Accounts List */}
        {activeTab === 'accounts' && (
          <div className="space-y-4 text-xs animate-in fade-in">
            <div className="flex items-center justify-between">
              <span className="font-semibold text-slate-700">系统用户列表</span>
              {adminUser?.role === 'admin' && (
                <button
                  type="button"
                  onClick={() => setShowAddUser(!showAddUser)}
                  className="px-3 py-1 bg-indigo-50 hover:bg-indigo-100 text-indigo-700 rounded-xl font-semibold border border-indigo-200 transition flex items-center space-x-1 cursor-pointer"
                >
                  <Plus className="w-3 h-3" />
                  <span>{showAddUser ? '取消' : '添加账号'}</span>
                </button>
              )}
            </div>

            {/* Inline reset password modal / card */}
            {resetTargetUser && (
              <form onSubmit={handleResetPassword} className="p-4 bg-indigo-50/70 border border-indigo-200 rounded-2xl space-y-3 animate-in fade-in">
                <div className="flex items-center justify-between">
                  <span className="font-bold text-indigo-900 block text-xs">
                    重置用户 [{resetTargetUser}] 的密码
                  </span>
                  <button
                    type="button"
                    onClick={() => setResetTargetUser(null)}
                    className="text-slate-400 hover:text-slate-600 cursor-pointer"
                  >
                    <X className="w-4 h-4" />
                  </button>
                </div>
                <div className="flex items-center space-x-2">
                  <input
                    type="password"
                    required
                    placeholder="输入该账号的新密码 (至少 6 位)"
                    value={resetNewPass}
                    onChange={(e) => setResetNewPass(e.target.value)}
                    className="flex-1 bg-white border border-indigo-200 rounded-xl px-3 py-1.5 text-xs text-slate-900 focus:outline-none focus:border-indigo-500"
                  />
                  <button
                    type="submit"
                    disabled={resetting}
                    className="px-4 py-1.5 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl font-bold transition flex items-center space-x-1 disabled:opacity-50 cursor-pointer"
                  >
                    {resetting ? <RefreshCw className="w-3 h-3 animate-spin" /> : <span>确认重置</span>}
                  </button>
                </div>
              </form>
            )}

            {/* Add user form */}
            {showAddUser && (
              <form onSubmit={handleCreateUser} className="p-4 bg-slate-50 border border-slate-200 rounded-2xl space-y-3 animate-in fade-in">
                <span className="font-bold text-slate-800 block text-xs">
                  新建管理员或操作员
                </span>
                <div className="grid grid-cols-1 sm:grid-cols-3 gap-2.5">
                  <div>
                    <label className="block text-slate-600 font-medium mb-1">
                      账号用户名
                    </label>
                    <input
                      type="text"
                      required
                      placeholder="如 operator_ops"
                      value={newUsername}
                      onChange={(e) => setNewUsername(e.target.value)}
                      className="w-full bg-white border border-slate-200 rounded-xl px-2.5 py-1.5 text-xs text-slate-800 focus:outline-none focus:border-indigo-500"
                    />
                  </div>
                  <div>
                    <label className="block text-slate-600 font-medium mb-1">
                      初始密码 (≥6位)
                    </label>
                    <input
                      type="password"
                      required
                      placeholder="密码"
                      value={newPassword}
                      onChange={(e) => setNewPassword(e.target.value)}
                      className="w-full bg-white border border-slate-200 rounded-xl px-2.5 py-1.5 text-xs text-slate-800 focus:outline-none focus:border-indigo-500"
                    />
                  </div>
                  <div>
                    <label className="block text-slate-600 font-medium mb-1">
                      系统角色
                    </label>
                    <select
                      value={newRole}
                      onChange={(e) => setNewRole(e.target.value)}
                      className="w-full bg-white border border-slate-200 rounded-xl px-2 py-1.5 text-xs text-slate-800 focus:outline-none focus:border-indigo-500"
                    >
                      <option value="operator">操作员 (可查阅/调用)</option>
                      <option value="admin">超级管理员 (全部管理权限)</option>
                    </select>
                  </div>
                </div>
                <div className="flex justify-end">
                  <button
                    type="submit"
                    disabled={creatingUser}
                    className="px-4 py-1.5 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl font-bold transition flex items-center space-x-1 disabled:opacity-50 cursor-pointer"
                  >
                    {creatingUser ? <RefreshCw className="w-3 h-3 animate-spin" /> : <Plus className="w-3 h-3" />}
                    <span>确认创建账号</span>
                  </button>
                </div>
              </form>
            )}

            {/* Users table */}
            <div className="rounded-2xl border border-slate-200 overflow-hidden">
              <table className="w-full text-left border-collapse text-xs">
                <thead>
                  <tr className="border-b border-slate-200 text-slate-500 bg-slate-50/80">
                    <th className="py-2.5 px-4 font-semibold">用户名</th>
                    <th className="py-2.5 px-3 font-semibold">角色</th>
                    <th className="py-2.5 px-3 font-semibold">创建时间</th>
                    <th className="py-2.5 px-4 text-right font-semibold">操作</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100">
                  {users.map((u) => {
                    const isSelf = u.username === adminUser?.username;
                    const isDefaultAdmin = u.username === 'admin';

                    return (
                      <tr key={u.id} className="hover:bg-slate-50/60 transition">
                        <td className="py-3 px-4 font-bold text-slate-900 font-mono">
                          {u.username}
                          {isSelf && (
                            <span className="ml-1.5 text-[10px] text-indigo-600 bg-indigo-50 px-1.5 py-0.5 rounded font-sans">当前</span>
                          )}
                        </td>
                        <td className="py-3 px-3">
                          <span className={`px-2 py-0.5 rounded-md text-[10px] font-semibold ${
                            u.role === 'admin'
                              ? 'bg-purple-50 text-purple-700 border border-purple-200'
                              : 'bg-slate-100 text-slate-600'
                          }`}>
                            {u.role === 'admin' ? '超级管理员' : '操作员'}
                          </span>
                        </td>
                        <td className="py-3 px-3 text-slate-500 font-sans">
                          {u.created_at || '-'}
                        </td>
                        <td className="py-3 px-4 text-right space-x-2">
                          {adminUser?.role === 'admin' && (
                            <button
                              type="button"
                              onClick={() => {
                                setResetTargetUser(u.username);
                                setResetNewPass('');
                              }}
                              className="text-indigo-600 hover:text-indigo-800 text-xs font-medium transition cursor-pointer"
                            >
                              重置密码
                            </button>
                          )}
                          {!(isDefaultAdmin || isSelf) && adminUser?.role === 'admin' && (
                            <button
                              type="button"
                              onClick={() => handleDeleteUser(u.username)}
                              className="text-rose-600 hover:text-rose-800 text-xs font-semibold transition cursor-pointer"
                            >
                              删除
                            </button>
                          )}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </div>
        )}

        {/* Tab 2: Change Password */}
        {activeTab === 'password' && (
          <form onSubmit={handleChangePassword} className="space-y-4 text-xs animate-in fade-in">
            {passError && (
              <div className="p-3 rounded-2xl bg-rose-50 border border-rose-200 text-rose-800 flex items-center space-x-2">
                <AlertCircle className="w-4 h-4 shrink-0 text-rose-600" />
                <span>{passError}</span>
              </div>
            )}

            {passSuccess && (
              <div className="p-3 rounded-2xl bg-emerald-50 border border-emerald-200 text-emerald-800 flex items-center space-x-2">
                <CheckCircle2 className="w-4 h-4 shrink-0 text-emerald-600" />
                <span>密码修改成功！</span>
              </div>
            )}

            <div>
              <label className="block font-semibold text-slate-700 mb-1">
                当前旧密码 <span className="text-rose-500">*</span>
              </label>
              <div className="relative">
                <input
                  required
                  type={showOldPass ? 'text' : 'password'}
                  value={oldPassword}
                  onChange={(e) => setOldPassword(e.target.value)}
                  placeholder="请输入当前账号的旧密码"
                  className="w-full bg-slate-50 border border-slate-200 rounded-xl px-3 py-2 text-xs text-slate-800 focus:outline-none focus:border-indigo-500 pr-9"
                />
                <button
                  type="button"
                  onClick={() => setShowOldPass(!showOldPass)}
                  className="absolute right-2.5 top-2.5 text-slate-400 hover:text-slate-600 cursor-pointer"
                >
                  {showOldPass ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
                </button>
              </div>
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <div>
                <label className="block font-semibold text-slate-700 mb-1">
                  新密码 (≥6位) <span className="text-rose-500">*</span>
                </label>
                <div className="relative">
                  <input
                    required
                    type={showNewPass ? 'text' : 'password'}
                    value={newPass}
                    onChange={(e) => setNewPass(e.target.value)}
                    placeholder="输入新密码"
                    className="w-full bg-slate-50 border border-slate-200 rounded-xl px-3 py-2 text-xs text-slate-800 focus:outline-none focus:border-indigo-500 pr-9"
                  />
                  <button
                    type="button"
                    onClick={() => setShowNewPass(!showNewPass)}
                    className="absolute right-2.5 top-2.5 text-slate-400 hover:text-slate-600 cursor-pointer"
                  >
                    {showNewPass ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
                  </button>
                </div>
              </div>

              <div>
                <label className="block font-semibold text-slate-700 mb-1">
                  确认新密码 <span className="text-rose-500">*</span>
                </label>
                <input
                  required
                  type={showNewPass ? 'text' : 'password'}
                  value={confirmPass}
                  onChange={(e) => setConfirmPass(e.target.value)}
                  placeholder="再次输入新密码"
                  className="w-full bg-slate-50 border border-slate-200 rounded-xl px-3 py-2 text-xs text-slate-800 focus:outline-none focus:border-indigo-500"
                />
              </div>
            </div>

            <div className="pt-2 flex justify-end space-x-2.5">
              <button
                type="button"
                onClick={onClose}
                className="px-4 py-2 rounded-xl bg-slate-100 text-slate-700 hover:bg-slate-200 transition font-medium"
              >
                取消
              </button>
              <button
                type="submit"
                disabled={passLoading}
                className="px-5 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl font-bold transition flex items-center space-x-1.5 disabled:opacity-50 cursor-pointer shadow-sm"
              >
                {passLoading ? (
                  <>
                    <RefreshCw className="w-3.5 h-3.5 animate-spin" />
                    <span>正在保存...</span>
                  </>
                ) : (
                  <span>确认修改密码</span>
                )}
              </button>
            </div>
          </form>
        )}
      </div>
    </div>
  );
}
