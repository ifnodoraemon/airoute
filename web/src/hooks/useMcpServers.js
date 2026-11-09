import { useEffect, useCallback } from 'react';

export default function useMcpServers({
  adminFetch, isZh, mcpSettings, newServer, savingServer, setAddServerModalOpen, setLoading, setMcpSettings,
  setNewServer, setSavingServer, setServers, setTogglingGlobal, setTogglingServer, showToast, togglingGlobal
}) {
  const fetchServersAndSettings = useCallback(async () => {
    if (!adminFetch) return;
    setLoading(true);
    try {
      // 1. MCP Settings
      const setRes = await adminFetch('/api/v1/admin/mcp/settings');
      if (setRes.ok) {
        const setData = await setRes.json();
        if (setData.code === 0 && setData.data) {
          setMcpSettings(setData.data);
        }
      }

      // 2. MCP Servers list
      const srvRes = await adminFetch('/api/v1/admin/mcp/servers');
      if (srvRes.ok) {
        const srvData = await srvRes.json();
        if (srvData.code === 0 && srvData.data) {
          setServers(srvData.data);
        }
      }
    } catch (e) {
      console.warn('Failed to load MCP servers:', e);
    } finally {
      setLoading(false);
    }
  }, [adminFetch, setLoading, setMcpSettings, setServers]);

  useEffect(() => {
    fetchServersAndSettings();
  }, [fetchServersAndSettings]);

  const handleToggleGlobalMcp = async () => {
    if (!adminFetch || togglingGlobal) return;
    setTogglingGlobal(true);
    const nextState = !mcpSettings.mcp_enabled;
    try {
      const res = await adminFetch('/api/v1/admin/mcp/toggle', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ mcp_enabled: nextState })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        setMcpSettings(prev => ({ ...prev, mcp_enabled: nextState }));
        if (showToast) showToast(isZh ? `MCP 协议网关服务已${nextState ? '启用' : '关闭'}` : `MCP gateway service ${nextState ? 'enabled' : 'disabled'}`, 'success');
      } else {
        if (showToast) showToast(data.error || (isZh ? '切换失败' : 'Failed to toggle service'), 'error');
      }
    } catch (err) {
      if (showToast) showToast((isZh ? '请求异常: ' : 'Request error: ') + err.message, 'error');
    } finally {
      setTogglingGlobal(false);
    }
  };

  const handleToggleServer = async (serverId, currentEnabled) => {
    if (!adminFetch) return;
    setTogglingServer(prev => ({ ...prev, [serverId]: true }));
    try {
      const res = await adminFetch(`/api/v1/admin/mcp/servers/${serverId}/toggle`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ enabled: !currentEnabled })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        setServers(prev => prev.map(s => s.id === serverId ? { ...s, enabled: !currentEnabled } : s));
        if (showToast) showToast(isZh ? `MCP 服务 [${serverId}] 已${!currentEnabled ? '开启' : '关闭'}` : `MCP service [${serverId}] ${!currentEnabled ? 'enabled' : 'disabled'}`, 'success');
        fetchServersAndSettings();
      } else {
        if (showToast) showToast(data.error || (isZh ? '切换服务状态失败' : 'Failed to toggle server state'), 'error');
      }
    } catch (err) {
      if (showToast) showToast((isZh ? '请求异常: ' : 'Request error: ') + err.message, 'error');
    } finally {
      setTogglingServer(prev => ({ ...prev, [serverId]: false }));
    }
  };

  const handleDeleteServer = async (serverId) => {
    if (!adminFetch) return;
    if (!window.confirm(isZh ? `确定要移除自定义 MCP 服务 [${serverId}] 吗？` : `Are you sure you want to remove MCP service [${serverId}]?`)) return;
    try {
      const res = await adminFetch(`/api/v1/admin/mcp/servers/${serverId}`, {
        method: 'DELETE'
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        setServers(prev => prev.filter(s => s.id !== serverId));
        if (showToast) showToast(isZh ? `MCP 服务 [${serverId}] 已成功移除` : `MCP service [${serverId}] removed successfully`, 'success');
        fetchServersAndSettings();
      } else {
        if (showToast) showToast(data.error || (isZh ? '删除失败' : 'Failed to delete service'), 'error');
      }
    } catch (err) {
      if (showToast) showToast((isZh ? '请求异常: ' : 'Request error: ') + err.message, 'error');
    }
  };

  const handleCreateServer = async (e) => {
    e.preventDefault();
    if (!adminFetch || savingServer) return;
    if (!newServer.name || !newServer.endpoint) {
      if (showToast) showToast(isZh ? '请填写服务名称与 Endpoint' : 'Please fill in service name and Endpoint', 'error');
      return;
    }

    const payload = {
      ...newServer,
      tools: newServer.tools ? newServer.tools.split(/[,，\n]/).map(t => t.trim()).filter(Boolean) : [],
      prompts: newServer.prompts ? newServer.prompts.split(/[,，\n]/).map(p => p.trim()).filter(Boolean) : [],
      resources: newServer.resources ? newServer.resources.split(/[,，\n]/).map(r => r.trim()).filter(Boolean) : []
    };

    setSavingServer(true);
    try {
      const res = await adminFetch('/api/v1/admin/mcp/servers', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        if (showToast) showToast(isZh ? 'MCP 服务配置已保存' : 'MCP server configuration saved', 'success');
        setAddServerModalOpen(false);
        setNewServer({
          id: '',
          name: '',
          description: '',
          category: 'dev',
          transport: 'stdio',
          endpoint: '',
          author: 'Community',
          version: '1.0.0',
          tools: '',
          prompts: '',
          resources: '',
          env_vars: '',
          enabled: true
        });
        fetchServersAndSettings();
      } else {
        if (showToast) showToast(data.error || (isZh ? '创建 MCP 服务失败' : 'Failed to create MCP service'), 'error');
      }
    } catch (err) {
      if (showToast) showToast((isZh ? '请求异常: ' : 'Request error: ') + err.message, 'error');
    } finally {
      setSavingServer(false);
    }
  };
  return {
    fetchServersAndSettings, handleCreateServer, handleDeleteServer, handleToggleGlobalMcp, handleToggleServer
  };
}
