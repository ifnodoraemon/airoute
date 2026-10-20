import React, { useState, useEffect, useCallback } from 'react';
import {
  Users,
  UserPlus,
  Shield,
  Key,
  Trash2,
  Lock,
  Unlock,
  RefreshCw,
  Search,
  AlertCircle,
  Eye,
  EyeOff,
  UserCheck,
  Sparkles,
  Coins
} from 'lucide-react';
import useModalA11y from '../hooks/useModalA11y';
import useFormDirty from '../hooks/useFormDirty';
import useUnsavedGuard from '../hooks/useUnsavedGuard';
import DiscardChangesDialog from './DiscardChangesDialog';

export default function UserManagementView({ adminUser, adminFetch, showToast, lang = 'zh', t }) {
  const isZh = lang === 'zh';
  const [users, setUsers] = useState([]);
  const [loading, setLoading] = useState(false);
  const [searchFilter, setSearchFilter] = useState('');
  const [roleFilter, setRoleFilter] = useState('all');

  // Add User Modal State
  const [showAddModal, setShowAddModal] = useState(false);
  const [newUsername, setNewUsername] = useState('');
  const [newEmail, setNewEmail] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [newRole, setNewRole] = useState('user');
  const [newStatus, setNewStatus] = useState('active');
  const [newBalance, setNewBalance] = useState('10.0');
  const [newGroupName, setNewGroupName] = useState('default');
  const [creating, setCreating] = useState(false);
  const [addModalError, setAddModalError] = useState('');

  // Quota Adjustment Modal State
  const [quotaTargetUser, setQuotaTargetUser] = useState(null);
  const [quotaMode, setQuotaMode] = useState('delta'); // 'delta' or 'exact'
  const [quotaValue, setQuotaValue] = useState('50');
  const [adjustingQuota, setAdjustingQuota] = useState(false);
  const [quotaModalError, setQuotaModalError] = useState('');

  // Reset Password Modal
  const [resetTargetUser, setResetTargetUser] = useState(null);
  const [resetPassword, setResetPassword] = useState('');
  const [resetting, setResetting] = useState(false);
  const [resetModalError, setResetModalError] = useState('');

  // Dismissing a form modal (backdrop / X / Cancel / Esc) must never silently
  // drop typed input, so every dismissal is routed through a per-modal guard.
  // Drafts contain only the fields the user can edit in that modal.
  const addUserDraft = {
    username: newUsername,
    email: newEmail,
    password: newPassword,
    role: newRole,
    status: newStatus,
    balance: newBalance,
    groupName: newGroupName
  };
  const addUserDirty = useFormDirty(showAddModal, addUserDraft);
  const {
    confirming: addUserConfirming,
    requestClose: requestCloseAddUser,
    keepEditing: keepEditingAddUser,
    discard: discardAddUser
  } = useUnsavedGuard({ isOpen: showAddModal, isDirty: addUserDirty, onClose: () => setShowAddModal(false) });

  const quotaDraft = { mode: quotaMode, value: quotaValue };
  const quotaDirty = useFormDirty(!!quotaTargetUser, quotaDraft);
  const {
    confirming: quotaConfirming,
    requestClose: requestCloseQuota,
    keepEditing: keepEditingQuota,
    discard: discardQuota
  } = useUnsavedGuard({ isOpen: !!quotaTargetUser, isDirty: quotaDirty, onClose: () => setQuotaTargetUser(null) });

  const resetPasswordDraft = { password: resetPassword };
  const resetPasswordDirty = useFormDirty(!!resetTargetUser, resetPasswordDraft);
  const {
    confirming: resetConfirming,
    requestClose: requestCloseReset,
    keepEditing: keepEditingReset,
    discard: discardReset
  } = useUnsavedGuard({ isOpen: !!resetTargetUser, isDirty: resetPasswordDirty, onClose: () => setResetTargetUser(null) });

  useModalA11y(showAddModal, requestCloseAddUser);
  useModalA11y(!!quotaTargetUser, requestCloseQuota);
  useModalA11y(!!resetTargetUser, requestCloseReset);

  // Password visibility
  const [showNewPassPlain, setShowNewPassPlain] = useState(false);
  const [showResetPassPlain, setShowResetPassPlain] = useState(false);

  const generateSecurePassword = () => {
    const chars = 'abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789!@#$';
    const out = [];
    const rnd = new Uint32Array(12);
    crypto.getRandomValues(rnd);
    for (let i = 0; i < 12; i++) {
      out.push(chars.charAt(rnd[i] % chars.length));
    }
    return out.join('');
  };

  const [pricingGroups, setPricingGroups] = useState([]);

  const fetchUsers = useCallback(async () => {
    if (!adminFetch) return;
    setLoading(true);
    try {
      const res = await adminFetch('/api/v1/admin/users');
      const data = await res.json();
      if (res.ok && data.code === 0) {
        setUsers(data.data || []);
      } else {
        if (showToast) showToast(data.error || (isZh ? '获取用户列表失败' : 'Failed to fetch user list'), 'error');
      }
    } catch (err) {
      if (showToast) showToast((isZh ? '请求异常: ' : 'Request error: ') + err.message, 'error');
    } finally {
      setLoading(false);
    }
  }, [adminFetch, showToast, isZh]);

  useEffect(() => {
    fetchUsers();
    if (adminFetch) {
      adminFetch('/api/v1/admin/pricing')
        .then(r => r.json())
        .then(d => {
          if (d.code === 0 && Array.isArray(d.data)) {
            const set = new Set();
            d.data.forEach(p => { if (p.group_name) set.add(p.group_name.trim().toLowerCase()); });
            setPricingGroups(Array.from(set));
          }
        })
        .catch(() => {});
    }
  }, [fetchUsers, adminFetch]);

  const allAvailableGroups = React.useMemo(() => {
    const set = new Set(['default']);
    (pricingGroups || []).forEach(g => set.add(g));
    users.forEach(u => { if (u.group_name) set.add(u.group_name.trim().toLowerCase()); });
    return Array.from(set);
  }, [pricingGroups, users]);

  const handleCreateUser = async (e) => {
    if (e && e.preventDefault) e.preventDefault();
    setAddModalError('');
    if (!newUsername.trim()) {
      setAddModalError(isZh ? '请输入登录用户名' : 'Please enter username');
      return;
    }
    if (newUsername.trim().length < 3) {
      setAddModalError(isZh ? '用户名长度至少需要 3 个字符' : 'Username must be at least 3 characters');
      return;
    }
    if (!newPassword.trim()) {
      setAddModalError(isZh ? '请输入初始登录密码' : 'Please enter password');
      return;
    }
    if (newPassword.length < 6) {
      setAddModalError(isZh ? '密码长度至少需要 6 个字符' : 'Password must be at least 6 characters');
      return;
    }

    setCreating(true);
    try {
      const res = await adminFetch('/api/v1/admin/users', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          username: newUsername.trim(),
          email: newEmail.trim(),
          password: newPassword,
          role: newRole,
          status: newStatus,
          balance: parseFloat(newBalance) || 0,
          group_name: newGroupName.trim() || 'default'
        })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        if (showToast) showToast(isZh ? `用户 [${newUsername}] 创建成功，已自动生成 API 密钥` : `User [${newUsername}] created successfully with API key`, 'success');
        // Reset the draft to its baseline first, so the unsaved guard can never
        // block the close that follows a successful create.
        setNewUsername('');
        setNewEmail('');
        setNewPassword('');
        setNewRole('user');
        setNewStatus('active');
        setNewBalance('10.0');
        setNewGroupName('default');
        setAddModalError('');
        setShowAddModal(false);
        fetchUsers();
      } else {
        const errMsg = data.error || (isZh ? '创建账号失败' : 'Failed to create user account');
        setAddModalError(errMsg);
        if (showToast) showToast(errMsg, 'error');
      }
    } catch (err) {
      const errMsg = (isZh ? '网络异常: ' : 'Network error: ') + err.message;
      setAddModalError(errMsg);
      if (showToast) showToast(errMsg, 'error');
    } finally {
      setCreating(false);
    }
  };

  const handleToggleStatus = async (user) => {
    if (user.username === 'admin') {
      showToast(isZh ? '系统主管理员账号 (admin) 不允许被锁定' : 'System admin account cannot be locked', 'warning');
      return;
    }
    const newStatus = user.status === 'locked' ? 'active' : 'locked';
    const actionName = newStatus === 'locked' ? (isZh ? '锁定' : 'lock') : (isZh ? '解锁' : 'unlock');
    if (!window.confirm(isZh ? `确认要${actionName}用户 [${user.username}] 吗？${newStatus === 'locked' ? '锁定后将无法登录或调用 API。' : ''}` : `Confirm ${actionName} user [${user.username}]?`)) {
      return;
    }

    try {
      const res = await adminFetch(`/api/v1/admin/users/${user.username}/status`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ status: newStatus })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(data.message || (isZh ? `用户已成功${actionName}` : `User status updated to ${newStatus}`), 'success');
        fetchUsers();
      } else {
        showToast(data.error || (isZh ? `${actionName}失败` : 'Status update failed'), 'error');
      }
    } catch (err) {
      showToast((isZh ? '请求异常: ' : 'Request error: ') + err.message, 'error');
    }
  };

  const handleAdjustQuota = async (e) => {
    if (e && e.preventDefault) e.preventDefault();
    setQuotaModalError('');
    if (!quotaTargetUser) return;
    const val = parseFloat(quotaValue);
    if (isNaN(val)) {
      setQuotaModalError(isZh ? '请输入有效数字' : 'Please enter a valid number');
      return;
    }

    setAdjustingQuota(true);
    try {
      const payload = quotaMode === 'exact' ? { balance: val } : { delta: val };
      const res = await adminFetch(`/api/v1/admin/users/${quotaTargetUser.username}/balance`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(data.message || (isZh ? '额度调整成功' : 'Balance updated successfully'), 'success');
        // Restore the draft baseline before closing so the guard cannot prompt.
        setQuotaMode('delta');
        setQuotaValue('50');
        setQuotaTargetUser(null);
        fetchUsers();
      } else {
        setQuotaModalError(data.error || (isZh ? '额度调整失败' : 'Failed to adjust balance'));
      }
    } catch (err) {
      setQuotaModalError((isZh ? '请求异常: ' : 'Request error: ') + err.message);
    } finally {
      setAdjustingQuota(false);
    }
  };

  const handleUpdateGroup = async (user, newGroup) => {
    try {
      const res = await adminFetch(`/api/v1/admin/users/${user.username}/group`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ group_name: newGroup })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(isZh ? `已成功将用户 [${user.username}] 分组调整为 [${newGroup}]` : `User [${user.username}] tier changed to [${newGroup}]`, 'success');
        fetchUsers();
      } else {
        showToast(data.error || (isZh ? '调整分组失败' : 'Failed to change tier'), 'error');
      }
    } catch (err) {
      showToast((isZh ? '请求异常: ' : 'Request error: ') + err.message, 'error');
    }
  };

  const handleUpdateRole = async (user, newRole) => {
    if (!adminFetch) return;
    if (user.username === 'admin') {
      showToast(isZh ? '系统内置超级管理员 (admin) 不允许修改角色' : 'Cannot change role of system admin', 'warning');
      return;
    }
    if (adminUser && user.username === adminUser.username && newRole !== 'admin') {
      showToast(isZh ? '不能将当前登录的管理员账号降级为普通用户' : 'Cannot demote current logged-in user', 'warning');
      return;
    }
    const roleLabel = newRole === 'admin' ? (isZh ? '超级管理员' : 'Administrator') : (isZh ? '普通用户' : 'Regular User');
    if (!window.confirm(isZh ? `确认将用户 [${user.username}] 的角色更改为【${roleLabel}】吗？` : `Confirm changing user [${user.username}] role to [${roleLabel}]?`)) {
      return;
    }

    try {
      const res = await adminFetch(`/api/v1/admin/users/${user.username}/role`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ role: newRole })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(data.message || (isZh ? `用户 [${user.username}] 已切换为【${roleLabel}】` : `User [${user.username}] role updated to [${roleLabel}]`), 'success');
        fetchUsers();
      } else {
        showToast(data.error || (isZh ? '修改角色失败' : 'Failed to change role'), 'error');
      }
    } catch (err) {
      showToast((isZh ? '请求异常: ' : 'Request error: ') + err.message, 'error');
    }
  };

  const handleResetPassword = async (e) => {
    if (e && e.preventDefault) e.preventDefault();
    setResetModalError('');
    if (!resetTargetUser) return;
    if (!resetPassword.trim()) {
      setResetModalError(isZh ? '请输入新密码' : 'Please enter new password');
      return;
    }
    if (resetPassword.length < 6) {
      setResetModalError(isZh ? '新密码长度至少需要 6 个字符' : 'Password must be at least 6 characters');
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
        showToast(isZh ? `账号 [${resetTargetUser.username}] 密码已成功重置` : `Password for [${resetTargetUser.username}] reset successfully`, 'success');
        // Clear the draft before closing so the guard cannot prompt afterwards.
        setResetPassword('');
        setResetTargetUser(null);
      } else {
        setResetModalError(data.error || (isZh ? '重置密码失败' : 'Failed to reset password'));
      }
    } catch (err) {
      setResetModalError((isZh ? '请求异常: ' : 'Request error: ') + err.message);
    } finally {
      setResetting(false);
    }
  };

  const handleDeleteUser = async (targetUsername) => {
    if (targetUsername === 'admin') {
      showToast(isZh ? '系统默认主管理员账号 (admin) 不允许删除' : 'System default admin cannot be deleted', 'warning');
      return;
    }
    if (adminUser && targetUsername === adminUser.username) {
      showToast(isZh ? '无法删除当前正在登录的账号' : 'Cannot delete currently logged in account', 'warning');
      return;
    }
    if (!window.confirm(isZh ? `确定要删除用户 [${targetUsername}] 吗？` : `Are you sure you want to delete user [${targetUsername}]?`)) {
      return;
    }

    try {
      const res = await adminFetch(`/api/v1/admin/users/${targetUsername}`, {
        method: 'DELETE'
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(isZh ? `账号 [${targetUsername}] 已成功删除` : `User [${targetUsername}] deleted successfully`, 'success');
        fetchUsers();
      } else {
        showToast(data.error || (isZh ? '删除账号失败' : 'Failed to delete user'), 'error');
      }
    } catch (err) {
      showToast((isZh ? '请求异常: ' : 'Request error: ') + err.message, 'error');
    }
  };

  const filteredUsers = users.filter((u) => {
    if (roleFilter !== 'all' && u.role !== roleFilter) return false;
    if (!searchFilter.trim()) return true;
    const q = searchFilter.toLowerCase();
    return (
      (u.username && u.username.toLowerCase().includes(q)) ||
      (u.email && u.email.toLowerCase().includes(q)) ||
      (u.group_name && u.group_name.toLowerCase().includes(q))
    );
  });

  const totalUsers = users.length;
  const adminCount = users.filter((u) => u.role === 'admin').length;
  const regularCount = users.filter((u) => u.role === 'user').length;
  const lockedCount = users.filter((u) => u.status === 'locked').length;

  return (
    <div className="space-y-6">
      {/* 1. View Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h2 className="text-lg font-bold text-slate-900 dark:text-slate-100 tracking-tight">
            {isZh ? '用户管理' : 'User Management'}
          </h2>
        </div>

        <div className="flex items-center space-x-3">
          <button
            onClick={() => {
              setAddModalError('');
              setNewPassword(generateSecurePassword());
              setShowAddModal(true);
            }}
            className="flex items-center space-x-2 px-4 py-2.5 rounded-xl bg-indigo-600 hover:bg-indigo-700 text-white text-xs font-bold shadow-sm hover:shadow-sm transition cursor-pointer"
          >
            <UserPlus className="w-4 h-4" />
            <span>{isZh ? '新建用户' : 'New User'}</span>
          </button>
        </div>
      </div>

      {/* 2. Stat Metric Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <div className="p-5 rounded-2xl bg-white dark:bg-[#0F1420] border border-slate-100 dark:border-slate-800 shadow-sm flex items-center justify-between">
          <div>
            <div className="ui-meta font-semibold">{isZh ? '总注册用户' : 'Total Users'}</div>
            <div className="text-2xl font-black text-slate-900 dark:text-slate-100 mt-1">{totalUsers}</div>
          </div>
          <div className="w-10 h-10 rounded-xl bg-indigo-50 dark:bg-indigo-950/40 border border-indigo-100 dark:border-indigo-900 flex items-center justify-center text-indigo-600 dark:text-indigo-400">
            <Users className="w-5 h-5" />
          </div>
        </div>

        <div className="p-5 rounded-2xl bg-white dark:bg-[#0F1420] border border-slate-100 dark:border-slate-800 shadow-sm flex items-center justify-between">
          <div>
            <div className="ui-meta font-semibold">{isZh ? '超级管理员' : 'Administrators'}</div>
            <div className="text-2xl font-black text-purple-700 dark:text-purple-300 mt-1">{adminCount}</div>
          </div>
          <div className="w-10 h-10 rounded-xl bg-purple-50 dark:bg-purple-950/40 border border-purple-100 dark:border-purple-900 flex items-center justify-center text-purple-600 dark:text-purple-400">
            <Shield className="w-5 h-5" />
          </div>
        </div>

        <div className="p-5 rounded-2xl bg-white dark:bg-[#0F1420] border border-slate-100 dark:border-slate-800 shadow-sm flex items-center justify-between">
          <div>
            <div className="ui-meta font-semibold">{isZh ? '普通用户' : 'Regular Users'}</div>
            <div className="text-2xl font-black text-emerald-700 dark:text-emerald-300 mt-1">{regularCount}</div>
          </div>
          <div className="w-10 h-10 rounded-xl bg-emerald-50 dark:bg-emerald-950/40 border border-emerald-100 dark:border-emerald-900 flex items-center justify-center text-emerald-600 dark:text-emerald-400">
            <UserCheck className="w-5 h-5" />
          </div>
        </div>

        <div className="p-5 rounded-2xl bg-white dark:bg-[#0F1420] border border-slate-100 dark:border-slate-800 shadow-sm flex items-center justify-between">
          <div>
            <div className="ui-meta font-semibold">{isZh ? '已锁定账号' : 'Locked Accounts'}</div>
            <div className="text-2xl font-black text-rose-700 dark:text-rose-300 mt-1">{lockedCount}</div>
          </div>
          <div className="w-10 h-10 rounded-xl bg-rose-50 dark:bg-rose-950/40 border border-rose-100 dark:border-rose-900 flex items-center justify-center text-rose-600 dark:text-rose-400">
            <Lock className="w-5 h-5" />
          </div>
        </div>
      </div>

      {/* 3. Search and Filters */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 bg-white dark:bg-[#0F1420] p-4 rounded-2xl border border-slate-100 dark:border-slate-800 shadow-sm">
        <div className="flex items-center space-x-3 flex-1 max-w-md">
          <div className="relative w-full">
            <Search className="w-4 h-4 text-slate-400 absolute left-3.5 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              value={searchFilter}
              onChange={(e) => setSearchFilter(e.target.value)}
              placeholder={isZh ? '搜索用户名、邮箱、分组名称...' : 'Search username, email, group...'}
              className="w-full pl-9 pr-4 py-2 bg-slate-50 dark:bg-slate-900 dark:text-slate-100 dark:placeholder-slate-500 border border-slate-200 dark:border-slate-700 rounded-xl text-xs text-slate-800 focus:outline-none focus:border-indigo-500 focus:bg-white dark:focus:bg-slate-800"
            />
          </div>
        </div>

        <div className="flex items-center space-x-2">
          <div className="flex items-center space-x-1 bg-slate-100 dark:bg-slate-800 p-1 rounded-xl text-xs font-semibold">
            <button
              onClick={() => setRoleFilter('all')}
              className={`px-3 py-1.5 rounded-lg transition cursor-pointer ${
                roleFilter === 'all'
                  ? 'bg-white dark:bg-slate-700 text-slate-900 dark:text-white shadow-sm'
                  : 'text-slate-500 dark:text-slate-400 hover:text-slate-800 dark:hover:text-slate-100'
              }`}
            >
              {isZh ? '全部' : 'All'}
            </button>
            <button
              onClick={() => setRoleFilter('admin')}
              className={`px-3 py-1.5 rounded-lg transition cursor-pointer ${
                roleFilter === 'admin'
                  ? 'bg-white dark:bg-slate-700 text-purple-700 dark:text-purple-300 shadow-sm'
                  : 'text-slate-500 dark:text-slate-400 hover:text-slate-800 dark:hover:text-slate-100'
              }`}
            >
              {isZh ? '管理员' : 'Admins'}
            </button>
            <button
              onClick={() => setRoleFilter('user')}
              className={`px-3 py-1.5 rounded-lg transition cursor-pointer ${
                roleFilter === 'user'
                  ? 'bg-white dark:bg-slate-700 text-emerald-700 dark:text-emerald-300 shadow-sm'
                  : 'text-slate-500 dark:text-slate-400 hover:text-slate-800 dark:hover:text-slate-100'
              }`}
            >
              {isZh ? '普通用户' : 'Users'}
            </button>
          </div>

          <button
            onClick={fetchUsers}
            disabled={loading}
            className="p-2 text-slate-500 dark:text-slate-400 hover:text-indigo-600 dark:hover:text-indigo-400 hover:bg-slate-100 dark:hover:bg-slate-800 rounded-xl transition cursor-pointer"
            title={isZh ? '刷新列表' : 'Refresh list'}
          >
            <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin text-indigo-600' : ''}`} />
          </button>
        </div>
      </div>

      {/* 4. Table */}
      <div className="bg-white dark:bg-[#0F1420] rounded-2xl border border-slate-100 dark:border-slate-800 shadow-sm overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-left border-collapse text-xs">
            <thead>
              <tr className="border-b border-slate-100 dark:border-slate-800 text-slate-500 dark:text-slate-400 uppercase font-bold bg-slate-50/50 dark:bg-slate-900/50">
                <th className="py-3.5 px-4">{isZh ? '用户标识与邮箱' : 'User & Email'}</th>
                <th className="py-3.5 px-4">{isZh ? '角色' : 'Role'}</th>
                <th className="py-3.5 px-4">{isZh ? '状态' : 'Status'}</th>
                <th className="py-3.5 px-4">{isZh ? '钱包余额 / 额度' : 'Balance / Quota'}</th>
                <th className="py-3.5 px-4" title={isZh ? '管理员为该用户分配的保障等级，用户创建 Key 时可享此等级或默认组' : 'Tier assigned by admin, user keys can enjoy this tier or default'}>{isZh ? '账号保障分组' : 'Account Tier'}</th>
                <th className="py-3.5 px-4">{isZh ? '注册时间' : 'Registered At'}</th>
                <th className="py-3.5 px-4 text-right">{isZh ? '操作' : 'Actions'}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
              {loading && users.length === 0 ? (
                <tr>
                  <td colSpan="7" className="py-12 text-center ui-meta">
                    <RefreshCw className="w-5 h-5 animate-spin text-indigo-600 mx-auto mb-2" />
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
                    <tr key={u.id} className="hover:bg-slate-50/70 dark:hover:bg-slate-800/40 transition">
                      <td className="py-3 px-4">
                        <div className="flex items-center space-x-2.5">
                          <div className="w-7 h-7 rounded-xl bg-slate-100 dark:bg-slate-800 flex items-center justify-center text-slate-600 dark:text-slate-300 font-bold shrink-0 uppercase">
                            {u.username.charAt(0)}
                          </div>
                          <div>
                            <div className="font-bold text-slate-900 dark:text-slate-100 flex items-center space-x-1.5">
                              <span>{u.username}</span>
                              {isCurrentAdmin && (
                                <span className="text-[10px] px-1.5 py-0.2 bg-indigo-50 dark:bg-indigo-950/50 text-indigo-600 dark:text-indigo-300 rounded-md font-semibold">
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

                      <td className="py-3 px-4">
                        <select
                          value={u.role || 'user'}
                          disabled={isSysAdmin || isCurrentAdmin}
                          onChange={(e) => handleUpdateRole(u, e.target.value)}
                          className={`border rounded-lg px-2 py-1 text-xs font-bold focus:outline-none transition cursor-pointer ${
                            u.role === 'admin'
                              ? 'bg-purple-50 dark:bg-purple-950/50 text-purple-700 dark:text-purple-300 border-purple-200 dark:border-purple-800 hover:bg-purple-100 dark:hover:bg-purple-900/60'
                              : 'bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 border-emerald-200 dark:border-emerald-800 hover:bg-emerald-100 dark:hover:bg-emerald-900/60'
                          } disabled:opacity-75 disabled:cursor-not-allowed`}
                          title={isSysAdmin ? (isZh ? '系统默认管理员不可修改角色' : 'Cannot change role of system admin') : (isCurrentAdmin ? (isZh ? '当前登录账号不可降级自己' : 'Cannot demote current user') : (isZh ? '点击切换用户角色' : 'Switch role'))}
                        >
                          <option value="user">{isZh ? '普通用户 (user)' : 'Regular User (user)'}</option>
                          <option value="admin">{isZh ? '超级管理员 (admin)' : 'Administrator (admin)'}</option>
                        </select>
                      </td>

                      <td className="py-3 px-4">
                        <span
                          className={`inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-bold ${
                            isLocked
                              ? 'bg-rose-100 dark:bg-rose-950/60 text-rose-800 dark:text-rose-300'
                              : 'bg-emerald-100 dark:bg-emerald-950/60 text-emerald-800 dark:text-emerald-300'
                          }`}
                        >
                          {isLocked ? (isZh ? '已锁定 (禁止调用)' : 'Locked') : (isZh ? '正常使用' : 'Active')}
                        </span>
                      </td>

                      <td className="py-3 px-4">
                        <div className="flex items-center space-x-2">
                          {u.role === 'admin' ? (
                            <span className="font-bold text-indigo-600 dark:text-indigo-400">{isZh ? '¥ 无限额度' : '¥ Unlimited'}</span>
                          ) : (
                            <span
                              className={`font-mono font-bold ${
                                u.balance <= 0 ? 'text-rose-600 dark:text-rose-400' : 'text-slate-900 dark:text-slate-100'
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
                            className="p-1 hover:bg-slate-100 dark:hover:bg-slate-800 rounded-sm text-slate-400 hover:text-indigo-600 dark:hover:text-indigo-400 transition"
                            title={isZh ? '调整用户额度' : 'Adjust balance'}
                          >
                            <Coins className="w-3.5 h-3.5" />
                          </button>
                        </div>
                      </td>

                      <td className="py-3 px-4">
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
                          className="bg-slate-50 dark:bg-slate-900 dark:text-slate-100 dark:placeholder-slate-500 border border-slate-200 dark:border-slate-700 rounded-lg px-2 py-1 text-xs font-medium text-slate-800 focus:outline-none focus:border-indigo-500 cursor-pointer"
                        >
                          {allAvailableGroups.map(g => (
                            <option key={g} value={g}>
                              {g === 'default' ? (isZh ? '默认组 (default)' : 'Default (default)') : g === 'vip' ? (isZh ? 'VIP 组 (vip)' : 'VIP (vip)') : g === 'enterprise' ? (isZh ? '企业组 (enterprise)' : 'Enterprise (enterprise)') : `${g} ${isZh ? '组' : 'Group'}`}
                            </option>
                          ))}
                          <option value="__custom__">{isZh ? '+ 输入自定义分组...' : '+ Custom tier...'}</option>
                        </select>
                      </td>

                      <td className="py-3 px-4 ui-meta">
                        {u.created_at || '-'}
                      </td>

                      <td className="py-3 px-4 text-right space-x-1">
                        {/* Lock / Unlock Toggle */}
                        <button
                          onClick={() => handleToggleStatus(u)}
                          disabled={isSysAdmin}
                          className={`p-1.5 rounded-lg transition ${
                            isLocked
                              ? 'bg-rose-50 dark:bg-rose-950/40 text-rose-600 dark:text-rose-400 hover:bg-rose-100 dark:hover:bg-rose-900/50'
                              : 'hover:bg-slate-100 dark:hover:bg-slate-800 text-slate-500 dark:text-slate-400 hover:text-rose-600 dark:hover:text-rose-400'
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
                            setResetModalError('');
                          }}
                          className="p-1.5 hover:bg-slate-100 dark:hover:bg-slate-800 rounded-lg text-slate-500 dark:text-slate-400 hover:text-indigo-600 dark:hover:text-indigo-400 transition"
                          title={isZh ? '重置登录密码' : 'Reset password'}
                        >
                          <Key className="w-3.5 h-3.5" />
                        </button>

                        {/* Delete User */}
                        <button
                          onClick={() => handleDeleteUser(u.username)}
                          disabled={isSysAdmin || isCurrentAdmin}
                          className="p-1.5 hover:bg-rose-50 dark:hover:bg-rose-950/40 rounded-lg text-slate-400 hover:text-rose-600 dark:hover:text-rose-400 transition disabled:opacity-30"
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

      {/* MODAL 1: Create New User */}
      {showAddModal && (
        <div
          className="fixed inset-0 bg-slate-900/60 flex items-center justify-center p-4 z-50 backdrop-blur-sm animate-in fade-in"
          onClick={(e) => {
            if (e.target === e.currentTarget) requestCloseAddUser();
          }}
        >
          <div className="relative bg-white dark:bg-[#0F1420] border border-slate-200 dark:border-slate-800 rounded-2xl p-6 max-w-md w-full shadow-2xl space-y-4 animate-in zoom-in-95 duration-150">
            <div className="flex items-center justify-between border-b border-slate-100 dark:border-slate-800 pb-3">
              <div className="flex items-center space-x-2">
                <UserPlus className="w-5 h-5 text-indigo-600 dark:text-indigo-400" />
                <h3 className="font-bold text-base text-slate-900 dark:text-slate-100">{isZh ? '创建新用户账号' : 'Create User Account'}</h3>
              </div>
              <button
                onClick={() => requestCloseAddUser()}
                className="p-1 hover:bg-slate-100 dark:hover:bg-slate-800 rounded-lg text-slate-400 hover:text-slate-600 dark:hover:text-slate-300"
              >
                ✕
              </button>
            </div>

            {addModalError && (
              <div className="p-3 rounded-xl bg-rose-50 dark:bg-rose-950/40 border border-rose-200 dark:border-rose-800 text-rose-800 dark:text-rose-300 text-xs flex items-center space-x-2">
                <AlertCircle className="w-4 h-4 shrink-0 text-rose-600 dark:text-rose-400" />
                <span>{addModalError}</span>
              </div>
            )}

            <form onSubmit={handleCreateUser} className="space-y-3">
              <div>
                <label className="block text-xs font-semibold text-slate-700 dark:text-slate-300 mb-1">{isZh ? '登录用户名 *' : 'Username *'}</label>
                <input
                  type="text"
                  required
                  value={newUsername}
                  onChange={(e) => setNewUsername(e.target.value)}
                  placeholder={isZh ? '至少 3 个字符' : 'At least 3 characters'}
                  className="w-full bg-slate-50 dark:bg-slate-900 dark:text-slate-100 dark:placeholder-slate-500 border border-slate-200 dark:border-slate-700 rounded-xl px-3 py-2 text-xs text-slate-900 focus:outline-none focus:border-indigo-500 focus:bg-white dark:focus:bg-slate-800"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-700 dark:text-slate-300 mb-1">{isZh ? '绑定邮箱 (选填)' : 'Email (Optional)'}</label>
                <input
                  type="email"
                  value={newEmail}
                  onChange={(e) => setNewEmail(e.target.value)}
                  placeholder="user@example.com"
                  className="w-full bg-slate-50 dark:bg-slate-900 dark:text-slate-100 dark:placeholder-slate-500 border border-slate-200 dark:border-slate-700 rounded-xl px-3 py-2 text-xs text-slate-900 focus:outline-none focus:border-indigo-500 focus:bg-white dark:focus:bg-slate-800"
                />
              </div>

              <div>
                <div className="flex items-center justify-between mb-1">
                  <label className="text-xs font-semibold text-slate-700 dark:text-slate-300">{isZh ? '登录密码 *' : 'Password *'}</label>
                  <button
                    type="button"
                    onClick={() => setNewPassword(generateSecurePassword())}
                    className="text-xs text-indigo-600 dark:text-indigo-400 hover:underline flex items-center space-x-1"
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
                    className="w-full bg-slate-50 dark:bg-slate-900 dark:text-slate-100 dark:placeholder-slate-500 border border-slate-200 dark:border-slate-700 rounded-xl pl-3 pr-10 py-2 text-xs text-slate-900 focus:outline-none focus:border-indigo-500 focus:bg-white dark:focus:bg-slate-800 font-mono"
                  />
                  <button
                    type="button"
                    onClick={() => setShowNewPassPlain(!showNewPassPlain)}
                    className="absolute right-3 top-2.5 text-slate-400 hover:text-slate-600 dark:hover:text-slate-300"
                  >
                    {showNewPassPlain ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                  </button>
                </div>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-semibold text-slate-700 dark:text-slate-300 mb-1">{isZh ? '权限角色' : 'Role'}</label>
                  <select
                    value={newRole}
                    onChange={(e) => setNewRole(e.target.value)}
                    className="w-full bg-slate-50 dark:bg-slate-900 dark:text-slate-100 dark:placeholder-slate-500 border border-slate-200 dark:border-slate-700 rounded-xl px-3 py-2 text-xs text-slate-900 focus:outline-none focus:border-indigo-500"
                  >
                    <option value="user">{isZh ? '普通用户 (user)' : 'Regular User (user)'}</option>
                    <option value="admin">{isZh ? '超级管理员 (admin)' : 'Administrator (admin)'}</option>
                  </select>
                </div>

                <div>
                  <label className="block text-xs font-semibold text-slate-700 dark:text-slate-300 mb-1">{isZh ? '初始状态' : 'Initial Status'}</label>
                  <select
                    value={newStatus}
                    onChange={(e) => setNewStatus(e.target.value)}
                    className="w-full bg-slate-50 dark:bg-slate-900 dark:text-slate-100 dark:placeholder-slate-500 border border-slate-200 dark:border-slate-700 rounded-xl px-3 py-2 text-xs text-slate-900 focus:outline-none focus:border-indigo-500"
                  >
                    <option value="active">{isZh ? '正常活跃 (active)' : 'Active (active)'}</option>
                    <option value="locked">{isZh ? '直接锁定 (locked)' : 'Locked (locked)'}</option>
                  </select>
                </div>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-semibold text-slate-700 dark:text-slate-300 mb-1">{isZh ? '初始额度 (CNY)' : 'Initial Balance (CNY)'}</label>
                  <input
                    type="number"
                    step="0.01"
                    value={newBalance}
                    onChange={(e) => setNewBalance(e.target.value)}
                    placeholder="10.0"
                    className="w-full bg-slate-50 dark:bg-slate-900 dark:text-slate-100 dark:placeholder-slate-500 border border-slate-200 dark:border-slate-700 rounded-xl px-3 py-2 text-xs text-slate-900 focus:outline-none focus:border-indigo-500 font-mono"
                  />
                </div>

                <div>
                  <label className="block text-xs font-semibold text-slate-700 dark:text-slate-300 mb-1">{isZh ? '账号保障分组 (Tier)' : 'Account Tier'}</label>
                  <div className="space-y-1.5">
                    <select
                      value={allAvailableGroups.includes(newGroupName) ? newGroupName : '__custom__'}
                      onChange={(e) => {
                        if (e.target.value !== '__custom__') {
                          setNewGroupName(e.target.value);
                        }
                      }}
                      className="w-full bg-slate-50 dark:bg-slate-900 dark:text-slate-100 dark:placeholder-slate-500 border border-slate-200 dark:border-slate-700 rounded-xl px-3 py-2 text-xs text-slate-900 focus:outline-none focus:border-indigo-500"
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
                        className="w-full bg-slate-50 dark:bg-slate-900 dark:text-slate-100 dark:placeholder-slate-500 border border-slate-200 dark:border-slate-700 rounded-xl px-3 py-1.5 text-xs text-slate-900 focus:outline-none focus:border-indigo-500 font-mono"
                      />
                    )}
                  </div>
                </div>
              </div>

              <div className="pt-2 flex items-center justify-end space-x-2">
                <button
                  type="button"
                  onClick={() => requestCloseAddUser()}
                  className="px-4 py-2 border border-slate-200 dark:border-slate-800 hover:bg-slate-50 dark:hover:bg-slate-800 text-slate-700 dark:text-slate-300 rounded-xl text-xs font-medium cursor-pointer"
                >
                  {isZh ? '取消' : 'Cancel'}
                </button>
                <button
                  type="submit"
                  disabled={creating}
                  className="px-5 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-bold transition flex items-center space-x-1.5 shadow-sm disabled:opacity-50 cursor-pointer"
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
      )}

      {/* MODAL 2: Adjust Quota / Balance */}
      {quotaTargetUser && (
        <div
          className="fixed inset-0 bg-slate-900/60 flex items-center justify-center p-4 z-50 backdrop-blur-sm animate-in fade-in"
          onClick={(e) => {
            if (e.target === e.currentTarget) requestCloseQuota();
          }}
        >
          <div className="relative bg-white dark:bg-[#0F1420] border border-slate-200 dark:border-slate-800 rounded-2xl p-6 max-w-sm w-full shadow-2xl space-y-4 animate-in zoom-in-95 duration-150">
            <div className="flex items-center justify-between border-b border-slate-100 dark:border-slate-800 pb-3">
              <div className="flex items-center space-x-2">
                <Coins className="w-5 h-5 text-indigo-600 dark:text-indigo-400" />
                <h3 className="font-bold text-base text-slate-900 dark:text-slate-100">{isZh ? '调整用户可用额度' : 'Adjust User Quota / Balance'}</h3>
              </div>
              <button
                onClick={() => requestCloseQuota()}
                className="p-1 hover:bg-slate-100 dark:hover:bg-slate-800 rounded-lg text-slate-400 hover:text-slate-600 dark:hover:text-slate-300"
              >
                ✕
              </button>
            </div>

            <div className="p-3 bg-slate-50 dark:bg-slate-900/60 rounded-2xl border border-slate-100 dark:border-slate-800 text-xs space-y-1">
              <div className="ui-meta">{isZh ? '目标用户:' : 'Target User:'} <span className="font-bold text-slate-900 dark:text-slate-100">{quotaTargetUser.username}</span></div>
              <div className="ui-meta">{isZh ? '当前余额:' : 'Current Balance:'} <span className="font-bold font-mono text-indigo-600 dark:text-indigo-400">¥ {Number(quotaTargetUser.balance || 0).toFixed(2)}</span></div>
            </div>

            {quotaModalError && (
              <div className="p-2.5 rounded-xl bg-rose-50 dark:bg-rose-950/40 border border-rose-200 dark:border-rose-800 text-rose-800 dark:text-rose-300 text-xs">
                {quotaModalError}
              </div>
            )}

            <form onSubmit={handleAdjustQuota} className="space-y-3">
              <div className="grid grid-cols-2 gap-2 bg-slate-100 dark:bg-slate-800 p-1 rounded-xl text-xs font-semibold">
                <button
                  type="button"
                  onClick={() => setQuotaMode('delta')}
                  className={`py-1.5 rounded-lg transition ${
                    quotaMode === 'delta' ? 'bg-white dark:bg-slate-700 text-indigo-600 dark:text-indigo-300 shadow-sm' : 'text-slate-500 dark:text-slate-400'
                  }`}
                >
                  {isZh ? '增减额度 (+/-)' : 'Add/Deduct (+/-)'}
                </button>
                <button
                  type="button"
                  onClick={() => setQuotaMode('exact')}
                  className={`py-1.5 rounded-lg transition ${
                    quotaMode === 'exact' ? 'bg-white dark:bg-slate-700 text-indigo-600 dark:text-indigo-300 shadow-sm' : 'text-slate-500 dark:text-slate-400'
                  }`}
                >
                  {isZh ? '设定确切余额' : 'Set Exact Balance'}
                </button>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-700 dark:text-slate-300 mb-1">
                  {quotaMode === 'delta' ? (isZh ? '调整额度数值 (可输入负数扣除)' : 'Adjustment Amount (negative to deduct)') : (isZh ? '设定新的账户余额' : 'New Account Balance')}
                </label>
                <input
                  type="number"
                  step="0.01"
                  required
                  value={quotaValue}
                  onChange={(e) => setQuotaValue(e.target.value)}
                  className="w-full bg-slate-50 dark:bg-slate-900 dark:text-slate-100 dark:placeholder-slate-500 border border-slate-200 dark:border-slate-700 rounded-xl px-3 py-2 text-xs text-slate-900 focus:outline-none focus:border-indigo-500 font-mono text-lg font-bold"
                />
              </div>

              {/* Quick Preset Buttons */}
              <div className="flex items-center space-x-1.5 pt-1">
                {[10, 50, 100, 500].map((num) => (
                  <button
                    key={num}
                    type="button"
                    onClick={() => {
                      setQuotaMode('delta');
                      setQuotaValue(String(num));
                    }}
                    className="px-2 py-1 bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700 rounded-lg text-xs font-bold text-slate-700 dark:text-slate-200 cursor-pointer"
                  >
                    +{num}
                  </button>
                ))}
                <button
                  type="button"
                  onClick={() => {
                    setQuotaMode('exact');
                    setQuotaValue('0');
                  }}
                  className="px-2 py-1 bg-rose-50 dark:bg-rose-950/40 hover:bg-rose-100 dark:hover:bg-rose-900/60 rounded-lg text-xs font-bold text-rose-700 dark:text-rose-300 cursor-pointer"
                >
                  {isZh ? '归零' : 'Zero'}
                </button>
              </div>

              <div className="pt-2 flex items-center justify-end space-x-2">
                <button
                  type="button"
                  onClick={() => requestCloseQuota()}
                  className="px-4 py-2 border border-slate-200 dark:border-slate-800 hover:bg-slate-50 dark:hover:bg-slate-800 text-slate-700 dark:text-slate-300 rounded-xl text-xs font-medium cursor-pointer"
                >
                  {isZh ? '取消' : 'Cancel'}
                </button>
                <button
                  type="submit"
                  disabled={adjustingQuota}
                  className="px-5 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-bold transition flex items-center space-x-1.5 shadow-sm disabled:opacity-50 cursor-pointer"
                >
                  {adjustingQuota ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <span>{isZh ? '确认更新' : 'Update Balance'}</span>}
                </button>
              </div>
            </form>

            <DiscardChangesDialog
              open={quotaConfirming}
              onKeepEditing={keepEditingQuota}
              onDiscard={discardQuota}
              lang={lang}
              title={isZh ? '放弃未保存的额度调整？' : 'Discard this quota change?'}
            />
          </div>
        </div>
      )}

      {/* MODAL 3: Reset Password */}
      {resetTargetUser && (
        <div
          className="fixed inset-0 bg-slate-900/60 flex items-center justify-center p-4 z-50 backdrop-blur-sm animate-in fade-in"
          onClick={(e) => {
            if (e.target === e.currentTarget) requestCloseReset();
          }}
        >
          <div className="relative bg-white dark:bg-[#0F1420] border border-slate-200 dark:border-slate-800 rounded-2xl p-6 max-w-sm w-full shadow-2xl space-y-4 animate-in zoom-in-95 duration-150">
            <div className="flex items-center justify-between border-b border-slate-100 dark:border-slate-800 pb-3">
              <div className="flex items-center space-x-2">
                <Key className="w-5 h-5 text-indigo-600 dark:text-indigo-400" />
                <h3 className="font-bold text-base text-slate-900 dark:text-slate-100">{isZh ? '重置用户密码' : 'Reset User Password'}</h3>
              </div>
              <button
                onClick={() => requestCloseReset()}
                className="p-1 hover:bg-slate-100 dark:hover:bg-slate-800 rounded-lg text-slate-400 hover:text-slate-600 dark:hover:text-slate-300"
              >
                ✕
              </button>
            </div>

            <p className="text-xs text-slate-500 dark:text-slate-400">
              {isZh ? '正在重置用户 ' : 'Resetting password for user '}
              <span className="font-bold text-slate-900 dark:text-slate-100">[{resetTargetUser.username}]</span>
              {isZh ? ' 的登录密码。' : '.'}
            </p>

            {resetModalError && (
              <div className="p-2.5 rounded-xl bg-rose-50 dark:bg-rose-950/40 border border-rose-200 dark:border-rose-800 text-rose-800 dark:text-rose-300 text-xs">
                {resetModalError}
              </div>
            )}

            <form onSubmit={handleResetPassword} className="space-y-3">
              <div>
                <div className="flex items-center justify-between mb-1">
                  <label className="text-xs font-semibold text-slate-700 dark:text-slate-300">{isZh ? '新密码 *' : 'New Password *'}</label>
                  <button
                    type="button"
                    onClick={() => setResetPassword(generateSecurePassword())}
                    className="text-xs text-indigo-600 dark:text-indigo-400 hover:underline flex items-center space-x-1 cursor-pointer"
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
                    className="w-full bg-slate-50 dark:bg-slate-900 dark:text-slate-100 dark:placeholder-slate-500 border border-slate-200 dark:border-slate-700 rounded-xl pl-3 pr-10 py-2 text-xs text-slate-900 focus:outline-none focus:border-indigo-500 font-mono"
                  />
                  <button
                    type="button"
                    onClick={() => setShowResetPassPlain(!showResetPassPlain)}
                    className="absolute right-3 top-2.5 text-slate-400 hover:text-slate-600 dark:hover:text-slate-300 cursor-pointer"
                  >
                    {showResetPassPlain ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                  </button>
                </div>
              </div>

              <div className="pt-2 flex items-center justify-end space-x-2">
                <button
                  type="button"
                  onClick={() => requestCloseReset()}
                  className="px-4 py-2 border border-slate-200 dark:border-slate-800 hover:bg-slate-50 dark:hover:bg-slate-800 text-slate-700 dark:text-slate-300 rounded-xl text-xs font-medium cursor-pointer"
                >
                  {isZh ? '取消' : 'Cancel'}
                </button>
                <button
                  type="submit"
                  disabled={resetting}
                  className="px-5 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-bold transition flex items-center space-x-1.5 shadow-sm disabled:opacity-50 cursor-pointer"
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
      )}
    </div>
  );
}
