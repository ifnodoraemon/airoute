import { useState, useCallback } from 'react';
import useFormDirty from './useFormDirty';
import useUnsavedGuard from './useUnsavedGuard';
import useModalA11y from './useModalA11y';

export default function useUserForms({
  adminUser
}) {
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
  const [createdAccount, setCreatedAccount] = useState(null);

  // Quota Adjustment Modal State
  const [quotaTargetUser, setQuotaTargetUser] = useState(null);
  const [quotaMode, setQuotaMode] = useState('delta'); // 'delta' or 'exact'
  const [quotaValue, setQuotaValue] = useState('50');
  const [adjustingQuota, setAdjustingQuota] = useState(false);
  const [quotaModalError, setQuotaModalError] = useState('');

  // Reset Password Modal
  const [resetTargetUser, setResetTargetUser] = useState(null);
  const [resetPassword, setResetPassword] = useState('');
  // When an administrator resets their *own* password the gateway treats it as a
  // credential rotation, not an administrative override: it requires the current
  // password so a hijacked session cannot rotate the credential it stole. The
  // dialog therefore collects it for the self-reset case only.
  const [resetOldPassword, setResetOldPassword] = useState('');
  const [resetting, setResetting] = useState(false);
  const [resetModalError, setResetModalError] = useState('');
  const isSelfPasswordReset = !!resetTargetUser && !!adminUser && resetTargetUser.username === adminUser.username;

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

  // Password visibility
  const [showNewPassPlain, setShowNewPassPlain] = useState(false);
  const [showResetPassPlain, setShowResetPassPlain] = useState(false);
  const [showResetOldPassPlain, setShowResetOldPassPlain] = useState(false);

  const resetPasswordDraft = { oldPassword: resetOldPassword, password: resetPassword };
  const resetPasswordDirty = useFormDirty(!!resetTargetUser, resetPasswordDraft);
  // Closing the dialog is also what drops the typed credentials; the guard's
  // onClose must clear them so a reopened dialog never shows a stale password.
  const closeResetModal = useCallback(() => {
    setResetTargetUser(null);
    setResetPassword('');
    setResetOldPassword('');
    setResetModalError('');
    setShowResetPassPlain(false);
    setShowResetOldPassPlain(false);
  }, []);
  const {
    confirming: resetConfirming,
    requestClose: requestCloseReset,
    keepEditing: keepEditingReset,
    discard: discardReset
  } = useUnsavedGuard({ isOpen: !!resetTargetUser, isDirty: resetPasswordDirty, onClose: closeResetModal });

  useModalA11y(showAddModal, requestCloseAddUser);
  useModalA11y(!!quotaTargetUser, requestCloseQuota);
  useModalA11y(!!resetTargetUser, requestCloseReset);
  return {
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
  };
}
