import { apiMutation } from '../api/mutation';

export default function useUserCreation({
  adminFetch, fetchUsers, isZh, newBalance, newEmail, newGroupName, newPassword, newRole, newStatus,
  newUsername, onUserCreated, setAddModalError, setCreatedAccount, setCreating, setNewBalance, setNewEmail,
  setNewGroupName, setNewPassword, setNewRole, setNewStatus, setNewUsername, setShowAddModal, showToast
}) {
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
      const { data, ok: requestSucceeded } = await apiMutation('/api/v1/admin/users', 'POST', {
          username: newUsername.trim(),
          email: newEmail.trim(),
          password: newPassword,
          role: newRole,
          status: newStatus,
          balance: parseFloat(newBalance) || 0,
          group_name: newGroupName.trim() || 'default'
        }, adminFetch);

      if (requestSucceeded) {
        if (data.data?.api_key) setCreatedAccount(data.data);
        onUserCreated?.();
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
  return {
    handleCreateUser
  };
}
