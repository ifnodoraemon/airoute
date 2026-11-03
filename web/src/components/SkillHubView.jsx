import { useState, useEffect, useCallback } from 'react';
import {
  Sparkles,
  RefreshCw,
  BookOpen,
  FileText,
  Check,
  Copy,
  Plus,
  Search,
  Trash2,
  Pencil,
  Play,
  X,
  ShieldCheck,
  Terminal,
  Code2,
  Workflow,
  DownloadCloud,
  GitBranch,
  Bug,
  Globe,
  Lock,
  ArrowRight,
  HardDrive,
  Server,
  Folder,
  FolderTree
} from 'lucide-react';
import { getGatewayOrigin, copyToClipboard } from '../config';
import useModalA11y from '../hooks/useModalA11y';
import SkillTrialModal from './SkillTrialModal';

export default function SkillHubView({ adminFetch, onCopy, onTrial, showToast, lang = 'zh', t }) {
  const isZh = lang === 'zh';
  const [skills, setSkills] = useState([]);
  const [loading, setLoading] = useState(false);
  const [selectedCategory, setSelectedCategory] = useState('all');
  const [searchQuery, setSearchQuery] = useState('');
  const [togglingSkill, setTogglingSkill] = useState({});
  const [storageStatus, setStorageStatus] = useState(null);

  // Modals
  const [manifestSkill, setManifestSkill] = useState(null);
  const [manifestTab, setManifestTab] = useState('skill_md'); // 'skill_md' | 'bundle_tree' | 'metadata'
  const [clientGuideModalOpen, setClientGuideModalOpen] = useState(false);
  const [selectedSkillForGuide, setSelectedSkillForGuide] = useState(null);
  const [activeClientTab, setActiveClientTab] = useState('claude-code'); // 'claude-code' | 'opencode' | 'codex' | 'cursor'

  const [testModalSkill, setTestModalSkill] = useState(null);
  const [createModalOpen, setCreateModalOpen] = useState(false);
  const [editingSkillId, setEditingSkillId] = useState('');
  const [savingSkill, setSavingSkill] = useState(false);

  useModalA11y(clientGuideModalOpen, () => setClientGuideModalOpen(false));
  useModalA11y(!!manifestSkill, () => setManifestSkill(null));
  useModalA11y(createModalOpen, () => setCreateModalOpen(false));

  // New Skill Form
  const [newSkill, setNewSkill] = useState({
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

  const [copiedKey, setCopiedKey] = useState('');
  const origin = getGatewayOrigin();

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
  }, [adminFetch]);

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
  }, [adminFetch]);

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

  const categories = [
    { id: 'all', label: isZh ? '全部技能' : 'All Skills', icon: Workflow },
    { id: 'dev', label: isZh ? '研发工程' : 'Dev Engineering', icon: GitBranch },
    { id: 'test', label: isZh ? '质量保障 (TDD)' : 'QA & TDD', icon: Bug },
    { id: 'automation', label: isZh ? '端到端自动化' : 'E2E Automation', icon: Globe },
    { id: 'security', label: isZh ? '代码安全审计' : 'Security Audit', icon: Lock }
  ];

  const filteredSkills = skills.filter(s => {
    if (selectedCategory !== 'all' && s.category !== selectedCategory) {
      return false;
    }
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      const matchName = s.name?.toLowerCase().includes(q);
      const matchId = s.id?.toLowerCase().includes(q);
      const matchDesc = s.description?.toLowerCase().includes(q);
      const matchTools = s.tools?.some(t => t.toLowerCase().includes(q));
      return matchName || matchId || matchDesc || matchTools;
    }
    return true;
  });

  return (
    <div className="space-y-8 animate-in fade-in duration-200">
      {/* 顶部标题与轻量操作栏 */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 pb-6 border-b border-slate-200/70 dark:border-slate-800/80">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-xl font-bold tracking-tight text-slate-900 dark:text-white">
              {isZh ? 'Agent 技能中心 (Skills)' : 'Agent Skill Hub'}
            </h1>
            <span className="px-2.5 py-0.5 text-xs font-medium rounded-full bg-amber-50 dark:bg-amber-950/60 text-amber-700 dark:text-amber-300 border border-amber-200/80 dark:border-amber-800">
              agentskills.io
            </span>
            {storageStatus && (
              <span className="px-2.5 py-0.5 text-xs font-medium rounded-full bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-300 border border-slate-200/60 dark:border-slate-700/60">
                {storageStatus.driver === 's3' ? (isZh ? 'S3 存储就绪' : 'S3 Ready') : (isZh ? '本地存储' : 'Local Storage')}
              </span>
            )}
          </div>
          <p className="ui-meta mt-2 max-w-3xl leading-relaxed">
            {isZh
              ? '标准化智能体 SOP 作业包体系，支持线上代码审查、直连高速下载归档与一键接入 Claude Code、Cursor、OpenCode 等客户端。'
              : 'Standardized Agent SOP packages with online review, direct downloads, and seamless client integration.'}
          </p>
        </div>

        <div className="flex items-center gap-3 shrink-0">
          <button
            onClick={() => { setSelectedSkillForGuide(null); setClientGuideModalOpen(true); }}
            className="ui-btn-secondary shadow-2xs"
          >
            <BookOpen className="w-4 h-4 text-amber-600 dark:text-amber-400" />
            <span>{isZh ? '客户端接入指南' : 'Client Guide'}</span>
          </button>
          <button
            onClick={() => { setEditingSkillId(''); setNewSkill({ id: '', name: '', category: 'dev', description: '', author: 'Community Standard', version: '1.0.0', loading_mode: 'lazy', tools: '', manifest: '' }); setCreateModalOpen(true); }}
            className="ui-btn bg-amber-600 hover:bg-amber-700 text-white shadow-2xs"
          >
            <Plus className="w-4 h-4" />
            <span>{isZh ? '新建技能' : 'New Skill'}</span>
          </button>
        </div>
      </div>

      {/* 核心指标与规格看板 */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-5 p-6 rounded-2xl border border-slate-200/80 dark:border-slate-800 bg-white dark:bg-[#191B1F] shadow-xs">
        <div className="flex items-center gap-4">
          <div className="w-12 h-12 rounded-2xl bg-amber-50 dark:bg-amber-950/40 text-amber-600 dark:text-amber-400 border border-amber-200/60 dark:border-amber-800/60 flex items-center justify-center shrink-0 shadow-2xs">
            <Workflow className="w-6 h-6" />
          </div>
          <div>
            <div className="ui-meta">{isZh ? '已挂载 SOP 技能' : 'Available SOP Skills'}</div>
            <div className="text-xl font-bold font-mono text-slate-900 dark:text-white mt-0.5">
              {skills.filter(s => s.enabled).length} <span className="ui-meta font-normal">/ 共 {skills.length} 项</span>
            </div>
          </div>
        </div>

        <div className="flex items-center gap-4 border-t md:border-t-0 md:border-l border-slate-100 dark:border-slate-800/80 pt-4 md:pt-0 md:pl-6">
          <div className="w-12 h-12 rounded-2xl bg-[#EEF2FF] dark:bg-[#242D48] text-[#355CFF] dark:text-[#91A7FF] border border-[#355CFF]/20 dark:border-[#91A7FF]/30 flex items-center justify-center shrink-0 shadow-2xs">
            <ShieldCheck className="w-6 h-6" />
          </div>
          <div>
            <div className="ui-meta">{isZh ? '标准三阶段渐进加载' : 'Progressive Loading'}</div>
            <div className="text-xs font-semibold text-slate-800 dark:text-slate-200 mt-1 flex items-center gap-1.5 font-mono">
              <span className="px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-800 text-xs">L1 检索</span>
              <ArrowRight className="w-3 h-3 text-slate-400" />
              <span className="px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-800 text-xs">L2 SOP</span>
              <ArrowRight className="w-3 h-3 text-slate-400" />
              <span className="px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-800 text-xs">L3 执行</span>
            </div>
          </div>
        </div>

        <div className="flex items-center gap-4 border-t md:border-t-0 md:border-l border-slate-100 dark:border-slate-800/80 pt-4 md:pt-0 md:pl-6">
          <div className="w-12 h-12 rounded-2xl bg-emerald-50 dark:bg-emerald-950/40 text-emerald-600 dark:text-emerald-400 border border-emerald-200/60 dark:border-emerald-800/60 flex items-center justify-center shrink-0 shadow-2xs">
            <HardDrive className="w-6 h-6" />
          </div>
          <div>
            <div className="ui-meta">{isZh ? 'ZIP 产物归档存储' : 'Bundle Storage'}</div>
            <div className="text-xs font-medium text-slate-800 dark:text-slate-200 mt-1 flex items-center gap-1.5">
              <span className="w-2 h-2 rounded-full bg-emerald-500 inline-block animate-pulse"></span>
              <span>{storageStatus?.driver === 's3' ? (isZh ? 'S3 对象存储集群' : 'S3 Object Storage') : (isZh ? '本地高速持久存储' : 'Local Fast Storage')}</span>
            </div>
          </div>
        </div>
      </div>

      {/* 搜索与分类导航 */}
      <div className="flex flex-col md:flex-row items-stretch md:items-center justify-between gap-4 bg-white dark:bg-[#191B1F] p-2.5 rounded-2xl border border-slate-200/80 dark:border-slate-800 shadow-2xs">
        <div className="flex items-center gap-1.5 overflow-x-auto pb-1 md:pb-0 scrollbar-none px-1">
          {categories.map(cat => {
            const Icon = cat.icon;
            const active = selectedCategory === cat.id;
            return (
              <button
                key={cat.id}
                onClick={() => setSelectedCategory(cat.id)}
                className={`flex items-center gap-2 px-3.5 py-2 text-xs font-medium rounded-xl whitespace-nowrap transition-all cursor-pointer ${
                  active
                    ? 'bg-amber-600 text-white shadow-xs'
                    : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800/80'
                }`}
              >
                <Icon className="w-3.5 h-3.5" />
                <span>{cat.label}</span>
              </button>
            );
          })}
        </div>

        <div className="relative min-w-[260px] px-1">
          <Search className="w-4 h-4 text-slate-400 absolute left-3.5 top-2.5" />
          <input
            type="text"
            placeholder={isZh ? '搜索技能名称或适用工具...' : 'Search skill name or tool...'}
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="w-full pl-9 pr-3.5 py-2 text-xs rounded-xl border border-slate-200/80 dark:border-slate-700 bg-slate-50 dark:bg-slate-900/60 text-slate-900 dark:text-white placeholder-slate-400 dark:placeholder-slate-500 focus:outline-none focus:border-amber-500 transition-colors"
          />
        </div>
      </div>

      {/* 技能卡片列表 */}
      {loading && skills.length === 0 ? (
        // Without this the list rendered "No matching Agent Skills found" while the
        // first request was still in flight, which reads as a broken catalogue.
        <div className="p-16 text-center bg-white dark:bg-[#191B1F] rounded-2xl border border-slate-200 dark:border-slate-800">
          <RefreshCw className="w-8 h-8 text-[#355CFF] dark:text-[#91A7FF] mx-auto mb-3 animate-spin" />
          <p className="text-sm font-semibold text-slate-800 dark:text-slate-200">
            {isZh ? '正在加载 Agent 技能…' : 'Loading Agent Skills…'}
          </p>
        </div>
      ) : filteredSkills.length === 0 ? (
        <div className="p-16 text-center bg-white dark:bg-[#191B1F] rounded-2xl border border-dashed border-slate-200 dark:border-slate-800">
          <Sparkles className="w-10 h-10 text-slate-300 dark:text-slate-600 mx-auto mb-3" />
          <p className="text-sm font-semibold text-slate-800 dark:text-slate-200">
            {isZh ? '未找到匹配的 Agent 技能' : 'No matching Agent Skills found'}
          </p>
          <p className="ui-meta mt-1">
            {isZh ? '可在上方切换分类筛选或点击「新建技能」添加' : 'Switch category filters above or click "New Skill" to add one'}
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {filteredSkills.map(skill => {
            const isToggling = !!togglingSkill[skill.id];
            const isCustom = skill.id.startsWith('skill_') || skill.id.startsWith('custom_');

            return (
              <div
                key={skill.id}
                className={`flex flex-col justify-between p-6 rounded-2xl border transition-all duration-200 ${
                  skill.enabled
                    ? 'border-slate-200/80 dark:border-slate-800 bg-white dark:bg-[#191B1F] shadow-xs hover:shadow-md'
                    : 'border-slate-200/50 dark:border-slate-800/50 bg-slate-50/50 dark:bg-slate-900/30 opacity-75'
                }`}
              >
                <div>
                  {/* 头部：图标、名称与启停开关 */}
                  <div className="flex items-start justify-between gap-4 mb-4">
                    <div className="flex items-center gap-3.5 min-w-0">
                      <div className="w-11 h-11 rounded-2xl bg-amber-50/80 dark:bg-amber-950/50 text-amber-600 dark:text-amber-400 border border-amber-200/60 dark:border-amber-800/60 flex items-center justify-center shrink-0 shadow-2xs">
                        {skill.id === 'git-workflow' ? <GitBranch className="w-5 h-5" /> :
                         skill.id === 'test-driven-development' ? <Bug className="w-5 h-5" /> :
                         skill.id === 'browser-automation' ? <Globe className="w-5 h-5" /> :
                         <Lock className="w-5 h-5" />}
                      </div>
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <h3 className="text-sm font-semibold text-slate-900 dark:text-white truncate">
                            {skill.name}
                          </h3>
                          <span className="px-2 py-0.5 text-xs font-mono rounded-md bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-400">
                            v{skill.version || '1.0.0'}
                          </span>
                        </div>
                        <div className="ui-meta font-mono truncate mt-0.5">
                          {skill.id}
                        </div>
                      </div>
                    </div>

                    <button
                      onClick={() => handleToggleSkill(skill.id, skill.enabled)}
                      disabled={isToggling}
                      className={`relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none ${skill.enabled ? 'bg-amber-600' : 'bg-slate-300 dark:bg-slate-700'}`}
                      title={skill.enabled ? (isZh ? '点击禁用' : 'Click to disable') : (isZh ? '点击启用' : 'Click to enable')}
                    >
                      <span
                        className={`pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow-sm ring-0 transition duration-200 ease-in-out ${skill.enabled ? 'translate-x-4' : 'translate-x-0'}`}
                      />
                    </button>
                  </div>

                  {/* 简介 */}
                  <p className="text-xs text-slate-600 dark:text-slate-400 leading-relaxed line-clamp-2 my-4">
                    {skill.description}
                  </p>

                  {/* 适用工具清单 */}
                  {skill.tools && skill.tools.length > 0 && (
                    <div className="flex items-center gap-1.5 flex-wrap mb-5 text-xs">
                      <span className="ui-meta mr-0.5">{isZh ? '工具集:' : 'Tools:'}</span>
                      {skill.tools.slice(0, 5).map(t => (
                        <span
                          key={t}
                          className="px-2 py-0.5 font-mono text-xs bg-slate-50 dark:bg-slate-900 text-slate-600 dark:text-slate-300 rounded-md border border-slate-200/70 dark:border-slate-800"
                        >
                          {t}
                        </span>
                      ))}
                      {skill.tools.length > 5 && (
                        <span className="ui-meta font-mono font-medium">+{skill.tools.length - 5}</span>
                      )}
                    </div>
                  )}
                </div>

                {/* 卡片底部操作按钮 */}
                <div className="pt-4 mt-auto border-t border-slate-100 dark:border-slate-800/80 flex items-center justify-between gap-2">
                  <div className="flex items-center gap-2 flex-wrap">
                    <a
                      href={`/api/v1/skills/${skill.id}/download`}
                      download={`${skill.id}.zip`}
                      className="ui-btn ui-btn-sm border border-emerald-200/80 dark:border-emerald-800/80 bg-emerald-50 dark:bg-emerald-950/40 text-emerald-700 dark:text-emerald-300 hover:bg-emerald-100 dark:hover:bg-emerald-900/50 shadow-2xs"
                      title={isZh ? '直接下载 .zip 压缩包' : 'Direct download .zip archive'}
                    >
                      <DownloadCloud className="w-3.5 h-3.5" />
                      <span>{isZh ? '下载 .zip' : 'Download .zip'}</span>
                    </a>
                    <button
                      onClick={() => { setManifestSkill(skill); setManifestTab('skill_md'); }}
                      className="ui-btn ui-btn-sm border border-slate-200/70 dark:border-slate-700/70 bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300 hover:bg-slate-200 dark:hover:bg-slate-700 shadow-2xs"
                    >
                      <FileText className="w-3.5 h-3.5" />
                      <span>{isZh ? '查看 SOP' : 'View SOP'}</span>
                    </button>
                    <button
                      onClick={() => { setSelectedSkillForGuide(skill); setClientGuideModalOpen(true); }}
                      className="ui-btn ui-btn-sm border border-[#355CFF]/30 dark:border-[#91A7FF]/30 bg-[#EEF2FF] dark:bg-[#242D48] text-[#355CFF] dark:text-[#91A7FF] hover:bg-[#E0E7FF] dark:hover:bg-[#2e3a5e] shadow-2xs"
                    >
                      <Terminal className="w-3.5 h-3.5" />
                      <span>{isZh ? '接入' : 'Connect'}</span>
                    </button>
                    <button
                      title={isZh ? '编辑技能' : 'Edit skill'}
                      onClick={() => { setEditingSkillId(skill.id); setNewSkill({ ...skill, tools: (skill.tools || []).join(', ') }); setCreateModalOpen(true); }}
                      className="ui-btn-secondary"
                    ><Pencil className="w-3.5 h-3.5" />{isZh ? '编辑' : 'Edit'}</button>
                    <button
                      onClick={() => setTestModalSkill(skill)}
                      className="ui-btn-ghost ui-btn-sm"
                    >
                      <Play className="w-3 h-3" />
                      <span>{isZh ? '模型实测' : 'Model test'}</span>
                    </button>
                  </div>

                  {isCustom && (
                    <button
                      onClick={() => handleDeleteSkill(skill.id)}
                      className="ui-icon-btn hover:text-rose-500 hover:bg-rose-50 dark:hover:bg-rose-950/40"
                      title={isZh ? '删除自定义技能' : 'Delete custom skill'}
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </button>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* 客户端接入指南弹窗 (Claude Code / OpenCode / Codex / Cursor) */}
      {clientGuideModalOpen && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm"
          onClick={(e) => {
            if (e.target === e.currentTarget) setClientGuideModalOpen(false);
          }}
        >
          <div className="bg-white dark:bg-[#191B1F] rounded-xl shadow-2xl border border-gray-200 dark:border-slate-800 max-w-2xl w-full p-6 space-y-4 max-h-[90vh] overflow-y-auto">
            <div className="flex items-center justify-between pb-3 border-b border-gray-200 dark:border-gray-800">
              <div className="flex items-center gap-2.5">
                <div className="p-2 bg-linear-to-br from-amber-500 to-orange-600 text-white rounded-lg">
                  <Terminal className="w-5 h-5" />
                </div>
                <div>
                  <h3 className="text-base font-bold text-gray-900 dark:text-white">
                    {isZh ? '智能体客户端接入示例' : 'Agent Client Integration Examples'}
                  </h3>
                  <p className="ui-meta">
                    {selectedSkillForGuide 
                      ? (isZh ? `将 [${selectedSkillForGuide.name}] 导入您的 Agent 编程环境` : `Import [${selectedSkillForGuide.name}] into your Agent environment`)
                      : (isZh ? '将 Agent Skills 规范集成到各大主流智能体客户端' : 'Integrate Agent Skills standards into major AI clients')}
                  </p>
                </div>
              </div>
              <button
                onClick={() => setClientGuideModalOpen(false)}
                className="ui-icon-btn"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            {/* 解耦规范与原理说明横幅 */}
            {(() => {
              const currentId = selectedSkillForGuide?.id || 'git-workflow';
              return (
                <div className="p-3 bg-amber-50 dark:bg-amber-950/40 rounded-lg border border-amber-200/80 dark:border-amber-800/60 text-xs text-amber-900 dark:text-amber-200 leading-relaxed space-y-1.5">
                  <div className="font-semibold flex items-center justify-between text-amber-800 dark:text-amber-100">
                    <div className="flex items-center gap-1.5">
                      <CheckCircle2 className="w-4 h-4 text-emerald-600" />
                      <span>{isZh ? 'SOP 规范正文与客户端接入完全解耦' : 'SOP Specification decoupled from client environments'}</span>
                    </div>
                    <a
                      href={`/api/v1/skills/${currentId}/download`}
                      download={`${currentId}.zip`}
                      className="ui-btn ui-btn-sm font-semibold bg-emerald-600 hover:bg-emerald-700 text-white"
                    >
                      <DownloadCloud className="w-3.5 h-3.5" />
                      {isZh ? `一键下载 ${currentId}.zip` : `Download ${currentId}.zip`}
                    </a>
                  </div>
                  <p>
                    {isZh ? (
                      <>技能遵循 <strong>agentskills.io</strong> 开放标准以标准 ZIP 压缩包分发。<code className="font-mono bg-white/70 dark:bg-gray-800 px-1 py-0.5 rounded-sm">SKILL.md</code> 内部保持纯净的 SOP 规范与规则契约，不掺杂任何特定客户端安装脚本。工具由本机系统环境或 MCP 协议广场提供，<strong>零网络下载等待</strong>。</>
                    ) : (
                      <>Skills follow open <strong>agentskills.io</strong> standard distributed as ZIP archives. <code className="font-mono bg-white/70 dark:bg-gray-800 px-1 py-0.5 rounded-sm">SKILL.md</code> maintains pure SOP contracts without vendor lock-in. Tools are provided locally or via MCP protocol, <strong>with zero wait</strong>.</>
                    )}
                  </p>
                </div>
              );
            })()}

            {/* 客户端选择 Tabs */}
            <div className="flex items-center gap-2 border-b border-gray-200 dark:border-gray-800 pb-2">
              {[
                { id: 'claude-code', label: 'Claude Code' },
                { id: 'opencode', label: 'OpenCode' },
                { id: 'codex', label: 'Codex / OpenAI SDK' },
                { id: 'cursor', label: 'Cursor / Cline' }
              ].map(c => (
                <button
                  key={c.id}
                  onClick={() => setActiveClientTab(c.id)}
                  className={`px-3 py-1.5 text-xs font-medium rounded-lg transition-colors ${activeClientTab === c.id ? 'bg-amber-600 text-white' : 'text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800'}`}
                >
                  {c.label}
                </button>
              ))}
            </div>

            {/* 接入指令与代码片段 */}
            {(() => {
              const currentId = selectedSkillForGuide?.id || 'git-workflow';
              return (
                <div className="space-y-3">
                  {activeClientTab === 'claude-code' && (
                    <div className="space-y-3 text-xs">
                      <div className="p-3 bg-gray-50 dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800 space-y-2">
                        <div className="font-semibold text-gray-800 dark:text-gray-200 flex items-center justify-between">
                          <span>{isZh ? '步骤 1：下载并解压标准技能包至 Claude Code 技能目录' : 'Step 1: Download & unzip standard package to Claude Code skills dir'}</span>
                          <button
                            onClick={() => handleCopyText(`mkdir -p .claude/skills && curl -sL ${origin}/api/v1/skills/${currentId}/download -o ${currentId}.zip && unzip -q -o ${currentId}.zip -d .claude/skills/ && rm -f ${currentId}.zip`, 'cmd-claude-curl')}
                            className="text-xs text-amber-600 dark:text-amber-400 hover:underline inline-flex items-center gap-1"
                          >
                            {copiedKey === 'cmd-claude-curl' ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                            {isZh ? '复制一键命令' : 'Copy Command'}
                          </button>
                        </div>
                        <pre className="p-2.5 bg-gray-900 text-gray-100 rounded-sm font-mono text-xs overflow-x-auto">
{`# ${isZh ? '一键下载解压标准包' : 'Quick download & unpack'} (${currentId}.zip)
mkdir -p .claude/skills
curl -sL ${origin}/api/v1/skills/${currentId}/download -o ${currentId}.zip
unzip -q -o ${currentId}.zip -d .claude/skills/
rm -f ${currentId}.zip`}
                        </pre>
                      </div>

                      <div className="p-3 bg-gray-50 dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800 space-y-2">
                        <div className="font-semibold text-gray-800 dark:text-gray-200">
                          {isZh ? '步骤 2：启动 Claude Code，智能体将自动索引并渐进式激活该技能' : 'Step 2: Launch Claude Code; agent will auto-index and activate skill'}
                        </div>
                        <pre className="p-2.5 bg-gray-900 text-gray-100 rounded-sm font-mono text-xs">
{`claude
# ${isZh ? '直接在对话中提出需求，Claude Code 会自动读取 .claude/skills/' + currentId + '/SKILL.md 标准流程执行！' : 'Prompt directly in chat; Claude Code auto-activates .claude/skills/' + currentId + '/SKILL.md!'}`}
                        </pre>
                      </div>
                    </div>
                  )}

                  {activeClientTab === 'opencode' && (
                    <div className="space-y-3 text-xs">
                      <div className="p-3 bg-gray-50 dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800 space-y-2">
                        <div className="font-semibold text-gray-800 dark:text-gray-200 flex items-center justify-between">
                          <span>{isZh ? '步骤 1：下载并解压标准技能包至 OpenCode 技能目录' : 'Step 1: Download & unzip package to OpenCode skills dir'}</span>
                          <button
                            onClick={() => handleCopyText(`mkdir -p .opencode/skills && curl -sL ${origin}/api/v1/skills/${currentId}/download -o ${currentId}.zip && unzip -q -o ${currentId}.zip -d .opencode/skills/ && rm -f ${currentId}.zip`, 'cmd-opencode-curl')}
                            className="text-xs text-amber-600 dark:text-amber-400 hover:underline inline-flex items-center gap-1"
                          >
                            {copiedKey === 'cmd-opencode-curl' ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                            {isZh ? '复制一键命令' : 'Copy Command'}
                          </button>
                        </div>
                        <pre className="p-2.5 bg-gray-900 text-gray-100 rounded-sm font-mono text-xs overflow-x-auto">
{`# ${isZh ? '一键下载解压标准包' : 'Quick download & unpack'} (${currentId}.zip)
mkdir -p .opencode/skills
curl -sL ${origin}/api/v1/skills/${currentId}/download -o ${currentId}.zip
unzip -q -o ${currentId}.zip -d .opencode/skills/
rm -f ${currentId}.zip`}
                        </pre>
                      </div>

                      <div className="p-3 bg-gray-50 dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800 space-y-2">
                        <div className="font-semibold text-gray-800 dark:text-gray-200">
                          {isZh ? '步骤 2：启动 OpenCode' : 'Step 2: Launch OpenCode'}
                        </div>
                        <pre className="p-2.5 bg-gray-900 text-gray-100 rounded-sm font-mono text-xs">
{`opencode
# ${isZh ? 'OpenCode 原生兼容 agentskills.io 开放标准，遇到触发词自动载入 SOP 并调用工具' : 'OpenCode natively adheres to agentskills.io, auto-loading SOP on keyword triggers'}`}
                        </pre>
                      </div>
                    </div>
                  )}

                  {activeClientTab === 'codex' && (
                    <div className="space-y-3 text-xs">
                      <div className="font-semibold text-gray-800 dark:text-gray-200 flex items-center justify-between">
                        <span>{isZh ? 'Python SDK (OpenAI / Codex API) 载入与网关调用示例：' : 'Python SDK (OpenAI / Codex API) Gateway Integration Example:'}</span>
                        <button
                          onClick={() => handleCopyText(`import os\nimport urllib.request\nimport zipfile\nimport pathlib\nfrom openai import OpenAI\n\n# 1. Pull & unzip skill archive\nskill_id = "${currentId}"\nzip_path = f"{skill_id}.zip"\nurllib.request.urlretrieve(f"${origin}/api/v1/skills/{skill_id}/download", zip_path)\nwith zipfile.ZipFile(zip_path, "r") as z:\n    z.extractall(".skills/")\nos.remove(zip_path)\n\n# 2. Read pure SKILL.md as System SOP\nskill_sop = pathlib.Path(f".skills/{skill_id}/SKILL.md").read_text(encoding="utf-8")\n\n# 3. Connect via airoute unified gateway\nclient = OpenAI(base_url="${origin}/v1", api_key="sk-airoute-key")\nresponse = client.chat.completions.create(\n    model="claude-3-7-sonnet",\n    messages=[\n        {"role": "system", "content": f"Follow this Skill SOP:\\n{skill_sop}"},\n        {"role": "user", "content": "Execute task following workflow specification"}\n    ]\n)\nprint(response.choices[0].message.content)`, 'cmd-codex')}
                          className="text-xs text-amber-600 dark:text-amber-400 hover:underline inline-flex items-center gap-1"
                        >
                          {copiedKey === 'cmd-codex' ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                          {isZh ? '复制代码' : 'Copy Code'}
                        </button>
                      </div>
                      <pre className="p-3 bg-gray-900 text-gray-100 rounded-lg font-mono text-xs overflow-x-auto">
{`import os
import urllib.request
import zipfile
import pathlib
from openai import OpenAI

# 1. Pull & unzip skill archive
skill_id = "${currentId}"
zip_path = f"{skill_id}.zip"
urllib.request.urlretrieve(f"${origin}/api/v1/skills/{skill_id}/download", zip_path)
with zipfile.ZipFile(zip_path, "r") as z:
    z.extractall(".skills/")
os.remove(zip_path)

# 2. Read pure SKILL.md as System SOP
skill_sop = pathlib.Path(f".skills/{skill_id}/SKILL.md").read_text(encoding="utf-8")

# 3. Connect via airoute unified gateway
client = OpenAI(base_url="${origin}/v1", api_key="sk-airoute-key")
response = client.chat.completions.create(
    model="claude-3-7-sonnet",
    messages=[
        {"role": "system", "content": f"Follow this Skill SOP:\\n{skill_sop}"},
        {"role": "user", "content": "Execute task following workflow specification"}
    ]
)
print(response.choices[0].message.content)`}
                      </pre>
                    </div>
                  )}

                  {activeClientTab === 'cursor' && (
                    <div className="space-y-3 text-xs">
                      <div className="p-3 bg-gray-50 dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800 space-y-2">
                        <div className="font-semibold text-gray-800 dark:text-gray-200 flex items-center justify-between">
                          <span>{isZh ? 'Cursor / Cline 导入方案' : 'Cursor / Cline Import Options'}</span>
                          <button
                            onClick={() => handleCopyText(`mkdir -p .cursor/skills && curl -sL ${origin}/api/v1/skills/${currentId}/download -o ${currentId}.zip && unzip -q -o ${currentId}.zip -d .cursor/skills/ && rm -f ${currentId}.zip`, 'cmd-cursor')}
                            className="text-xs text-amber-600 dark:text-amber-400 hover:underline inline-flex items-center gap-1"
                          >
                            {copiedKey === 'cmd-cursor' ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                            {isZh ? '复制一键命令' : 'Copy Command'}
                          </button>
                        </div>
                        <pre className="p-2.5 bg-gray-900 text-gray-100 rounded-sm font-mono text-xs overflow-x-auto">
{`# ${isZh ? '方案 A: 解压至 Cursor 技能目录' : 'Option A: Unpack to Cursor skills directory'}
mkdir -p .cursor/skills
curl -sL ${origin}/api/v1/skills/${currentId}/download -o ${currentId}.zip
unzip -q -o ${currentId}.zip -d .cursor/skills/
rm -f ${currentId}.zip

# ${isZh ? '方案 B: 直接追加至项目根目录的 .cursorrules' : 'Option B: Append directly to root .cursorrules'}
cat .cursor/skills/${currentId}/SKILL.md >> .cursorrules`}
                        </pre>
                      </div>
                    </div>
                  )}
                </div>
              );
            })()}

            <div className="flex justify-end pt-2 border-t border-gray-100 dark:border-gray-800">
              <button
                onClick={() => setClientGuideModalOpen(false)}
                className="px-4 py-2 text-xs font-medium text-gray-700 dark:text-gray-300 bg-gray-100 dark:bg-gray-800 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-700"
              >
                {isZh ? '关闭' : 'Close'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 技能详情 / SKILL.md / ZIP 归档包与元数据弹窗 */}
      {manifestSkill && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm"
          onClick={(e) => {
            if (e.target === e.currentTarget) setManifestSkill(null);
          }}
        >
          <div className="bg-white dark:bg-[#191B1F] rounded-xl shadow-2xl border border-gray-200 dark:border-slate-800 max-w-3xl w-full p-6 space-y-4 max-h-[90vh] overflow-y-auto">
            {/* 模态框头部 */}
            <div className="flex items-center justify-between pb-3 border-b border-gray-200 dark:border-gray-800">
              <div className="flex items-center gap-2.5">
                <div className="p-2 bg-amber-100 dark:bg-amber-900/50 text-amber-600 dark:text-amber-400 rounded-lg">
                  <FileText className="w-5 h-5" />
                </div>
                <div>
                  <h3 className="text-base font-bold text-gray-900 dark:text-white flex items-center gap-2">
                    {manifestSkill.name}
                    <span className="text-xs font-mono font-normal text-amber-600 bg-amber-50 dark:bg-amber-950/60 px-1.5 py-0.5 rounded-sm border border-amber-200 dark:border-amber-800">
                      {manifestSkill.id}
                    </span>
                  </h3>
                  <p className="ui-meta">
                    {isZh ? '遵循 agentskills.io 开放标准的智能体 SOP 标准包与元数据契约' : 'Standard SOP package and metadata contract adhering to agentskills.io'}
                  </p>
                </div>
              </div>
              <div className="flex items-center gap-2">
                <a
                  href={`/api/v1/skills/${manifestSkill.id}/download`}
                  download={`${manifestSkill.id}.zip`}
                  className="inline-flex items-center gap-1 px-2.5 py-1.5 text-xs font-semibold bg-emerald-600 hover:bg-emerald-700 text-white rounded-lg transition-colors shadow-sm"
                  title={isZh ? '下载包含 SKILL.md 与脚本的 agentskills.io 标准 ZIP 压缩包' : 'Download standard ZIP package containing SKILL.md and scripts'}
                >
                  <DownloadCloud className="w-3.5 h-3.5" />
                  <span>{isZh ? '下载 .zip 包' : 'Download .zip'}</span>
                </a>
                <button
                  onClick={() => { setSelectedSkillForGuide(manifestSkill); setClientGuideModalOpen(true); }}
                  className="inline-flex items-center gap-1 px-2.5 py-1.5 text-xs font-medium text-[#355CFF] dark:text-[#91A7FF] bg-[#EEF2FF] dark:bg-[#242D48] border border-[#355CFF]/30 dark:border-[#91A7FF]/30 rounded-lg hover:bg-[#E0E7FF] dark:hover:bg-[#2e3a5e] transition-colors"
                >
                  <Terminal className="w-3.5 h-3.5" />
                  <span>{isZh ? '接入客户端' : 'Connect Client'}</span>
                </button>
                <button
                  onClick={() => setManifestSkill(null)}
                  className="ui-icon-btn"
                >
                  <X className="w-5 h-5" />
                </button>
              </div>
            </div>

            {/* Tab 导航 */}
            <div className="flex items-center justify-between border-b border-gray-200 dark:border-gray-800 pb-2">
              <div className="flex items-center gap-2">
                <button
                  onClick={() => setManifestTab('skill_md')}
                  className={`flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg transition-colors ${
                    manifestTab === 'skill_md'
                      ? 'bg-amber-600 text-white shadow-sm'
                      : 'text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800'
                  }`}
                >
                  <FileText className="w-3.5 h-3.5" />
                  <span>{isZh ? 'SKILL.md (纯净 SOP)' : 'SKILL.md (Pure SOP)'}</span>
                </button>
                <button
                  onClick={() => setManifestTab('bundle_tree')}
                  className={`flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg transition-colors ${
                    manifestTab === 'bundle_tree'
                      ? 'bg-amber-600 text-white shadow-sm'
                      : 'text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800'
                  }`}
                >
                  <FolderTree className="w-3.5 h-3.5" />
                  <span>{isZh ? '归档包目录树 (ZIP Tree)' : 'Archive Tree (ZIP Tree)'}</span>
                </button>
                <button
                  onClick={() => setManifestTab('metadata')}
                  className={`flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg transition-colors ${
                    manifestTab === 'metadata'
                      ? 'bg-amber-600 text-white shadow-sm'
                      : 'text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800'
                  }`}
                >
                  <Code2 className="w-3.5 h-3.5" />
                  <span>metadata.json</span>
                </button>
              </div>

              {manifestTab === 'skill_md' && (
                <button
                  onClick={() => handleCopyText(manifestSkill.manifest, 'manifest-full')}
                  className="inline-flex items-center gap-1 px-2.5 py-1 text-xs font-medium bg-gray-100 dark:bg-gray-800 text-gray-700 dark:text-gray-300 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-700 transition-colors"
                >
                  {copiedKey === 'manifest-full' ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                  <span>{copiedKey === 'manifest-full' ? (isZh ? '已复制' : 'Copied') : (isZh ? '复制 SOP' : 'Copy SOP')}</span>
                </button>
              )}
            </div>

            {/* Tab 1: SKILL.md */}
            {manifestTab === 'skill_md' && (
              <div className="space-y-2">
                <div className="flex items-center justify-between ui-meta px-1">
                  <span>{isZh ? '纯业务 SOP 指令契约 (不包含客户端下载逻辑，三级渐进披露规范)' : 'Pure business SOP contract (3-tier progressive disclosure)'}</span>
                  <span className="font-mono">{manifestSkill.manifest.length} {isZh ? '字符' : 'chars'}</span>
                </div>
                <div className="bg-gray-900 text-gray-100 p-4 rounded-xl font-mono text-xs overflow-x-auto max-h-[55vh] border border-gray-800">
                  <pre>{manifestSkill.manifest}</pre>
                </div>
              </div>
            )}

            {/* Tab 2: 归档包目录树 (ZIP Tree) */}
            {manifestTab === 'bundle_tree' && (
              <div className="space-y-4">
                {/* 存储后端与加速说明 */}
                <div className="p-3 bg-linear-to-r from-gray-50 to-slate-100 dark:from-gray-900 dark:to-[#191B1F] rounded-xl border border-gray-200 dark:border-gray-800 flex items-start justify-between gap-3 text-xs">
                  <div className="space-y-1">
                    <div className="font-semibold text-gray-800 dark:text-gray-200 flex items-center gap-1.5">
                      {storageStatus?.driver === 's3' ? (
                        <>
                          <Server className="w-4 h-4 text-emerald-600" />
                          <span>{isZh ? `分布式对象存储已激活 (RustFS / S3: bucket ` : `Distributed Object Storage Active (S3: bucket `}<code>{storageStatus.s3_bucket || 'skills'}</code>)</span>
                        </>
                      ) : (
                        <>
                          <HardDrive className="w-4 h-4 text-slate-500" />
                          <span>{isZh ? `本地文件驱动已激活 (Local: ` : `Local File Driver Active (Local: `}<code>{storageStatus?.local_path || 'data/skills_storage'}</code>)</span>
                        </>
                      )}
                    </div>
                    <p className="ui-meta">
                      {storageStatus?.driver === 's3'
                        ? (isZh ? '下载请求由网关直接签发 AWS SigV4 预签名临时 URL，302 重定向直连 RustFS 高速传输，完全释放网关带宽。' : 'Download requests are signed with AWS SigV4 presigned URLs and redirected 302 to RustFS high-speed transfer, offloading gateway bandwidth.')
                        : (isZh ? '归档包由网关本地磁盘存储提供流式下载与持久化。配置 STORAGE_DRIVER=s3 可无缝切换至 RustFS 对象存储。' : 'Archive packages are streamed directly from gateway local disk. Set STORAGE_DRIVER=s3 to switch to RustFS.')}
                    </p>
                  </div>
                  <span className="px-2 py-0.5 rounded-sm text-xs font-mono font-medium bg-emerald-100 dark:bg-emerald-950/60 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800 shrink-0">
                    S3 Key: skills/{manifestSkill.id}.zip
                  </span>
                </div>

                {/* 目录树预览 */}
                <div className="bg-gray-900 text-gray-100 p-4 rounded-xl font-mono text-xs border border-gray-800 space-y-1.5">
                  <div className="text-amber-400 font-bold flex items-center gap-1.5 pb-2 border-b border-gray-800">
                    <Folder className="w-4 h-4" />
                    <span>{manifestSkill.id}.zip ({isZh ? 'agentskills.io 标准封装包' : 'agentskills.io standard bundle'})</span>
                  </div>
                  <div className="pl-4 space-y-1 text-gray-300 text-xs">
                    <div className="text-blue-400 flex items-center gap-1.5">
                      <span>└── 📁 {manifestSkill.id}/</span>
                    </div>
                    <div className="pl-6 space-y-1 text-gray-300">
                      <div className="flex items-center justify-between text-emerald-300">
                        <span>├── 📄 SKILL.md</span>
                        <span className="text-gray-400 text-xs">{isZh ? '# 核心 SOP 流程契约与三级执行阶段' : '# Core SOP contract & 3-stage execution'}</span>
                      </div>
                      <div className="flex items-center justify-between text-yellow-300">
                        <span>├── 📄 metadata.json</span>
                        <span className="text-gray-400 text-xs">{isZh ? '# 技能描述、版本与作者元数据' : '# Skill description, version, and author metadata'}</span>
                      </div>
                      <div className="text-blue-300">
                        <span>├── 📁 scripts/</span>
                      </div>
                      <div className="pl-6 flex items-center justify-between text-purple-300">
                        <span>│   └── ⚙️ run.sh</span>
                        <span className="text-gray-400 text-xs">{isZh ? '# 可选本机自动化演练或 Hook 脚本' : '# Optional local rehearsal or hook script'}</span>
                      </div>
                      <div className="text-blue-300">
                        <span>└── 📁 references/</span>
                      </div>
                      <div className="pl-6 flex items-center justify-between text-sky-300">
                        <span>    └── 📑 context.md</span>
                        <span className="text-gray-400 text-xs">{isZh ? '# 补充上下文与规则详情' : '# Supplemental context & rule details'}</span>
                      </div>
                    </div>
                  </div>
                </div>

                {/* 快速拉取命令 */}
                <div className="p-3 bg-gray-50 dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 space-y-1.5 text-xs">
                  <div className="flex items-center justify-between text-gray-700 dark:text-gray-300 font-medium">
                    <span>{isZh ? '终端下载并解压命令 (Terminal Quickstart)' : 'Terminal Quickstart Download & Unzip'}</span>
                    <button
                      onClick={() => handleCopyText(`curl -sL ${origin}/api/v1/skills/${manifestSkill.id}/download -o ${manifestSkill.id}.zip && unzip -q -o ${manifestSkill.id}.zip -d skills/`, 'tree-curl')}
                      className="text-xs text-amber-600 dark:text-amber-400 hover:underline inline-flex items-center gap-1"
                    >
                      {copiedKey === 'tree-curl' ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                      {isZh ? '复制命令' : 'Copy Command'}
                    </button>
                  </div>
                  <pre className="p-2 bg-gray-900 text-gray-200 rounded-sm font-mono text-xs overflow-x-auto">
{`curl -sL ${origin}/api/v1/skills/${manifestSkill.id}/download -o ${manifestSkill.id}.zip
unzip -q -o ${manifestSkill.id}.zip -d skills/`}
                  </pre>
                </div>
              </div>
            )}

            {/* Tab 3: metadata.json */}
            {manifestTab === 'metadata' && (
              <div className="space-y-2">
                <div className="flex items-center justify-between ui-meta px-1">
                  <span>{isZh ? 'agentskills.io 元数据声明契约' : 'agentskills.io Metadata Contract'}</span>
                  <button
                    onClick={() => handleCopyText(JSON.stringify({
                      schema_version: "agentskills.io/v1",
                      id: manifestSkill.id,
                      name: manifestSkill.name,
                      version: manifestSkill.version || "1.0.0",
                      author: manifestSkill.author || "Community Standard",
                      category: manifestSkill.category,
                      loading_mode: manifestSkill.loading_mode || "lazy",
                      tools: manifestSkill.tools || [],
                      enabled: manifestSkill.enabled,
                      storage: {
                        artifact_key: `skills/${manifestSkill.id}.zip`,
                        driver: storageStatus?.driver || 'local'
                      }
                    }, null, 2), 'meta-json')}
                    className="inline-flex items-center gap-1 text-amber-600 dark:text-amber-400 hover:underline"
                  >
                    {copiedKey === 'meta-json' ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                    <span>{copiedKey === 'meta-json' ? (isZh ? '已复制' : 'Copied') : (isZh ? '复制 JSON' : 'Copy JSON')}</span>
                  </button>
                </div>
                <div className="bg-gray-900 text-gray-100 p-4 rounded-xl font-mono text-xs overflow-x-auto max-h-[55vh] border border-gray-800">
                  <pre>
{JSON.stringify({
  schema_version: "agentskills.io/v1",
  id: manifestSkill.id,
  name: manifestSkill.name,
  description: manifestSkill.description,
  version: manifestSkill.version || "1.0.0",
  author: manifestSkill.author || "Community Standard",
  category: manifestSkill.category,
  loading_mode: manifestSkill.loading_mode || "lazy",
  tools: manifestSkill.tools || [],
  enabled: manifestSkill.enabled,
  storage: {
    artifact_key: `skills/${manifestSkill.id}.zip`,
    driver: storageStatus?.driver || 'local'
  }
}, null, 2)}
                  </pre>
                </div>
              </div>
            )}

            {/* 模态框底部 */}
            <div className="flex justify-end pt-2 border-t border-gray-100 dark:border-gray-800">
              <button
                onClick={() => setManifestSkill(null)}
                className="px-4 py-2 text-xs font-medium text-gray-700 dark:text-gray-300 bg-gray-100 dark:bg-gray-800 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-700"
              >
                {isZh ? '关闭' : 'Close'}
              </button>
            </div>
          </div>
        </div>
      )}

      {testModalSkill && <SkillTrialModal skill={testModalSkill} lang={lang} onClose={() => setTestModalSkill(null)} onTrial={onTrial} />}

      {/* 新建自定义技能模态框 */}
      {createModalOpen && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm"
          onClick={(e) => {
            if (e.target === e.currentTarget) setCreateModalOpen(false);
          }}
        >
          <div className="bg-white dark:bg-[#191B1F] rounded-xl shadow-2xl border border-gray-200 dark:border-slate-800 max-w-xl w-full p-6 space-y-4 max-h-[90vh] overflow-y-auto">
            <div className="flex items-center justify-between pb-3 border-b border-gray-200 dark:border-gray-800">
              <div className="flex items-center gap-2.5">
                <div className="p-2 bg-linear-to-br from-amber-500 to-orange-600 text-white rounded-lg">
                  <Plus className="w-5 h-5" />
                </div>
                <div>
                  <h3 className="text-base font-bold text-gray-900 dark:text-white">
                    {editingSkillId ? (isZh ? '编辑 Agent 技能' : 'Edit Agent Skill') : (isZh ? '新建自定义 Agent 技能' : 'Create Custom Agent Skill')}
                  </h3>
                  <p className="ui-meta">
                    {isZh ? '遵循 agentskills.io 规范编写自定义工作流 SOP' : 'Author custom workflow SOP adhering to agentskills.io standards'}
                  </p>
                </div>
              </div>
              <button
                onClick={() => setCreateModalOpen(false)}
                className="ui-icon-btn"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <form onSubmit={handleCreateSkill} className="space-y-4">
              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                <div>
                  <label className="ui-label">
                    {isZh ? '技能 ID (如: api-designer) *' : 'Skill ID (e.g. api-designer) *'}
                  </label>
                  <input
                    type="text"
                    required
                    placeholder={isZh ? "如: api-designer" : "e.g. api-designer"}
                    readOnly={!!editingSkillId}
                    value={newSkill.id}
                    onChange={(e) => setNewSkill({ ...newSkill, id: e.target.value })}
                    className="ui-field bg-gray-50 dark:bg-gray-900"
                  />
                </div>
                <div>
                  <label className="ui-label">
                    {isZh ? '技能名称 *' : 'Skill Name *'}
                  </label>
                  <input
                    type="text"
                    required
                    placeholder={isZh ? '如: OpenAPI 3.0 接口规范设计' : 'e.g. OpenAPI 3.0 API Design'}
                    value={newSkill.name}
                    onChange={(e) => setNewSkill({ ...newSkill, name: e.target.value })}
                    className="ui-field bg-gray-50 dark:bg-gray-900"
                  />
                </div>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                <div>
                  <label className="ui-label">
                    {isZh ? '分类领域' : 'Category'}
                  </label>
                  <select
                    value={newSkill.category}
                    onChange={(e) => setNewSkill({ ...newSkill, category: e.target.value })}
                    className="ui-field bg-gray-50 dark:bg-gray-900"
                  >
                    <option value="dev">{isZh ? '研发工程 (dev)' : 'Dev Engineering (dev)'}</option>
                    <option value="test">{isZh ? '质量保障 (test)' : 'QA & TDD (test)'}</option>
                    <option value="automation">{isZh ? '端到端自动化 (automation)' : 'E2E Automation (automation)'}</option>
                    <option value="security">{isZh ? '代码安全审计 (security)' : 'Security Audit (security)'}</option>
                  </select>
                </div>
                <div>
                  <label className="ui-label">
                    {isZh ? '依赖工具 (逗号分隔)' : 'Required Tools (comma separated)'}
                  </label>
                  <input
                    type="text"
                    placeholder={isZh ? "如: run_command, view_file" : "e.g. run_command, view_file"}
                    value={newSkill.tools}
                    onChange={(e) => setNewSkill({ ...newSkill, tools: e.target.value })}
                    className="ui-field bg-gray-50 dark:bg-gray-900 font-mono"
                  />
                </div>
              </div>

              <div>
                <label className="ui-label">
                  {isZh ? '简要描述与触发时机 *' : 'Description & Trigger Conditions *'}
                </label>
                <textarea
                  rows="2"
                  required
                  placeholder={isZh ? '说明该技能的功能以及智能体在何时应当激活...' : 'Describe skill capabilities and trigger situations...'}
                  value={newSkill.description}
                  onChange={(e) => setNewSkill({ ...newSkill, description: e.target.value })}
                  className="ui-field bg-gray-50 dark:bg-gray-900"
                />
              </div>

              <div>
                <label className="ui-label">
                  {isZh ? 'SKILL.md 规范内容 (Markdown)' : 'SKILL.md Specification (Markdown)'}
                </label>
                <textarea
                  rows="5"
                  placeholder={`---
name: ${newSkill.id || 'my-skill'}
description: ${isZh ? '描述内容' : 'Skill description'}
allowed-tools:
  - run_command
---

# ${isZh ? '技能执行指引' : 'Skill Execution Instructions'}
1. ${isZh ? '第一步...' : 'Step 1...'}
2. ${isZh ? '第二步...' : 'Step 2...'}`}
                  value={newSkill.manifest}
                  onChange={(e) => setNewSkill({ ...newSkill, manifest: e.target.value })}
                  className="w-full px-3 py-2 font-mono text-xs rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-900 text-gray-100"
                />
              </div>

              <div className="flex items-center justify-end gap-2.5 pt-3 border-t border-gray-200 dark:border-gray-800">
                <button
                  type="button"
                  onClick={() => setCreateModalOpen(false)}
                  className="px-4 py-2 text-xs font-medium text-gray-700 dark:text-gray-300 bg-gray-100 dark:bg-gray-800 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-700"
                >
                  {isZh ? '取消' : 'Cancel'}
                </button>
                <button
                  type="submit"
                  disabled={savingSkill}
                  className="px-4 py-2 text-xs font-medium text-white bg-amber-600 rounded-lg hover:bg-amber-700 shadow-sm"
                >
                  {savingSkill ? (isZh ? '保存中...' : 'Saving...') : (isZh ? '保存技能' : 'Save Skill')}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
