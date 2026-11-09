import { useEffect, useCallback } from 'react';
import { copyToClipboard } from '../config';

export default function useSkillActions({
  adminFetch, editingSkillId, isZh, newSkill, onCopy, savingSkill, setCopiedKey, setCreateModalOpen,
  setLoading, setNewSkill, setSavingSkill, setSkills, setStorageStatus, setTogglingSkill, showToast
}) {
  const fetchSkills = useCallback(async () => {
    if (!adminFetch) return;
    setLoading(true);
    try {
      const res = await adminFetch('/api/v1/admin/skills');
      if (res.ok) {
        const data = await res.json();
        if (data.code === 0 && data.data) {
          setSkills(data.data);
        }
      }
    } catch (err) {
      console.warn('Failed to load skills:', err);
    } finally {
      setLoading(false);
    }
  }, [adminFetch, setLoading, setSkills]);

  const fetchStorageStatus = useCallback(async () => {
    if (!adminFetch) return;
    try {
      const res = await adminFetch('/api/v1/admin/storage/status');
      if (res.ok) {
        const data = await res.json();
        if (data.code === 0 && data.data) {
          setStorageStatus(data.data);
        }
      }
    } catch (err) {
      console.warn('Failed to load storage status:', err);
    }
  }, [adminFetch, setStorageStatus]);

  useEffect(() => {
    fetchSkills();
    fetchStorageStatus();
  }, [fetchSkills, fetchStorageStatus]);

  const handleToggleSkill = async (skillId, currentEnabled) => {
    if (!adminFetch) return;
    setTogglingSkill(prev => ({ ...prev, [skillId]: true }));
    try {
      const res = await adminFetch(`/api/v1/admin/skills/${skillId}/toggle`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ enabled: !currentEnabled })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        setSkills(prev => prev.map(s => s.id === skillId ? { ...s, enabled: !currentEnabled } : s));
        if (showToast) showToast(isZh ? `技能 [${skillId}] 已${!currentEnabled ? '启用' : '禁用'}` : `Skill [${skillId}] ${!currentEnabled ? 'enabled' : 'disabled'}`, 'success');
      } else {
        if (showToast) showToast(data.error || (isZh ? '切换技能状态失败' : 'Failed to toggle skill status'), 'error');
      }
    } catch (err) {
      if (showToast) showToast((isZh ? '请求异常: ' : 'Request error: ') + err.message, 'error');
    } finally {
      setTogglingSkill(prev => ({ ...prev, [skillId]: false }));
    }
  };

  const handleCreateSkill = async (e) => {
    e.preventDefault();
    if (!adminFetch || savingSkill) return;
    if (!newSkill.name.trim() || !newSkill.description.trim()) {
      if (showToast) showToast(isZh ? '技能名称与描述不能为空' : 'Skill name and description cannot be empty', 'error');
      return;
    }

    const payload = {
      ...newSkill,
      id: newSkill.id.trim() || `skill_${Date.now()}`,
      tools: newSkill.tools ? newSkill.tools.split(/[,，\n]/).map(s => s.trim()).filter(Boolean) : ['run_command', 'view_file'],
      enabled: editingSkillId ? newSkill.enabled : true
    };

    setSavingSkill(true);
    try {
      const res = await adminFetch('/api/v1/admin/skills', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        if (showToast) showToast(isZh ? '技能已保存' : 'Skill saved', 'success');
        setCreateModalOpen(false);
        setNewSkill({
          id: '',
          name: '',
          category: 'dev',
          description: '',
          author: 'Community Standard',
          version: '1.0.0',
          loading_mode: 'lazy',
          tools: '',
          manifest: ''
        });
        fetchSkills();
      } else {
        if (showToast) showToast(data.error || (isZh ? '创建失败' : 'Creation failed'), 'error');
      }
    } catch (err) {
      if (showToast) showToast((isZh ? '请求异常: ' : 'Request error: ') + err.message, 'error');
    } finally {
      setSavingSkill(false);
    }
  };

  const handleDeleteSkill = async (skillId) => {
    if (!adminFetch) return;
    if (!window.confirm(isZh ? `确定要删除技能 [${skillId}] 吗？` : `Confirm delete skill [${skillId}]?`)) return;
    try {
      const res = await adminFetch(`/api/v1/admin/skills/${skillId}`, {
        method: 'DELETE'
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        setSkills(prev => prev.filter(s => s.id !== skillId));
        if (showToast) showToast(isZh ? `技能 [${skillId}] 已删除` : `Skill [${skillId}] deleted`, 'success');
      } else {
        if (showToast) showToast(data.error || (isZh ? '删除失败' : 'Delete failed'), 'error');
      }
    } catch (err) {
      if (showToast) showToast((isZh ? '请求异常: ' : 'Request error: ') + err.message, 'error');
    }
  };

  const handleCopyText = async (text, key) => {
    if (onCopy) {
      onCopy(text);
    } else {
      await copyToClipboard(text);
    }
    setCopiedKey(key);
    setTimeout(() => setCopiedKey(''), 2000);
    if (showToast) showToast(isZh ? '已复制到剪贴板' : 'Copied to clipboard', 'success');
  };
  return {
    handleCopyText, handleCreateSkill, handleDeleteSkill, handleToggleSkill
  };
}
