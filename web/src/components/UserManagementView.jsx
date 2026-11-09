import useUserCredentials from '../hooks/useUserCredentials';
import useUserGovernance from '../hooks/useUserGovernance';
import useUserCreation from '../hooks/useUserCreation';
import useUserData from '../hooks/useUserData';
import useUserForms from '../hooks/useUserForms';
import UserFilters from './UserFilters';
import UserTable from './UserTable';
import UserCreateModal from './UserCreateModal';
import UserQuotaModal from './UserQuotaModal';
import UserPasswordModal from './UserPasswordModal';

import { Users, UserPlus, Shield, Lock, UserCheck } from 'lucide-react';

import OneTimeKeyDialog from './OneTimeKeyDialog';

export default function UserManagementView({ adminUser, adminFetch, showToast, onUserCreated, lang = 'zh' }) {
  const isZh = lang === 'zh';
  const {
    addModalError, addUserConfirming, adjustingQuota, createdAccount, creating, discardAddUser, discardQuota,
    discardReset, isSelfPasswordReset, keepEditingAddUser, keepEditingQuota, keepEditingReset, loading,
    newBalance, newEmail, newGroupName, newPassword, newRole, newStatus, newUsername, quotaConfirming,
    quotaModalError, quotaMode, quotaTargetUser, quotaValue, requestCloseAddUser, requestCloseQuota,
    requestCloseReset, resetConfirming, resetModalError, resetOldPassword, resetPassword, resetTargetUser,
    resetting, roleFilter, searchFilter, setAddModalError, setAdjustingQuota, setCreatedAccount, setCreating,
    setLoading, setNewBalance, setNewEmail, setNewGroupName, setNewPassword, setNewRole, setNewStatus,
    setNewUsername, setQuotaModalError, setQuotaMode, setQuotaTargetUser, setQuotaValue, setResetModalError,
    setResetOldPassword, setResetPassword, setResetTargetUser, setResetting, setRoleFilter, setSearchFilter,
    setShowAddModal, setShowNewPassPlain, setShowResetOldPassPlain, setShowResetPassPlain, setUsers,
    showAddModal, showNewPassPlain, showResetOldPassPlain, showResetPassPlain, users
  } = useUserForms({
    adminUser
  });

  const {
    allAvailableGroups, fetchUsers, generateSecurePassword
  } = useUserData({
    adminFetch, isZh, setLoading, setUsers, showToast, users
  });

  const {
    handleCreateUser
  } = useUserCreation({
    adminFetch, fetchUsers, isZh, newBalance, newEmail, newGroupName, newPassword, newRole, newStatus,
    newUsername, onUserCreated, setAddModalError, setCreatedAccount, setCreating, setNewBalance, setNewEmail,
    setNewGroupName, setNewPassword, setNewRole, setNewStatus, setNewUsername, setShowAddModal, showToast
  });

  const {
    handleAdjustQuota, handleToggleStatus, handleUpdateGroup, handleUpdateRole
  } = useUserGovernance({
    adminFetch, adminUser, fetchUsers, isZh, quotaMode, quotaTargetUser, quotaValue, setAdjustingQuota,
    setQuotaModalError, setQuotaMode, setQuotaTargetUser, setQuotaValue, showToast
  });

  const {
    handleDeleteUser, handleResetPassword
  } = useUserCredentials({
    adminFetch, adminUser, fetchUsers, isSelfPasswordReset, isZh, resetOldPassword, resetPassword,
    resetTargetUser, setResetModalError, setResetOldPassword, setResetPassword, setResetTargetUser,
    setResetting, showToast
  });

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
          <h2 className="ui-page-title">
            {isZh ? '用户管理' : 'User Management'}
          </h2>
          <p className="ui-page-subtitle">
            {isZh ? '管理平台注册用户、权限角色分配与账户额度' : 'Manage platform users, roles, and balance quotas'}
          </p>
        </div>

        <div className="flex items-center space-x-3">
          <button
            onClick={() => {
              setAddModalError('');
              setNewPassword(generateSecurePassword());
              setShowAddModal(true);
            }}
            className="ui-btn-primary ui-btn-sm"
          >
            <UserPlus className="w-4 h-4" />
            <span>{isZh ? '新建用户' : 'New User'}</span>
          </button>
        </div>
      </div>

      {/* 2. Stat Metric Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <div className="ui-card p-4 flex items-center justify-between">
          <div>
            <div className="ui-meta font-medium">{isZh ? '总注册用户' : 'Total Users'}</div>
            <div className="ui-metric-number mt-1">{totalUsers}</div>
          </div>
          <div className="w-9 h-9 rounded-lg bg-[#EEF2FF] dark:bg-[#242D48] flex items-center justify-center text-[#355CFF] dark:text-[#91A7FF]">
            <Users className="w-4.5 h-4.5" />
          </div>
        </div>

        <div className="ui-card p-4 flex items-center justify-between">
          <div>
            <div className="ui-meta font-medium">{isZh ? '超级管理员' : 'Administrators'}</div>
            <div className="ui-metric-number text-purple-600 dark:text-purple-400 mt-1">{adminCount}</div>
          </div>
          <div className="w-9 h-9 rounded-lg bg-purple-50 dark:bg-purple-950/40 flex items-center justify-center text-purple-600 dark:text-purple-400">
            <Shield className="w-4.5 h-4.5" />
          </div>
        </div>

        <div className="ui-card p-4 flex items-center justify-between">
          <div>
            <div className="ui-meta font-medium">{isZh ? '普通用户' : 'Regular Users'}</div>
            <div className="ui-metric-number text-emerald-600 dark:text-emerald-400 mt-1">{regularCount}</div>
          </div>
          <div className="w-9 h-9 rounded-lg bg-emerald-50 dark:bg-emerald-950/40 flex items-center justify-center text-emerald-600 dark:text-emerald-400">
            <UserCheck className="w-4.5 h-4.5" />
          </div>
        </div>

        <div className="ui-card p-4 flex items-center justify-between">
          <div>
            <div className="ui-meta font-medium">{isZh ? '已锁定账号' : 'Locked Accounts'}</div>
            <div className="ui-metric-number text-rose-600 dark:text-rose-400 mt-1">{lockedCount}</div>
          </div>
          <div className="w-9 h-9 rounded-lg bg-rose-50 dark:bg-rose-950/40 flex items-center justify-center text-rose-600 dark:text-rose-400">
            <Lock className="w-4.5 h-4.5" />
          </div>
        </div>
      </div>

      {/* 3. Search and Filters */}
      <UserFilters {...{
        fetchUsers, isZh, loading, roleFilter, searchFilter, setRoleFilter, setSearchFilter
      }} />

      {/* 4. Table */}
      <UserTable {...{
        adminUser, allAvailableGroups, filteredUsers, generateSecurePassword, handleDeleteUser, handleToggleStatus,
        handleUpdateGroup, handleUpdateRole, isZh, loading, setQuotaModalError, setQuotaMode, setQuotaTargetUser,
        setQuotaValue, setResetModalError, setResetOldPassword, setResetPassword, setResetTargetUser, users
      }} />

      {/* MODAL 1: Create New User */}
      {createdAccount && <OneTimeKeyDialog credential={createdAccount.api_key} username={createdAccount.username} lang={lang} onClose={() => setCreatedAccount(null)} />}

      {showAddModal && (
        <UserCreateModal {...{
          addModalError, addUserConfirming, allAvailableGroups, creating, discardAddUser, generateSecurePassword,
          handleCreateUser, isZh, keepEditingAddUser, lang, newBalance, newEmail, newGroupName, newPassword, newRole,
          newStatus, newUsername, requestCloseAddUser, setNewBalance, setNewEmail, setNewGroupName, setNewPassword,
          setNewRole, setNewStatus, setNewUsername, setShowNewPassPlain, showNewPassPlain
        }} />
      )}

      {/* MODAL 2: Adjust Quota / Balance */}
      {quotaTargetUser && (
        <UserQuotaModal {...{
          adjustingQuota, discardQuota, handleAdjustQuota, isZh, keepEditingQuota, lang, quotaConfirming,
          quotaModalError, quotaMode, quotaTargetUser, quotaValue, requestCloseQuota, setQuotaMode, setQuotaValue
        }} />
      )}

      {/* MODAL 3: Reset Password */}
      {resetTargetUser && (
        <UserPasswordModal {...{
          discardReset, generateSecurePassword, handleResetPassword, isSelfPasswordReset, isZh, keepEditingReset,
          lang, requestCloseReset, resetConfirming, resetModalError, resetOldPassword, resetPassword, resetTargetUser,
          resetting, setResetOldPassword, setResetPassword, setShowResetOldPassPlain, setShowResetPassPlain,
          showResetOldPassPlain, showResetPassPlain
        }} />
      )}
    </div>
  );
}
