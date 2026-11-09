import useSkillActions from '../hooks/useSkillActions';
import useSkillState from '../hooks/useSkillState';
import SkillCards from './SkillCards';
import SkillClientGuide from './SkillClientGuide';
import SkillManifestModal from './SkillManifestModal';
import SkillCreateModal from './SkillCreateModal';

import { Sparkles, RefreshCw, BookOpen, Plus, Search, ShieldCheck, Workflow, GitBranch, Bug, Globe, Lock, ArrowRight, HardDrive } from 'lucide-react';

import SkillTrialModal from './SkillTrialModal';

export default function SkillHubView({ adminFetch, onCopy, onTrial, showToast, lang = 'zh', }) {
  const isZh = lang === 'zh';
  const {
    activeClientTab, clientGuideModalOpen, copiedKey, createModalOpen, editingSkillId, loading, manifestSkill,
    manifestTab, newSkill, origin, savingSkill, searchQuery, selectedCategory, selectedSkillForGuide,
    setActiveClientTab, setClientGuideModalOpen, setCopiedKey, setCreateModalOpen, setEditingSkillId,
    setLoading, setManifestSkill, setManifestTab, setNewSkill, setSavingSkill, setSearchQuery,
    setSelectedCategory, setSelectedSkillForGuide, setSkills, setStorageStatus, setTestModalSkill,
    setTogglingSkill, skills, storageStatus, testModalSkill, togglingSkill
  } = useSkillState({

  });

  const {
    handleCopyText, handleCreateSkill, handleDeleteSkill, handleToggleSkill
  } = useSkillActions({
    adminFetch, editingSkillId, isZh, newSkill, onCopy, savingSkill, setCopiedKey, setCreateModalOpen,
    setLoading, setNewSkill, setSavingSkill, setSkills, setStorageStatus, setTogglingSkill, showToast
  });

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
        <SkillCards {...{
          filteredSkills, handleDeleteSkill, handleToggleSkill, isZh, setClientGuideModalOpen, setCreateModalOpen,
          setEditingSkillId, setManifestSkill, setManifestTab, setNewSkill, setSelectedSkillForGuide,
          setTestModalSkill, togglingSkill
        }} />
      )}

      {/* 客户端接入指南弹窗 (Claude Code / OpenCode / Codex / Cursor) */}
      {clientGuideModalOpen && (
        <SkillClientGuide {...{
          activeClientTab, copiedKey, handleCopyText, isZh, origin, selectedSkillForGuide, setActiveClientTab,
          setClientGuideModalOpen
        }} />
      )}

      {/* 技能详情 / SKILL.md / ZIP 归档包与元数据弹窗 */}
      {manifestSkill && (
        <SkillManifestModal {...{
          copiedKey, handleCopyText, isZh, manifestSkill, manifestTab, origin, setClientGuideModalOpen,
          setManifestSkill, setManifestTab, setSelectedSkillForGuide, storageStatus
        }} />
      )}

      {testModalSkill && <SkillTrialModal skill={testModalSkill} lang={lang} onClose={() => setTestModalSkill(null)} onTrial={onTrial} />}

      {/* 新建自定义技能模态框 */}
      {createModalOpen && (
        <SkillCreateModal {...{
          editingSkillId, handleCreateSkill, isZh, newSkill, savingSkill, setCreateModalOpen, setNewSkill
        }} />
      )}
    </div>
  );
}
