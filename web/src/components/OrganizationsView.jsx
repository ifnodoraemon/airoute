import { useCallback, useEffect, useMemo, useState } from 'react';
import { Building2, RefreshCw, Plus, Users, ArrowRightLeft, Archive, Trash2 } from 'lucide-react';

const LEVEL_LABELS = {
  zh: { unit: '单位', department: '部门', project: '项目' },
  en: { unit: 'Unit', department: 'Department', project: 'Project' },
};

const LEVEL_BADGES = {
  unit: 'bg-indigo-50 text-indigo-700 dark:bg-indigo-950/40 dark:text-indigo-300',
  department: 'bg-sky-50 text-sky-700 dark:bg-sky-950/40 dark:text-sky-300',
  project: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300',
};

const emptyDraft = { level: 'unit', parent_id: '', name: '', code: '' };

// OrganizationsView is the console surface for the three-level organization
// tree. It deliberately keeps the tree flat and indents by the server-provided
// depth: the list endpoint is already ordered by materialized path, so the
// client never has to guess the hierarchy.
export default function OrganizationsView({ adminFetch, showToast, lang = 'zh' }) {
  const isZh = lang === 'zh';
  const labels = LEVEL_LABELS[isZh ? 'zh' : 'en'];
  const [orgs, setOrgs] = useState([]);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [selectedId, setSelectedId] = useState(null);
  const [members, setMembers] = useState([]);
  const [draft, setDraft] = useState(emptyDraft);
  const [editName, setEditName] = useState('');
  const [moveParent, setMoveParent] = useState('');
  const [memberDraft, setMemberDraft] = useState({ user_id: '', role: 'member' });

  const request = useCallback(async (path, options) => {
    const res = await adminFetch(path, options);
    const body = await res.json().catch(() => ({}));
    if (!res.ok || body.code !== 0) {
      throw new Error(body.error || `HTTP ${res.status}`);
    }
    return body.data;
  }, [adminFetch]);

  const notify = useCallback((message) => {
    if (showToast) showToast(message, 'success');
  }, [showToast]);

  const fail = useCallback((err) => {
    if (showToast) showToast(err?.message || String(err), 'error');
  }, [showToast]);

  const loadOrgs = useCallback(async () => {
    if (!adminFetch) return;
    setLoading(true);
    try {
      const data = await request('/api/v1/admin/orgs');
      setOrgs(Array.isArray(data) ? data : []);
    } catch (err) {
      fail(err);
    } finally {
      setLoading(false);
    }
  }, [adminFetch, request, fail]);

  useEffect(() => {
    loadOrgs();
  }, [loadOrgs]);

  const selected = useMemo(
    () => orgs.find((org) => String(org.id) === String(selectedId)) || null,
    [orgs, selectedId],
  );

  const loadMembers = useCallback(async (orgId) => {
    try {
      const data = await request(`/api/v1/admin/orgs/${orgId}/members`);
      setMembers(Array.isArray(data) ? data : []);
    } catch {
      setMembers([]);
    }
  }, [request]);

  useEffect(() => {
    if (selectedId) loadMembers(selectedId);
    else setMembers([]);
  }, [selectedId, loadMembers]);

  const parentOptions = useMemo(() => {
    if (draft.level === 'unit') return [];
    const wanted = draft.level === 'department' ? 'unit' : 'department';
    return orgs.filter((org) => org.level === wanted && org.status === 'active');
  }, [orgs, draft.level]);

  const moveOptions = useMemo(() => {
    if (!selected || selected.level === 'unit') return [];
    const wanted = selected.level === 'department' ? 'unit' : 'department';
    return orgs.filter((org) => org.level === wanted && org.status === 'active' && org.id !== selected.id);
  }, [orgs, selected]);

  const selectOrg = (org) => {
    setSelectedId(org.id);
    setEditName(org.name);
    setMoveParent(String(org.parent_id || ''));
  };

  const createOrg = async (event) => {
    event.preventDefault();
    if (!adminFetch) return;
    setBusy(true);
    try {
      const payload = {
        level: draft.level,
        name: draft.name.trim(),
        code: draft.code.trim(),
        parent_id: draft.level === 'unit' ? 0 : Number(draft.parent_id),
      };
      await request('/api/v1/admin/orgs', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      notify(isZh ? '组织已创建' : 'Organization created');
      setDraft(emptyDraft);
      await loadOrgs();
    } catch (err) {
      fail(err);
    } finally {
      setBusy(false);
    }
  };

  const updateSelected = async (patch, successMessage) => {
    if (!selected || !adminFetch) return;
    setBusy(true);
    try {
      await request(`/api/v1/admin/orgs/${selected.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(patch),
      });
      notify(successMessage);
      await loadOrgs();
    } catch (err) {
      fail(err);
    } finally {
      setBusy(false);
    }
  };

  const moveSelected = async () => {
    if (!selected || !moveParent || !adminFetch) return;
    setBusy(true);
    try {
      await request(`/api/v1/admin/orgs/${selected.id}/move`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ parent_id: Number(moveParent) }),
      });
      notify(isZh ? '组织已移动' : 'Organization moved');
      await loadOrgs();
    } catch (err) {
      fail(err);
    } finally {
      setBusy(false);
    }
  };

  const addMember = async (event) => {
    event.preventDefault();
    if (!selected || !adminFetch || !memberDraft.user_id) return;
    setBusy(true);
    try {
      const data = await request(`/api/v1/admin/orgs/${selected.id}/members`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ user_id: Number(memberDraft.user_id), role: memberDraft.role }),
      });
      setMembers(Array.isArray(data) ? data : []);
      setMemberDraft({ user_id: '', role: 'member' });
      notify(isZh ? '成员已加入' : 'Member added');
    } catch (err) {
      fail(err);
    } finally {
      setBusy(false);
    }
  };

  const removeMember = async (userId) => {
    if (!selected || !adminFetch) return;
    setBusy(true);
    try {
      await request(`/api/v1/admin/orgs/${selected.id}/members/${userId}`, { method: 'DELETE' });
      setMembers((prev) => prev.filter((member) => member.user_id !== userId));
      notify(isZh ? '成员已移除' : 'Member removed');
    } catch (err) {
      fail(err);
    } finally {
      setBusy(false);
    }
  };

  const inputClass = 'w-full px-2.5 py-1.5 rounded-lg border border-[#E4E6E9] dark:border-[#2C3038] bg-white dark:bg-[#191B1F] text-xs text-[#202124] dark:text-[#F1F2F4]';
  const buttonClass = 'inline-flex items-center space-x-1.5 px-2.5 py-1.5 rounded-lg bg-[#355CFF] hover:bg-[#2A4AE0] text-white text-xs font-medium transition disabled:opacity-50';

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center space-x-2">
          <Building2 className="w-5 h-5 text-[#355CFF] dark:text-[#91A7FF]" />
          <div>
            <h2 className="text-sm font-semibold text-[#202124] dark:text-[#F1F2F4]">
              {isZh ? '组织与成员' : 'Organizations & Members'}
            </h2>
            <p className="text-[11px] text-[#656B75] dark:text-[#A1A7B0]">
              {isZh ? '单位 → 部门 → 项目；新密钥默认归属属主所在组织。' : 'Unit → department → project. New keys inherit the owner organization.'}
            </p>
          </div>
        </div>
        <button type="button" onClick={loadOrgs} disabled={loading} className="inline-flex items-center space-x-1.5 px-2.5 py-1.5 rounded-lg border border-[#E4E6E9] dark:border-[#2C3038] text-xs text-[#656B75] dark:text-[#A1A7B0] disabled:opacity-50">
          <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
          <span>{isZh ? '刷新' : 'Refresh'}</span>
        </button>
      </div>

      <form onSubmit={createOrg} className="grid grid-cols-1 md:grid-cols-6 gap-2 p-3 rounded-xl border border-[#E4E6E9] dark:border-[#2C3038] bg-white dark:bg-[#191B1F]">
        <select value={draft.level} onChange={(e) => setDraft((prev) => ({ ...prev, level: e.target.value, parent_id: '' }))} className={inputClass} aria-label={isZh ? '层级' : 'Level'}>
          <option value="unit">{labels.unit}</option>
          <option value="department">{labels.department}</option>
          <option value="project">{labels.project}</option>
        </select>
        <select value={draft.parent_id} onChange={(e) => setDraft((prev) => ({ ...prev, parent_id: e.target.value }))} className={inputClass} disabled={draft.level === 'unit'} aria-label={isZh ? '上级组织' : 'Parent organization'}>
          <option value="">{isZh ? '上级组织' : 'Parent'}</option>
          {parentOptions.map((org) => (
            <option key={org.id} value={org.id}>{org.name}</option>
          ))}
        </select>
        <input value={draft.name} onChange={(e) => setDraft((prev) => ({ ...prev, name: e.target.value }))} className={inputClass} placeholder={isZh ? '名称' : 'Name'} aria-label={isZh ? '名称' : 'Name'} />
        <input value={draft.code} onChange={(e) => setDraft((prev) => ({ ...prev, code: e.target.value }))} className={inputClass} placeholder={isZh ? '编码（唯一）' : 'Code (unique)'} aria-label={isZh ? '编码' : 'Code'} />
        <button type="submit" disabled={busy || !draft.name.trim() || !draft.code.trim() || (draft.level !== 'unit' && !draft.parent_id)} className={buttonClass}>
          <Plus className="w-3.5 h-3.5" />
          <span>{isZh ? '创建' : 'Create'}</span>
        </button>
      </form>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <div className="rounded-xl border border-[#E4E6E9] dark:border-[#2C3038] bg-white dark:bg-[#191B1F] overflow-hidden">
          <div className="px-3 py-2 border-b border-[#E4E6E9] dark:border-[#2C3038] text-xs font-semibold text-[#202124] dark:text-[#F1F2F4]">
            {isZh ? '组织树' : 'Organization tree'} <span className="font-mono text-[#656B75] dark:text-[#A1A7B0]">({orgs.length})</span>
          </div>
          <div className="max-h-[520px] overflow-y-auto">
            {orgs.length === 0 && (
              <div className="p-6 text-center text-xs text-[#656B75] dark:text-[#A1A7B0]">{isZh ? '暂无组织' : 'No organizations yet'}</div>
            )}
            {orgs.map((org) => (
              <button
                key={org.id}
                type="button"
                onClick={() => selectOrg(org)}
                className={`w-full text-left flex items-center justify-between px-3 py-2 border-b border-[#F0F1F2] dark:border-[#22252B] transition-colors ${String(selectedId) === String(org.id) ? 'bg-[#EEF2FF] dark:bg-[#242D48]' : 'hover:bg-[#F7F8FA] dark:hover:bg-[#202329]'}`}
                style={{ paddingLeft: `${12 + (org.depth || 0) * 18}px` }}
              >
                <span className="flex items-center space-x-2 min-w-0">
                  <span className={`px-1.5 py-0.5 rounded text-[10px] font-medium ${LEVEL_BADGES[org.level] || LEVEL_BADGES.unit}`}>{labels[org.level] || org.level}</span>
                  <span className="truncate text-xs text-[#202124] dark:text-[#F1F2F4]">{org.name}</span>
                  <span className="font-mono text-[10px] text-[#656B75] dark:text-[#A1A7B0]">{org.code}</span>
                </span>
                {org.status !== 'active' && (
                  <span className="text-[10px] text-amber-600 dark:text-amber-400">{isZh ? '已归档' : 'Archived'}</span>
                )}
              </button>
            ))}
          </div>
        </div>

        <div className="rounded-xl border border-[#E4E6E9] dark:border-[#2C3038] bg-white dark:bg-[#191B1F] p-3 space-y-3">
          {!selected && (
            <div className="py-10 text-center text-xs text-[#656B75] dark:text-[#A1A7B0]">{isZh ? '选择左侧组织进行管理' : 'Select an organization'}</div>
          )}
          {selected && (
            <>
              <div className="flex items-center justify-between">
                <div>
                  <div className="text-xs font-semibold text-[#202124] dark:text-[#F1F2F4]">{selected.name}</div>
                  <div className="font-mono text-[10px] text-[#656B75] dark:text-[#A1A7B0]">{selected.path}</div>
                </div>
                <span className={`px-1.5 py-0.5 rounded text-[10px] ${LEVEL_BADGES[selected.level] || LEVEL_BADGES.unit}`}>{labels[selected.level]}</span>
              </div>

              <div className="flex items-end space-x-2">
                <label className="flex-1 text-[11px] text-[#656B75] dark:text-[#A1A7B0]">
                  {isZh ? '名称' : 'Name'}
                  <input value={editName} onChange={(e) => setEditName(e.target.value)} className={`${inputClass} mt-1`} />
                </label>
                <button type="button" disabled={busy || !editName.trim() || editName === selected.name} onClick={() => updateSelected({ name: editName, status: selected.status }, isZh ? '已保存' : 'Saved')} className={buttonClass}>
                  {isZh ? '保存' : 'Save'}
                </button>
                <button type="button" disabled={busy} onClick={() => updateSelected({ status: selected.status === 'active' ? 'archived' : 'active' }, isZh ? '状态已更新' : 'Status updated')} className="inline-flex items-center space-x-1.5 px-2.5 py-1.5 rounded-lg border border-[#E4E6E9] dark:border-[#2C3038] text-xs text-[#656B75] dark:text-[#A1A7B0]">
                  <Archive className="w-3.5 h-3.5" />
                  <span>{selected.status === 'active' ? (isZh ? '归档' : 'Archive') : (isZh ? '启用' : 'Activate')}</span>
                </button>
              </div>

              {selected.level !== 'unit' && (
                <div className="flex items-end space-x-2">
                  <label className="flex-1 text-[11px] text-[#656B75] dark:text-[#A1A7B0]">
                    {isZh ? '移动到' : 'Move under'}
                    <select value={moveParent} onChange={(e) => setMoveParent(e.target.value)} className={`${inputClass} mt-1`}>
                      <option value="">{isZh ? '选择新的上级' : 'Select parent'}</option>
                      {moveOptions.map((org) => (
                        <option key={org.id} value={org.id}>{org.name}</option>
                      ))}
                    </select>
                  </label>
                  <button type="button" disabled={busy || !moveParent || moveParent === String(selected.parent_id || '')} onClick={moveSelected} className={buttonClass}>
                    <ArrowRightLeft className="w-3.5 h-3.5" />
                    <span>{isZh ? '移动' : 'Move'}</span>
                  </button>
                </div>
              )}

              <div className="pt-2 border-t border-[#E4E6E9] dark:border-[#2C3038]">
                <div className="flex items-center space-x-1.5 text-xs font-semibold text-[#202124] dark:text-[#F1F2F4] mb-2">
                  <Users className="w-3.5 h-3.5" />
                  <span>{isZh ? '成员' : 'Members'} ({members.length})</span>
                </div>
                <form onSubmit={addMember} className="flex items-center space-x-2 mb-2">
                  <input value={memberDraft.user_id} onChange={(e) => setMemberDraft((prev) => ({ ...prev, user_id: e.target.value }))} className={inputClass} placeholder={isZh ? '用户 ID' : 'User ID'} aria-label={isZh ? '用户 ID' : 'User ID'} />
                  <select value={memberDraft.role} onChange={(e) => setMemberDraft((prev) => ({ ...prev, role: e.target.value }))} className={inputClass} aria-label={isZh ? '角色' : 'Role'}>
                    <option value="member">{isZh ? '成员' : 'Member'}</option>
                    <option value="admin">{isZh ? '管理员' : 'Admin'}</option>
                  </select>
                  <button type="submit" disabled={busy || !memberDraft.user_id} className={buttonClass}>{isZh ? '加入' : 'Add'}</button>
                </form>
                <div className="space-y-1 max-h-40 overflow-y-auto">
                  {members.map((member) => (
                    <div key={member.user_id} className="flex items-center justify-between px-2 py-1 rounded bg-[#F7F8FA] dark:bg-[#202329]">
                      <span className="text-xs text-[#202124] dark:text-[#F1F2F4]">
                        {member.username || `#${member.user_id}`}
                        <span className="ml-2 text-[10px] text-[#656B75] dark:text-[#A1A7B0]">{member.role}</span>
                      </span>
                      <button type="button" onClick={() => removeMember(member.user_id)} className="text-[#656B75] hover:text-red-500" title={isZh ? '移除' : 'Remove'}>
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  ))}
                  {members.length === 0 && <div className="text-[11px] text-[#656B75] dark:text-[#A1A7B0]">{isZh ? '暂无成员' : 'No members'}</div>}
                </div>
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
