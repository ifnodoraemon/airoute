// The API key domain: the create form, the per-row operations, the batch
// operations, and the copy feedback.
//
// Extracted from App.jsx alongside the channel domain. Credentials are always
// generated server-side; the browser never invents an authentication secret.

import { useCallback, useState } from 'react';
import { apiFetch, apiJSON } from '../api/client';
import { copyToClipboard as crossOriginCopy } from '../config';

function emptyKey(adminUser, isZh, keyCount) {
  const tenantNum = (keyCount || 0) + 1;
  return {
    tenant_id: isZh ? `密钥-${tenantNum}` : `Key-${tenantNum}`,
    key: '',
    rpm: 0,
    budget: 0,
    allowed_models: [],
    group_name: adminUser?.role === 'admin' ? 'default' : (adminUser?.group_name || 'default'),
  };
}

export function useKeyActions({ keys = [], adminUser, showToast, isZh, refresh }) {
  const [showKeyModal, setShowKeyModal] = useState(false);
  const [newKey, setNewKey] = useState({ tenant_id: '', key: '', rpm: 60 });
  const [submittingKey, setSubmittingKey] = useState(false);
  const [selectedKeyIds, setSelectedKeyIds] = useState([]);
  const [copiedKey, setCopiedKey] = useState('');

  // Handle Key Modal Opening. Credentials are always generated server-side with
  // crypto/rand; the browser never invents an authentication secret.
  const handleOpenCreateKey = useCallback(() => {
    setNewKey(emptyKey(adminUser, isZh, keys?.length || 0));
    setShowKeyModal(true);
  }, [adminUser, isZh, keys]);

  // Handle Key Creation
  const handleCreateKey = useCallback(async (e) => {
    if (e && e.preventDefault) e.preventDefault();
    if (submittingKey) return;
    if (!newKey.tenant_id.trim()) {
      showToast(isZh ? '请输入密钥名称' : 'Please enter key name', 'warning');
      return;
    }
    setSubmittingKey(true);
    try {
      const res = await apiFetch('/api/v1/admin/keys', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(newKey),
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(isZh ? 'API 访问密钥创建成功' : 'API access key created successfully', 'success');
        setShowKeyModal(false);
        // fetchKeys() did not exist, so the success path threw a ReferenceError and
        // the list never refreshed. The console loader is the real refresh.
        refresh();
      } else {
        showToast((isZh ? '创建失败: ' : 'Creation failed: ') + (data.error || (isZh ? '未知错误' : 'Unknown error')), 'error');
      }
    } catch (e) {
      showToast((isZh ? '请求异常: ' : 'Request error: ') + e.message, 'error');
    } finally {
      setSubmittingKey(false);
    }
  }, [submittingKey, newKey, isZh, showToast, refresh]);

  // Handle Key Group Update
  const handleUpdateKeyGroup = useCallback(async (keyId, newGroup) => {
    try {
      const res = await apiFetch(`/api/v1/admin/keys/${keyId}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ group_name: newGroup }),
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(isZh ? `密钥计费分组已成功调整为 [${newGroup}]` : `Key billing group updated to [${newGroup}]`, 'success');
        refresh();
      } else {
        showToast((isZh ? '调整分组失败: ' : 'Update group failed: ') + (data.error || (isZh ? '未知错误' : 'Unknown error')), 'error');
      }
    } catch (err) {
      showToast((isZh ? '请求异常: ' : 'Request error: ') + err.message, 'error');
    }
  }, [isZh, showToast, refresh]);

  // Handle Key Deletion
  const handleDeleteKey = useCallback(async (id) => {
    if (!window.confirm(isZh ? '确认注销该客户端访问密钥？' : 'Are you sure you want to revoke this API key?')) return;
    try {
      await apiJSON(`/api/v1/admin/keys/${id}`, { method: 'DELETE' });
      showToast(isZh ? '客户端访问密钥已注销' : 'Client API key revoked', 'info');
      refresh();
    } catch (err) {
      showToast((isZh ? '注销密钥失败: ' : 'Failed to revoke key: ') + err.message, 'error');
    }
  }, [isZh, showToast, refresh]);

  // Handle Key Status Toggle (Enable/Disable)
  const handleToggleKeyStatus = useCallback(async (k) => {
    const nextStatus = k.status === 'disabled' ? 'active' : 'disabled';
    const actionText = nextStatus === 'active' ? (isZh ? '启用' : 'enable') : (isZh ? '禁用/停用' : 'disable');
    try {
      await apiJSON(`/api/v1/admin/keys/${k.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ status: nextStatus }),
      });
      showToast(
        isZh
          ? `已${actionText}密钥 ${k.key}，集群毫秒级热同步完成`
          : `Key ${k.key} status set to ${nextStatus}, cluster synced`,
        'success'
      );
      refresh();
    } catch (err) {
      showToast(`${actionText} ${isZh ? '失败: ' : 'failed: '}${err.message}`, 'error');
    }
  }, [isZh, showToast, refresh]);

  const handleBatchDeleteKeys = useCallback(async () => {
    if (selectedKeyIds.length === 0) return;
    if (!window.confirm(isZh ? `确认批量注销选中的 ${selectedKeyIds.length} 个 API 密钥？` : `Confirm revoke selected ${selectedKeyIds.length} API keys?`)) return;
    try {
      const res = await apiFetch('/api/v1/admin/keys/batch-delete', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids: selectedKeyIds })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(data.message || (isZh ? '已批量注销密钥' : 'Selected keys revoked'), 'success');
        setSelectedKeyIds([]);
        refresh();
      } else {
        showToast(data.error || (isZh ? '批量注销失败' : 'Failed to revoke keys'), 'error');
      }
    } catch (e) {
      showToast((isZh ? '请求异常: ' : 'Request error: ') + e.message, 'error');
    }
  }, [selectedKeyIds, isZh, showToast, refresh]);

  const handleBatchStatusKeys = useCallback(async (status) => {
    if (selectedKeyIds.length === 0) return;
    try {
      const res = await apiFetch('/api/v1/admin/keys/batch-status', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids: selectedKeyIds, status })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(data.message || (isZh ? '批量状态更新成功' : 'Batch status update successful'), 'success');
        setSelectedKeyIds([]);
        refresh();
      } else {
        showToast(data.error || (isZh ? '更新失败' : 'Update failed'), 'error');
      }
    } catch (e) {
      showToast((isZh ? '请求异常: ' : 'Request error: ') + e.message, 'error');
    }
  }, [selectedKeyIds, isZh, showToast, refresh]);

  // Copy text helper
  const copyToClipboard = useCallback(async (txt) => {
    await crossOriginCopy(txt);
    setCopiedKey(txt);
    showToast(isZh ? '已成功复制到剪贴板！' : 'Copied to clipboard!', 'success');
    setTimeout(() => setCopiedKey(''), 2500);
  }, [isZh, showToast]);

  const reset = useCallback(() => {
    setShowKeyModal(false);
    setSelectedKeyIds([]);
    setCopiedKey('');
  }, []);

  return {
    showKeyModal,
    newKey,
    submittingKey,
    selectedKeyIds,
    copiedKey,
    setNewKey,
    setShowKeyModal,
    setSelectedKeyIds,
    setCopiedKey,
    handleOpenCreateKey,
    handleCreateKey,
    handleUpdateKeyGroup,
    handleDeleteKey,
    handleToggleKeyStatus,
    handleBatchDeleteKeys,
    handleBatchStatusKeys,
    copyToClipboard,
    reset,
  };
}
