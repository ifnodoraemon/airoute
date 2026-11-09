
export default function useConsoleLogout({
  isZh, resetWorkspaceState, showToast, signOutLocal
}) {
  const handleLogout = async () => {
    try {
      await fetch('/api/v1/auth/logout', { method: 'POST', credentials: 'same-origin' });
    } catch (e) {
      // Even if the call fails, drop all local state below.
    }
    signOutLocal();
    resetWorkspaceState();
    showToast(isZh ? '已安全退出账号' : 'Successfully logged out', 'info');
  };
  return {
    handleLogout
  };
}
