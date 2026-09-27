import React, { useState, useEffect } from 'react';
import {
  Users,
  UserPlus,
  Shield,
  ShieldCheck,
  Key,
  Trash2,
  Lock,
  RefreshCw,
  Search,
  CheckCircle2,
  AlertCircle,
  Eye,
  EyeOff,
  UserCheck,
  X,
  Sparkles,
  Copy
} from 'lucide-react';

export default function UserManagementView({ adminUser, adminToken, adminFetch, showToast }) {
  const [users, setUsers] = useState([]);
  const [loading, setLoading] = useState(false);
  const [searchFilter, setSearchFilter] = useState('');
  const [roleFilter, setRoleFilter] = useState('all');

  // Modals
  const [showAddModal, setShowAddModal] = useState(false);
  const [newUsername, setNewUsername] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [newRole, setNewRole] = useState('operator');
  const [creating, setCreating] = useState(false);
  const [addModalError, setAddModalError] = useState('');

  // Reset Password Modal
  const [resetTargetUser, setResetTargetUser] = useState(null);
  const [resetPassword, setResetPassword] = useState('');
  const [resetting, setResetting] = useState(false);
  const [resetModalError, setResetModalError] = useState('');

  // Change Self Password Modal
  const [showChangeSelfPass, setShowChangeSelfPass] = useState(false);
  const [oldSelfPassword, setOldSelfPassword] = useState('');
  const [newSelfPassword, setNewSelfPassword] = useState('');
  const [confirmSelfPassword, setConfirmSelfPassword] = useState('');
  const [changingSelfPass, setChangingSelfPass] = useState(false);
  const [selfPassError, setSelfPassError] = useState('');

  // Password visibility toggles
  const [showNewPassPlain, setShowNewPassPlain] = useState(false);
  const [showResetPassPlain, setShowResetPassPlain] = useState(false);

  const generateSecurePassword = () => {
    const chars = 'abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789!@#$';
    let pass = '';
    for (let i = 0; i < 12; i++) {
      pass += chars.charAt(Math.floor(Math.random() * chars.length));
    }
    return pass;
  };

  const fetchUsers = async () => {
    if (!adminFetch) return;
    setLoading(true);
    try {
      const res = await adminFetch('/api/v1/admin/users');
      const data = await res.json();
      if (res.ok && data.code === 0) {
        setUsers(data.data || []);
      } else {
        if (showToast) showToast(data.error || '获取用户列表失败', 'error');
      }
    } catch (err) {
      if (showToast) showToast('请求异常: ' + err.message, 'error');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchUsers();
  }, []);

  const handleCreateUser = async (e) => {
    if (e && e.preventDefault) e.preventDefault();
    setAddModalError('');
    if (!newUsername.trim()) {
      setAddModalError('请输入登录用户名');
      if (showToast) showToast('请输入登录用户名', 'warning');
      return;
    }
    if (newUsername.trim().length < 3) {
      setAddModalError('用户名长度至少需要 3 个字符');
      if (showToast) showToast('用户名至少需要 3 个字符', 'warning');
      return;
    }
    if (!newPassword.trim()) {
      setAddModalError('请输入初始登录密码');
      if (showToast) showToast('请输入初始登录密码', 'warning');
      return;
    }
    if (newPassword.length < 6) {
      setAddModalError('密码长度至少需要 6 个字符');
      if (showToast) showToast('密码长度至少需要 6 个字符', 'warning');
      return;
    }

    setCreating(true);
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
        if (showToast) showToast(`账号 [${newUsername}] 创建成功`, 'success');
        setShowAddModal(false);
        setNewUsername('');
        setNewPassword('');
        setNewRole('operator');
        setAddModalError('');
        fetchUsers();
      } else {
        const errMsg = data.error || '创建账号失败';
        setAddModalError(errMsg);
        if (showToast) showToast(errMsg, 'error');
      }
    } catch (err) {
      const errMsg = '网络异常: ' + err.message;
      setAddModalError(errMsg);
      if (showToast) showToast(errMsg, 'error');
    } finally {
      setCreating(false);
    }
  };

  const handleResetPassword = async (e) => {
    if (e && e.preventDefault) e.preventDefault();
    setResetModalError('');
    if (!resetTargetUser) return;
    if (!resetPassword.trim()) {
      setResetModalError('请输入新密码');
      if (showToast) showToast('请输入新密码', 'warning');
      return;
    }
    if (resetPassword.length < 6) {
      setResetModalError('新密码长度至少需要 6 个字符');
      if (showToast) showToast('新密码长度至少需要 6 个字符', 'warning');
      return;
    }

    setResetting(true);
    try {
      const res = await adminFetch(`/api/v1/admin/users/${resetTargetUser.username}/password`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ new_password: resetPassword })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        if (showToast) showToast(`账号 [${resetTargetUser.username}] 密码已成功重置`, 'success');
        setResetTargetUser(null);
        setResetPassword('');
        setResetModalError('');
      } else {
        const errMsg = data.error || '重置密码失败';
        setResetModalError(errMsg);
        if (showToast) showToast(errMsg, 'error');
      }
    } catch (err) {
      const errMsg = '请求异常: ' + err.message;
      setResetModalError(errMsg);
      if (showToast) showToast(errMsg, 'error');
    } finally {
      setResetting(false);
    }
  };

  const handleDeleteUser = async (targetUsername) => {
    if (targetUsername === 'admin') {
      if (showToast) showToast('系统默认主管理员账号 (admin) 不允许删除', 'warning');
      return;
    }
    if (adminUser && targetUsername === adminUser.username) {
      if (showToast) showToast('无法删除当前正在登录的账号', 'warning');
      return;
    }
    if (!window.confirm(`确定要彻底删除账号 [${targetUsername}] 吗？此操作不可恢复。`)) {
      return;
    }

    try {
      const res = await adminFetch(`/api/v1/admin/users/${targetUsername}`, {
        method: 'DELETE'
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        if (showToast) showToast(`账号 [${targetUsername}] 已成功删除`, 'success');
        fetchUsers();
      } else {
        if (showToast) showToast(data.error || '删除账号失败', 'error');
      }
    } catch (err) {
      if (showToast) showToast('请求异常: ' + err.message, 'error');
    }
  };

  const handleChangeSelfPassword = async (e) => {
    if (e && e.preventDefault) e.preventDefault();
    setSelfPassError('');
    if (!oldSelfPassword || !newSelfPassword) {
      setSelfPassError('旧密码与新密码均不能为空');
      if (showToast) showToast('旧密码与新密码均不能为空', 'warning');
      return;
    }
    if (newSelfPassword.length < 6) {
      setSelfPassError('新密码长度至少需要 6 个字符');
      if (showToast) showToast('新密码长度至少需要 6 个字符', 'warning');
      return;
    }
    if (newSelfPassword !== confirmSelfPassword) {
      setSelfPassError('两次输入的新密码不一致');
      if (showToast) showToast('两次输入的新密码不一致', 'warning');
      return;
    }

    setChangingSelfPass(true);
    try {
      const res = await adminFetch('/api/v1/admin/auth/password', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          old_password: oldSelfPassword,
          new_password: newSelfPassword
        })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        if (showToast) showToast('个人登录密码修改成功，请谨记新密码', 'success');
        setShowChangeSelfPass(false);
        setOldSelfPassword('');
        setNewSelfPassword('');
        setConfirmSelfPassword('');
        setSelfPassError('');
      } else {
        const errMsg = data.error || '修改密码失败';
        setSelfPassError(errMsg);
        if (showToast) showToast(errMsg, 'error');
      }
    } catch (err) {
      const errMsg = '网络错误: ' + err.message;
      setSelfPassError(errMsg);
      if (showToast) showToast(errMsg, 'error');
    } finally {
      setChangingSelfPass(false);
    }
  };

  const filteredUsers = users.filter((u) => {
    if (roleFilter !== 'all' && u.role !== roleFilter) return false;
    if (searchFilter.trim()) {
      const q = searchFilter.toLowerCase();
      return u.username.toLowerCase().includes(q) || u.role.toLowerCase().includes(q);
    }
    return true;
  });

  const totalUsers = users.length;
  const adminCount = users.filter(u => u.role === 'admin').length;
  const operatorCount = users.filter(u => u.role !== 'admin').length;

  return (
    <div className="space-y-8 animate-in fade-in duration-200">
      {/* 1. Header */}
      <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-slate-900 tracking-tight flex items-center space-x-3">
            <ShieldCheck className="w-7 h-7 text-indigo-600" />
            <span>用户管理</span>
          </h1>
        </div>

        <div className="flex items-center space-x-3">
          <button
            onClick={() => {
              setSelfPassError('');
              setShowChangeSelfPass(true);
            }}
            className="flex items-center space-x-2 px-4 py-2.5 rounded-xl border border-slate-200 hover:border-indigo-300 bg-white hover:bg-slate-50 text-slate-700 text-sm font-medium shadow-xs transition cursor-pointer"
          >
            <Key className="w-4 h-4 text-slate-500" />
            <span>修改我的密码</span>
          </button>

          <button
            onClick={() => {
              setAddModalError('');
              setShowAddModal(true);
            }}
            className="flex items-center space-x-2 px-5 py-2.5 rounded-xl bg-indigo-600 hover:bg-indigo-700 text-white text-sm font-semibold shadow-xs hover:shadow transition cursor-pointer"
          >
            <UserPlus className="w-4 h-4" />
            <span>新建账号</span>
          </button>
        </div>
      </div>

      {/* 2. Top Metric Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-5">
        <div className="p-6 rounded-2xl bg-white border border-slate-100 shadow-xs flex items-center justify-between">
          <div>
            <div className="text-xs font-semibold uppercase tracking-wider text-slate-400">总账号数</div>
            <div className="text-3xl font-extrabold text-slate-900 mt-2">{totalUsers}</div>
          </div>
          <div className="w-12 h-12 rounded-2xl bg-indigo-50 border border-indigo-100 flex items-center justify-center text-indigo-600">
            <Users className="w-6 h-6" />
          </div>
        </div>

        <div className="p-6 rounded-2xl bg-white border border-slate-100 shadow-xs flex items-center justify-between">
          <div>
            <div className="text-xs font-semibold uppercase tracking-wider text-slate-400">超级管理员</div>
            <div className="text-3xl font-extrabold text-purple-700 mt-2">{adminCount}</div>
          </div>
          <div className="w-12 h-12 rounded-2xl bg-purple-50 border border-purple-100 flex items-center justify-center text-purple-600">
            <Shield className="w-6 h-6" />
          </div>
        </div>

        <div className="p-6 rounded-2xl bg-white border border-slate-100 shadow-xs flex items-center justify-between">
          <div>
            <div className="text-xs font-semibold uppercase tracking-wider text-slate-400">运维操作员</div>
            <div className="text-3xl font-extrabold text-sky-700 mt-2">{operatorCount}</div>
          </div>
          <div className="w-12 h-12 rounded-2xl bg-sky-50 border border-sky-100 flex items-center justify-center text-sky-600">
            <UserCheck className="w-6 h-6" />
          </div>
        </div>

        <div className="p-6 rounded-2xl bg-white border border-slate-100 shadow-xs flex items-center justify-between">
          <div>
            <div className="text-xs font-semibold uppercase tracking-wider text-slate-400">当前身份</div>
            <div className="text-lg font-bold text-slate-800 mt-2 truncate max-w-[120px]">
              {adminUser?.username || '未登录'}
            </div>
            <div className="text-xs font-medium text-emerald-600 mt-0.5">
              {adminUser?.role === 'admin' ? '全部权限 (Admin)' : '运维操作 (Operator)'}
            </div>
          </div>
          <div className="w-12 h-12 rounded-2xl bg-emerald-50 border border-emerald-100 flex items-center justify-center text-emerald-600">
            <Lock className="w-6 h-6" />
          </div>
        </div>
      </div>

      {/* 3. Filter & Toolbar */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 bg-white p-5 rounded-2xl border border-slate-100 shadow-xs">
        <div className="flex items-center space-x-3 flex-1 max-w-md">
          <div className="relative w-full">
            <Search className="w-4 h-4 text-slate-400 absolute left-3.5 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              value={searchFilter}
              onChange={(e) => setSearchFilter(e.target.value)}
              placeholder="搜索账号用户名或角色..."
              className="w-full pl-9 pr-4 py-2 bg-slate-50 border border-slate-200 rounded-xl text-sm text-slate-800 placeholder-slate-400 focus:outline-hidden focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 transition"
            />
          </div>
        </div>

        <div className="flex items-center space-x-3">
          <div className="flex items-center space-x-1 bg-slate-100/80 p-1 rounded-xl text-xs font-medium">
            <button
              onClick={() => setRoleFilter('all')}
              className={`px-3 py-1.5 rounded-lg transition cursor-pointer ${
                roleFilter === 'all'
                  ? 'bg-white text-slate-900 shadow-xs font-semibold'
                  : 'text-slate-600 hover:text-slate-900'
              }`}
            >
              全部
            </button>
            <button
              onClick={() => setRoleFilter('admin')}
              className={`px-3 py-1.5 rounded-lg transition cursor-pointer ${
                roleFilter === 'admin'
                  ? 'bg-white text-purple-700 shadow-xs font-semibold'
                  : 'text-slate-600 hover:text-slate-900'
              }`}
            >
              管理员
            </button>
            <button
              onClick={() => setRoleFilter('operator')}
              className={`px-3 py-1.5 rounded-lg transition cursor-pointer ${
                roleFilter === 'operator'
                  ? 'bg-white text-sky-700 shadow-xs font-semibold'
                  : 'text-slate-600 hover:text-slate-900'
              }`}
            >
              操作员
            </button>
          </div>

          <button
            onClick={fetchUsers}
            disabled={loading}
            className="p-2 text-slate-500 hover:text-indigo-600 hover:bg-slate-100 rounded-xl transition cursor-pointer"
            title="刷新用户列表"
          >
            <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin text-indigo-600' : ''}`} />
          </button>
        </div>
      </div>

      {/* 4. User Table */}
      <div className="bg-white rounded-2xl border border-slate-100 shadow-xs overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-left border-collapse">
            <thead>
              <tr className="border-b border-slate-100 text-slate-400 text-xs uppercase font-semibold bg-slate-50/50">
                <th className="py-4 px-6">用户名</th>
                <th className="py-4 px-6">权限角色</th>
                <th className="py-4 px-6">创建时间</th>
                <th className="py-4 px-6">更新时间</th>
                <th className="py-4 px-6 text-right">操作</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 text-sm">
              {loading && users.length === 0 ? (
                <tr>
                  <td colSpan="5" className="py-12 text-center text-slate-400">
                    <div className="flex items-center justify-center space-x-2">
                      <RefreshCw className="w-4 h-4 animate-spin text-indigo-600" />
                      <span>正在加载用户数据...</span>
                    </div>
                  </td>
                </tr>
              ) : filteredUsers.length === 0 ? (
                <tr>
                  <td colSpan="5" className="py-12 text-center text-slate-400">
                    未找到符合条件的用户账号
                  </td>
                </tr>
              ) : (
                filteredUsers.map((u) => {
                  const isCurrent = adminUser && adminUser.username === u.username;
                  const isMasterAdmin = u.username === 'admin';
                  return (
                    <tr key={u.id} className="hover:bg-slate-50/60 transition-colors">
                      <td className="py-4 px-6 font-semibold text-slate-900 flex items-center space-x-3">
                        <div className="w-9 h-9 rounded-xl bg-slate-100 text-slate-600 flex items-center justify-center font-bold text-sm border border-slate-200/60">
                          {u.username.substring(0, 2).toUpperCase()}
                        </div>
                        <div>
                          <div className="flex items-center space-x-2">
                            <span>{u.username}</span>
                            {isCurrent && (
                              <span className="text-[10px] font-semibold text-emerald-700 bg-emerald-50 border border-emerald-200 px-1.5 py-0.5 rounded-md">
                                当前登录
                              </span>
                            )}
                          </div>
                        </div>
                      </td>
                      <td className="py-4 px-6">
                        {u.role === 'admin' ? (
                          <span className="inline-flex items-center space-x-1.5 px-2.5 py-1 rounded-full text-xs font-semibold bg-purple-50 text-purple-700 border border-purple-200">
                            <Shield className="w-3 h-3 text-purple-600" />
                            <span>超级管理员</span>
                          </span>
                        ) : (
                          <span className="inline-flex items-center space-x-1.5 px-2.5 py-1 rounded-full text-xs font-semibold bg-sky-50 text-sky-700 border border-sky-200">
                            <UserCheck className="w-3 h-3 text-sky-600" />
                            <span>运维操作员</span>
                          </span>
                        )}
                      </td>
                      <td className="py-4 px-6 text-slate-500 text-xs">
                        {u.created_at || '—'}
                      </td>
                      <td className="py-4 px-6 text-slate-500 text-xs">
                        {u.updated_at || '—'}
                      </td>
                      <td className="py-4 px-6 text-right space-x-2">
                        <button
                          onClick={() => {
                            setResetTargetUser(u);
                            setResetPassword('');
                          }}
                          className="px-3 py-1.5 rounded-lg border border-slate-200 text-xs font-medium text-slate-700 hover:text-indigo-600 hover:border-indigo-300 hover:bg-slate-50 transition cursor-pointer"
                        >
                          重置密码
                        </button>

                        <button
                          onClick={() => handleDeleteUser(u.username)}
                          disabled={isMasterAdmin || isCurrent}
                          className={`px-3 py-1.5 rounded-lg border text-xs font-medium transition cursor-pointer ${
                            isMasterAdmin || isCurrent
                              ? 'opacity-40 text-slate-400 border-slate-200 cursor-not-allowed'
                              : 'text-rose-600 border-rose-200 hover:bg-rose-50'
                          }`}
                          title={isMasterAdmin ? '系统默认主管理员不可删除' : isCurrent ? '无法删除当前账号' : '删除账号'}
                        >
                          删除
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

      {/* MODAL: Create New User */}
      {showAddModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/40 backdrop-blur-xs p-4 animate-in fade-in">
          <div className="w-full max-w-md bg-white rounded-3xl shadow-2xl border border-slate-100 p-7 space-y-6">
            <div className="flex items-center justify-between">
              <div className="flex items-center space-x-2.5">
                <div className="w-10 h-10 rounded-xl bg-indigo-50 flex items-center justify-center text-indigo-600">
                  <UserPlus className="w-5 h-5" />
                </div>
                <div>
                  <h3 className="text-lg font-bold text-slate-900">新建系统账号</h3>
                  <p className="text-xs text-slate-500">创建新管理员或运维人员登录凭证</p>
                </div>
              </div>
              <button
                onClick={() => setShowAddModal(false)}
                className="p-1.5 rounded-lg text-slate-400 hover:text-slate-600 hover:bg-slate-100 transition cursor-pointer"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            {addModalError && (
              <div className="p-3.5 rounded-xl bg-rose-50 border border-rose-200 text-rose-700 text-xs flex items-center space-x-2 animate-in fade-in">
                <AlertCircle className="w-4 h-4 shrink-0 text-rose-500" />
                <span>{addModalError}</span>
              </div>
            )}

            <form onSubmit={handleCreateUser} className="space-y-4">
              <div>
                <label className="block text-xs font-semibold text-slate-700 mb-1.5">
                  登录用户名
                </label>
                <input
                  type="text"
                  value={newUsername}
                  onChange={(e) => {
                    setNewUsername(e.target.value);
                    if (addModalError) setAddModalError('');
                  }}
                  placeholder="例如: ops_alice, dev_bob"
                  className="w-full px-4 py-2.5 rounded-xl border border-slate-200 text-sm text-slate-800 placeholder-slate-400 focus:outline-hidden focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 transition"
                />
              </div>

              <div>
                <div className="flex items-center justify-between mb-1.5">
                  <label className="text-xs font-semibold text-slate-700">
                    初始登录密码
                  </label>
                  <button
                    type="button"
                    onClick={() => {
                      const p = generateSecurePassword();
                      setNewPassword(p);
                      setShowNewPassPlain(true);
                      if (addModalError) setAddModalError('');
                      if (showToast) showToast('已自动生成高强度安全密码', 'info');
                    }}
                    className="text-[11px] text-indigo-600 hover:text-indigo-700 hover:underline flex items-center space-x-1 cursor-pointer"
                  >
                    <Sparkles className="w-3 h-3 text-indigo-500" />
                    <span>随机生成</span>
                  </button>
                </div>
                <div className="relative">
                  <input
                    type={showNewPassPlain ? 'text' : 'password'}
                    value={newPassword}
                    onChange={(e) => {
                      setNewPassword(e.target.value);
                      if (addModalError) setAddModalError('');
                    }}
                    placeholder="至少 6 位字符"
                    className="w-full px-4 py-2.5 rounded-xl border border-slate-200 text-sm font-mono text-slate-800 placeholder-slate-400 focus:outline-hidden focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 transition pr-10"
                  />
                  <button
                    type="button"
                    onClick={() => setShowNewPassPlain(!showNewPassPlain)}
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-slate-400 hover:text-slate-600"
                  >
                    {showNewPassPlain ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                  </button>
                </div>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-700 mb-1.5">
                  权限角色
                </label>
                <div className="grid grid-cols-2 gap-3">
                  <label
                    className={`flex items-center space-x-2.5 p-3 rounded-xl border cursor-pointer transition ${
                      newRole === 'operator'
                        ? 'border-indigo-500 bg-indigo-50/50 text-indigo-900 font-semibold'
                        : 'border-slate-200 hover:bg-slate-50 text-slate-700'
                    }`}
                  >
                    <input
                      type="radio"
                      name="role"
                      value="operator"
                      checked={newRole === 'operator'}
                      onChange={() => setNewRole('operator')}
                      className="hidden"
                    />
                    <UserCheck className="w-4 h-4 text-sky-600" />
                    <span className="text-xs">运维操作员</span>
                  </label>

                  <label
                    className={`flex items-center space-x-2.5 p-3 rounded-xl border cursor-pointer transition ${
                      newRole === 'admin'
                        ? 'border-indigo-500 bg-indigo-50/50 text-indigo-900 font-semibold'
                        : 'border-slate-200 hover:bg-slate-50 text-slate-700'
                    }`}
                  >
                    <input
                      type="radio"
                      name="role"
                      value="admin"
                      checked={newRole === 'admin'}
                      onChange={() => setNewRole('admin')}
                      className="hidden"
                    />
                    <Shield className="w-4 h-4 text-purple-600" />
                    <span className="text-xs">超级管理员</span>
                  </label>
                </div>
              </div>

              <div className="pt-3 flex items-center justify-end space-x-3">
                <button
                  type="button"
                  onClick={() => setShowAddModal(false)}
                  className="px-4 py-2.5 rounded-xl border border-slate-200 text-sm font-medium text-slate-600 hover:bg-slate-50 transition cursor-pointer"
                >
                  取消
                </button>
                <button
                  type="button"
                  onClick={handleCreateUser}
                  disabled={creating}
                  className="px-5 py-2.5 rounded-xl bg-indigo-600 hover:bg-indigo-700 text-white text-sm font-semibold shadow-xs transition cursor-pointer disabled:opacity-50"
                >
                  {creating ? '创建中...' : '确认创建'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* MODAL: Reset Password */}
      {resetTargetUser && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/40 backdrop-blur-xs p-4 animate-in fade-in">
          <div className="w-full max-w-md bg-white rounded-3xl shadow-2xl border border-slate-100 p-7 space-y-6">
            <div className="flex items-center justify-between">
              <div className="flex items-center space-x-2.5">
                <div className="w-10 h-10 rounded-xl bg-amber-50 flex items-center justify-center text-amber-600">
                  <Key className="w-5 h-5" />
                </div>
                <div>
                  <h3 className="text-lg font-bold text-slate-900">重置密码</h3>
                  <p className="text-xs text-slate-500">
                    为账号 <span className="font-semibold text-slate-800">[{resetTargetUser.username}]</span> 重设新登录密码
                  </p>
                </div>
              </div>
              <button
                onClick={() => setResetTargetUser(null)}
                className="p-1.5 rounded-lg text-slate-400 hover:text-slate-600 hover:bg-slate-100 transition cursor-pointer"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            {resetModalError && (
              <div className="p-3.5 rounded-xl bg-rose-50 border border-rose-200 text-rose-700 text-xs flex items-center space-x-2 animate-in fade-in">
                <AlertCircle className="w-4 h-4 shrink-0 text-rose-500" />
                <span>{resetModalError}</span>
              </div>
            )}

            <form onSubmit={handleResetPassword} className="space-y-4">
              <div>
                <div className="flex items-center justify-between mb-1.5">
                  <label className="text-xs font-semibold text-slate-700">
                    设置新密码
                  </label>
                  <button
                    type="button"
                    onClick={() => {
                      const p = generateSecurePassword();
                      setResetPassword(p);
                      setShowResetPassPlain(true);
                      if (resetModalError) setResetModalError('');
                      if (showToast) showToast('已自动生成高强度安全密码', 'info');
                    }}
                    className="text-[11px] text-indigo-600 hover:text-indigo-700 hover:underline flex items-center space-x-1 cursor-pointer"
                  >
                    <Sparkles className="w-3 h-3 text-indigo-500" />
                    <span>随机生成</span>
                  </button>
                </div>
                <div className="relative">
                  <input
                    type={showResetPassPlain ? 'text' : 'password'}
                    value={resetPassword}
                    onChange={(e) => {
                      setResetPassword(e.target.value);
                      if (resetModalError) setResetModalError('');
                    }}
                    placeholder="请输入至少 6 位新密码"
                    className="w-full px-4 py-2.5 rounded-xl border border-slate-200 text-sm font-mono text-slate-800 placeholder-slate-400 focus:outline-hidden focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 transition pr-10"
                  />
                  <button
                    type="button"
                    onClick={() => setShowResetPassPlain(!showResetPassPlain)}
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-slate-400 hover:text-slate-600"
                  >
                    {showResetPassPlain ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                  </button>
                </div>
              </div>

              <div className="pt-3 flex items-center justify-end space-x-3">
                <button
                  type="button"
                  onClick={() => setResetTargetUser(null)}
                  className="px-4 py-2.5 rounded-xl border border-slate-200 text-sm font-medium text-slate-600 hover:bg-slate-50 transition cursor-pointer"
                >
                  取消
                </button>
                <button
                  type="button"
                  onClick={handleResetPassword}
                  disabled={resetting}
                  className="px-5 py-2.5 rounded-xl bg-amber-600 hover:bg-amber-700 text-white text-sm font-semibold shadow-xs transition cursor-pointer disabled:opacity-50"
                >
                  {resetting ? '重置中...' : '确认重设'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* MODAL: Change Self Password */}
      {showChangeSelfPass && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/40 backdrop-blur-xs p-4 animate-in fade-in">
          <div className="w-full max-w-md bg-white rounded-3xl shadow-2xl border border-slate-100 p-7 space-y-6">
            <div className="flex items-center justify-between">
              <div className="flex items-center space-x-2.5">
                <div className="w-10 h-10 rounded-xl bg-indigo-50 flex items-center justify-center text-indigo-600">
                  <Lock className="w-5 h-5" />
                </div>
                <div>
                  <h3 className="text-lg font-bold text-slate-900">修改我的密码</h3>
                  <p className="text-xs text-slate-500">修改当前登录账号的密码</p>
                </div>
              </div>
              <button
                onClick={() => setShowChangeSelfPass(false)}
                className="p-1.5 rounded-lg text-slate-400 hover:text-slate-600 hover:bg-slate-100 transition cursor-pointer"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            {selfPassError && (
              <div className="p-3.5 rounded-xl bg-rose-50 border border-rose-200 text-rose-700 text-xs flex items-center space-x-2 animate-in fade-in">
                <AlertCircle className="w-4 h-4 shrink-0 text-rose-500" />
                <span>{selfPassError}</span>
              </div>
            )}

            <form onSubmit={handleChangeSelfPassword} className="space-y-4">
              <div>
                <label className="block text-xs font-semibold text-slate-700 mb-1.5">
                  原登录密码
                </label>
                <input
                  type="password"
                  value={oldSelfPassword}
                  onChange={(e) => {
                    setOldSelfPassword(e.target.value);
                    if (selfPassError) setSelfPassError('');
                  }}
                  placeholder="请输入当前正在使用的原密码"
                  className="w-full px-4 py-2.5 rounded-xl border border-slate-200 text-sm text-slate-800 placeholder-slate-400 focus:outline-hidden focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 transition"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-700 mb-1.5">
                  新密码
                </label>
                <input
                  type="password"
                  value={newSelfPassword}
                  onChange={(e) => {
                    setNewSelfPassword(e.target.value);
                    if (selfPassError) setSelfPassError('');
                  }}
                  placeholder="至少 6 位字符"
                  className="w-full px-4 py-2.5 rounded-xl border border-slate-200 text-sm text-slate-800 placeholder-slate-400 focus:outline-hidden focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 transition"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-700 mb-1.5">
                  确认新密码
                </label>
                <input
                  type="password"
                  value={confirmSelfPassword}
                  onChange={(e) => {
                    setConfirmSelfPassword(e.target.value);
                    if (selfPassError) setSelfPassError('');
                  }}
                  placeholder="再次输入新密码以确认"
                  className="w-full px-4 py-2.5 rounded-xl border border-slate-200 text-sm text-slate-800 placeholder-slate-400 focus:outline-hidden focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 transition"
                />
              </div>

              <div className="pt-3 flex items-center justify-end space-x-3">
                <button
                  type="button"
                  onClick={() => setShowChangeSelfPass(false)}
                  className="px-4 py-2.5 rounded-xl border border-slate-200 text-sm font-medium text-slate-600 hover:bg-slate-50 transition cursor-pointer"
                >
                  取消
                </button>
                <button
                  type="button"
                  onClick={handleChangeSelfPassword}
                  disabled={changingSelfPass}
                  className="px-5 py-2.5 rounded-xl bg-indigo-600 hover:bg-indigo-700 text-white text-sm font-semibold shadow-xs transition cursor-pointer disabled:opacity-50"
                >
                  {changingSelfPass ? '提交中...' : '保存新密码'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
