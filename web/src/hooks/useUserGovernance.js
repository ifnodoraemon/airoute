import { apiMutation } from '../api/mutation';

export default function useUserGovernance({
  adminFetch, adminUser, fetchUsers, isZh, quotaMode, quotaTargetUser, quotaValue, setAdjustingQuota,
  setQuotaModalError, setQuotaMode, setQuotaTargetUser, setQuotaValue, showToast
}) {
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
      const { data, ok: requestSucceeded } = await apiMutation(`/api/v1/admin/users/${encodeURIComponent(user.username)}/status`, 'POST', { status: newStatus }, adminFetch);

      if (requestSucceeded) {
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
    if (!Number.isFinite(val) || Math.abs(val) > 1000000000) {
      setQuotaModalError(isZh ? '请输入 -1000000000 至 1000000000 之间的金额' : 'Enter an amount between -1000000000 and 1000000000');
      return;
    }

    setAdjustingQuota(true);
    try {
      const payload = quotaMode === 'exact' ? { balance: val } : { delta: val };
      const { data, ok: requestSucceeded } = await apiMutation(`/api/v1/admin/users/${encodeURIComponent(quotaTargetUser.username)}/balance`, 'POST', payload, adminFetch);

      if (requestSucceeded) {
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
      const { data, ok: requestSucceeded } = await apiMutation(`/api/v1/admin/users/${encodeURIComponent(user.username)}/group`, 'POST', { group_name: newGroup }, adminFetch);

      if (requestSucceeded) {
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
      const { data, ok: requestSucceeded } = await apiMutation(`/api/v1/admin/users/${encodeURIComponent(user.username)}/role`, 'POST', { role: newRole }, adminFetch);

      if (requestSucceeded) {
        showToast(data.message || (isZh ? `用户 [${user.username}] 已切换为【${roleLabel}】` : `User [${user.username}] role updated to [${roleLabel}]`), 'success');
        fetchUsers();
      } else {
        showToast(data.error || (isZh ? '修改角色失败' : 'Failed to change role'), 'error');
      }
    } catch (err) {
      showToast((isZh ? '请求异常: ' : 'Request error: ') + err.message, 'error');
    }
  };
  return {
    handleAdjustQuota, handleToggleStatus, handleUpdateGroup, handleUpdateRole
  };
}
