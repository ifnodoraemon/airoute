// The API key domain: the create form, the per-row operations, the batch
// operations, and the copy feedback.
//
// Extracted from App.jsx alongside the channel domain. Credentials are always
// generated server-side; the browser never invents an authentication secret.

import { useCallback, useState } from 'react';
import { apiFetch, apiJSON } from '../api/client';
import { copyToClipboard as crossOriginCopy } from '../config';

// Self-service key quotas. The gateway applies these to keys created through the
// personal-key endpoint, and clamps rather than rejects: the form states the same
// numbers up front instead of pretending 0 means "unlimited" for everyone.
// An unlimited key is an administrator decision, so only the admin form offers it.
export const SELF_SERVICE_DEFAULT_RPM = 60;
export const SELF_SERVICE_MAX_RPM = 6000;
export const SELF_SERVICE_DEFAULT_TPM = 100000;
export const SELF_SERVICE_MAX_TPM = 10000000;

export function clampSelfServiceRpm(value) {
  const parsed = Number.isFinite(Number(value)) ? Math.floor(Number(value)) : 0;
  if (parsed <= 0) return SELF_SERVICE_DEFAULT_RPM;
  return Math.min(parsed, SELF_SERVICE_MAX_RPM);
}

export function clampSelfServiceTpm(value) {
  const parsed = Number.isFinite(Number(value)) ? Math.floor(Number(value)) : 0;
  if (parsed <= 0) return SELF_SERVICE_DEFAULT_TPM;
  return Math.min(parsed, SELF_SERVICE_MAX_TPM);
}

function emptyKey(adminUser, isZh, keyCount) {
  const isAdmin = adminUser?.role === 'admin';
  const tenantNum = (keyCount || 0) + 1;
  return {
    // Only an administrator chooses the tenant; for a self-service key the gateway
    // derives it from the account, so the form keeps it empty and never sends it.
    tenant_id: isAdmin ? (isZh ? `密钥-${tenantNum}` : `Key-${tenantNum}`) : '',
    key: '',
    // 0 means "unlimited" for an admin-created key only.
    rpm: isAdmin ? 0 : SELF_SERVICE_DEFAULT_RPM,
    tpm: isAdmin ? 0 : SELF_SERVICE_DEFAULT_TPM,
    budget: 0,
    allowed_models: [],
    group_name: isAdmin ? 'default' : (adminUser?.group_name || 'default'),
  };
}

export function useKeyActions({ keys = [], adminUser, showToast, isZh, refresh }) {
  const [showKeyModal, setShowKeyModal] = useState(false);
  const [newKey, setNewKey] = useState({ tenant_id: '', key: '', rpm: 60 });
  const [submittingKey, setSubmittingKey] = useState(false);
  const [selectedKeyIds, setSelectedKeyIds] = useState([]);
  const [copiedKey, setCopiedKey] = useState('');
  // The plaintext credential of the key that was just created. The server returns it
  // exactly once; it is never read back from the list, so this is the only place the
  // console can show it (and it is dropped as soon as the dialog closes).
  const [createdKey, setCreatedKey] = useState(null);

  // Handle Key Modal Opening. Credentials are always generated server-side with
  // crypto/rand; the browser never invents an authentication secret.
  const handleOpenCreateKey = useCallback(() => {
    setNewKey(emptyKey(adminUser, isZh, keys?.length || 0));
    setCreatedKey(null);
    setShowKeyModal(true);
  }, [adminUser, isZh, keys]);

  // Handle Key Creation
  const handleCreateKey = useCallback(async (e) => {
    if (e && e.preventDefault) e.preventDefault();
    if (submittingKey) return;
    const isAdmin = adminUser?.role === 'admin';
    if (isAdmin && !(newKey.tenant_id || '').trim()) {
      showToast(isZh ? '请输入密钥名称' : 'Please enter key name', 'warning');
      return;
    }

    // The tenant comes from the session for a self-service key: sending one would be
    // ignored at best, so the form does not pretend it can choose.
    const requestedRpm = Number(newKey.rpm) || 0;
    const requestedTpm = Number(newKey.tpm) || 0;
    const payload = {
      allowed_models: newKey.allowed_models || [],
      budget: Number(newKey.budget) || 0,
      group_name: newKey.group_name,
      rpm: isAdmin ? Math.max(0, requestedRpm) : clampSelfServiceRpm(requestedRpm),
      tpm: isAdmin ? Math.max(0, requestedTpm) : clampSelfServiceTpm(requestedTpm),
    };
    if (isAdmin) {
      payload.tenant_id = newKey.tenant_id.trim();
      payload.key = newKey.key || '';
    }

    setSubmittingKey(true);
    try {
      const res = await apiFetch('/api/v1/admin/keys', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        const created = data.data || {};
        showToast(isZh ? 'API 访问密钥创建成功' : 'API access key created successfully', 'success');
        // Keep the dialog open: the plaintext credential exists only in this
        // response, so this is the single chance to show it. Re-open it explicitly:
        // a dismissal may have slipped through while the request was in flight, and
        // it must not swallow the one-time credential.
        setShowKeyModal(true);
        setCreatedKey({
          key: created.key || '',
          keyId: created.key_id || '',
          tenant: created.tenant_id || (adminUser?.username || ''),
          rpm: created.rpm ?? payload.rpm,
          tpm: created.tpm ?? payload.tpm,
          quotaAdjusted: !isAdmin && (payload.rpm !== requestedRpm || payload.tpm !== requestedTpm),
        });
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
  }, [submittingKey, newKey, adminUser, isZh, showToast, refresh]);

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
    // Name the key by its public identifier: the list no longer carries a credential.
    const keyRef = k.key_id || `#${k.id}`;
    try {
      await apiJSON(`/api/v1/admin/keys/${k.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ status: nextStatus }),
      });
      showToast(
        isZh
          ? `已${actionText}密钥 ${keyRef}，集群毫秒级热同步完成`
          : `Key ${keyRef} status set to ${nextStatus}, cluster synced`,
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

  // Copy text helper. The clipboard helper reports whether the copy really
  // happened; a failed copy must never be announced as a success, because the
  // caller may then close the one-time-credential dialog believing the secret
  // was saved.
  const copyToClipboard = useCallback(async (txt) => {
    const copied = await crossOriginCopy(txt);
    if (!copied) {
      showToast(
        isZh ? '复制失败，请手动选中并复制' : 'Copy failed — select the text and copy it manually',
        'error'
      );
      return false;
    }
    setCopiedKey(txt);
    showToast(isZh ? '已成功复制到剪贴板！' : 'Copied to clipboard!', 'success');
    setTimeout(() => setCopiedKey(''), 2500);
    return true;
  }, [isZh, showToast]);

  const reset = useCallback(() => {
    setShowKeyModal(false);
    setCreatedKey(null);
    setSelectedKeyIds([]);
    setCopiedKey('');
  }, []);

  return {
    showKeyModal,
    newKey,
    submittingKey,
    selectedKeyIds,
    copiedKey,
    createdKey,
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
