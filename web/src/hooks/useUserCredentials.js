import { apiMutation } from '../api/mutation';

export default function useUserCredentials({
  adminFetch, adminUser, fetchUsers, isSelfPasswordReset, isZh, resetOldPassword, resetPassword,
  resetTargetUser, setResetModalError, setResetOldPassword, setResetPassword, setResetTargetUser,
  setResetting, showToast
}) {
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
    // Resetting your own password is a rotation: the gateway rejects it without
    // the current one, so validate locally first instead of eating a 400.
    if (isSelfPasswordReset && !resetOldPassword.trim()) {
      setResetModalError(isZh ? '重置本人密码需要输入当前登录密码' : 'Enter your current password to reset your own password');
      return;
    }

    setResetting(true);
    try {
      const payload = { new_password: resetPassword };
      if (isSelfPasswordReset) payload.old_password = resetOldPassword;
      const { data, ok: requestSucceeded } = await apiMutation(`/api/v1/admin/users/${encodeURIComponent(resetTargetUser.username)}/password`, 'POST', payload, adminFetch);

      if (requestSucceeded) {
        showToast(isZh ? `账号 [${resetTargetUser.username}] 密码已成功重置` : `Password for [${resetTargetUser.username}] reset successfully`, 'success');
        // Clear the drafts before closing so the guard cannot prompt afterwards.
        setResetPassword('');
        setResetOldPassword('');
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
    if (!window.confirm(isZh ? `确定删除用户 [${targetUsername}] 吗？登录和密钥将立即失效，账务记录会保留，该用户名不能重新注册。` : `Delete user [${targetUsername}]? Login and API keys will be revoked. Billing records and the username will be retained.`)) {
      return;
    }

    try {
      const { data, ok: requestSucceeded } = await apiMutation(`/api/v1/admin/users/${encodeURIComponent(targetUsername)}`, 'DELETE', undefined, adminFetch);

      if (requestSucceeded) {
        showToast(isZh ? `账号 [${targetUsername}] 已成功删除` : `User [${targetUsername}] deleted successfully`, 'success');
        fetchUsers();
      } else {
        showToast(data.error || (isZh ? '删除账号失败' : 'Failed to delete user'), 'error');
      }
    } catch (err) {
      showToast((isZh ? '请求异常: ' : 'Request error: ') + err.message, 'error');
    }
  };
  return {
    handleDeleteUser, handleResetPassword
  };
}
