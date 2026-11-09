export default function useConsoleWorkspaceReset({ resetOverlays, getData, getUi }) {
  const resetWorkspaceState = () => {
    const data = getData();
    const ui = getUi();
    // The console data hook owns its own slices, including stats, prices and
    // pricingGroups, which this function used to leave behind.
    data.resetConsoleData();
    data.resetLogsData();
    data.resetChannelActions();
    data.resetKeyActions();
    ui.resetSelections();
    resetOverlays();
    // The batch selections and the channel form belong to the channel/key hooks,
    // which do not clear them on logout: they are only reset here.
    data.setSelectedLogIds([]);
    data.setSelectedKeyIds([]);
    data.setSelectedChannelIds([]);
    data.setEditingChannelId(null);
    data.setShowChannelModal(false);
    data.setShowKeyModal(false);
    // Console-shell state that is scoped to the account rather than to a view.
    // These were missed: the dashboard stayed drilled into the previous operator's
    // key row (so the next session's KPI cards reported that key, or all zeros
    // once it was deleted), and an announcement dialog left open survived into the
    // next session. The audit-log key filter is account-scoped too: a stale
    // `key_id` from another account silently filters the new account's log to
    // empty, so the cursor and key drill-down are reset with it.
    ui.setDashboardSelectedKey('');
    ui.setShowAnnouncementModal(false);
    ui.setEditingAnnouncementId(null);
    ui.setShowNotificationCompose(false);
    ui.setNotificationKind('announcement');
    data.setLogFilters({ key_id: '', page: '1' });
  };
  return {
    resetWorkspaceState
  };
}
