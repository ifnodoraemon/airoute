import React, { useState, useEffect } from 'react';
import {
  Sparkles,
  BookOpen,
  FileText,
  Check,
  Copy,
  Plus,
  Search,
  Trash2,
  Play,
  X,
  ExternalLink,
  ShieldCheck,
  Cpu,
  Layers,
  Terminal,
  Activity,
  Code2,
  Workflow,
  HelpCircle,
  AlertCircle,
  FileCode,
  DownloadCloud,
  CheckCircle2,
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

export default function SkillHubView({ adminFetch, onCopy, showToast }) {
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
  const [testPrompt, setTestPrompt] = useState('');
  const [testOutput, setTestOutput] = useState(null);
  const [testingSkill, setTestingSkill] = useState(false);
  const [createModalOpen, setCreateModalOpen] = useState(false);

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

  const fetchSkills = async () => {
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
  };

  const fetchStorageStatus = async () => {
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
  };

  useEffect(() => {
    fetchSkills();
    fetchStorageStatus();
  }, []);

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
        if (showToast) showToast(`技能 [${skillId}] 已${!currentEnabled ? '启用' : '禁用'}`, 'success');
      } else {
        if (showToast) showToast(data.error || '切换技能状态失败', 'error');
      }
    } catch (err) {
      if (showToast) showToast('请求异常: ' + err.message, 'error');
    } finally {
      setTogglingSkill(prev => ({ ...prev, [skillId]: false }));
    }
  };

  const handleCreateSkill = async (e) => {
    e.preventDefault();
    if (!adminFetch) return;
    if (!newSkill.name.trim() || !newSkill.description.trim()) {
      if (showToast) showToast('技能名称与描述不能为空', 'error');
      return;
    }

    const payload = {
      ...newSkill,
      id: newSkill.id.trim() || `skill_${Date.now()}`,
      tools: newSkill.tools ? newSkill.tools.split(/[,，\n]/).map(s => s.trim()).filter(Boolean) : ['run_command', 'view_file'],
      enabled: true
    };

    try {
      const res = await adminFetch('/api/v1/admin/skills', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        if (showToast) showToast('自定义技能创建成功', 'success');
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
        if (showToast) showToast(data.error || '创建失败', 'error');
      }
    } catch (err) {
      if (showToast) showToast('请求异常: ' + err.message, 'error');
    }
  };

  const handleDeleteSkill = async (skillId) => {
    if (!adminFetch) return;
    if (!window.confirm(`确定要删除技能 [${skillId}] 吗？`)) return;
    try {
      const res = await adminFetch(`/api/v1/admin/skills/${skillId}`, {
        method: 'DELETE'
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        setSkills(prev => prev.filter(s => s.id !== skillId));
        if (showToast) showToast(`技能 [${skillId}] 已删除`, 'success');
      } else {
        if (showToast) showToast(data.error || '删除失败', 'error');
      }
    } catch (err) {
      if (showToast) showToast('请求异常: ' + err.message, 'error');
    }
  };

  const handleCopyText = (text, key) => {
    if (onCopy) {
      onCopy(text);
    } else {
      navigator.clipboard.writeText(text);
    }
    setCopiedKey(key);
    setTimeout(() => setCopiedKey(''), 2000);
    if (showToast) showToast('已复制到剪贴板', 'success');
  };

  const handleOpenTest = (skill) => {
    setTestModalSkill(skill);
    setTestOutput(null);
    if (skill.id === 'git-workflow') {
      setTestPrompt('请分析当前暂存区的改动，并按照 Conventional Commits 格式撰写一条严谨的 feat 提交说明。');
    } else if (skill.id === 'test-driven-development') {
      setTestPrompt('为字符串反转函数设计失败测试用例，覆盖空串、中文字符与 Emoji 边界。');
    } else if (skill.id === 'browser-automation') {
      setTestPrompt('打开 http://localhost:8080 并截图确认页面核心标题正常渲染。');
    } else if (skill.id === 'security-audit') {
      setTestPrompt('扫描项目中所有 SQL 查询代码，检查是否存在未做参数化绑定的字符串拼接注入漏洞。');
    } else {
      setTestPrompt('按照该技能的规范执行一次标准作业步骤。');
    }
    setTestModalSkill(skill);
  };

  const handleExecuteTest = async () => {
    if (!testModalSkill) return;
    setTestingSkill(true);
    setTestOutput(null);

    // Simulate standard SOP execution trace
    setTimeout(() => {
      setTestingSkill(false);
      setTestOutput({
        status: 'SUCCESS',
        skill_id: testModalSkill.id,
        sop_stage: 'L3_EXECUTION_COMPLETED',
        tools_called: testModalSkill.tools,
        trace: [
          `[L1] 匹配技能触发词，加载元数据 (${testModalSkill.id})`,
          `[L2] 解析并激活 SKILL.md 执行指引与前置约束`,
          `[L3] 智能体使用宿主/MCP 工具集 [${(testModalSkill.tools || []).join(', ')}] 完成验证`
        ],
        verdict: `✓ 已按照 ${testModalSkill.name} 标准作业程序（SOP）成功模拟执行，输入：“${testPrompt}” 符合规范契约。`
      });
      if (showToast) showToast('技能演练模拟执行成功', 'success');
    }, 400);
  };

  const categories = [
    { id: 'all', label: '全部技能', icon: Workflow },
    { id: 'dev', label: '研发工程', icon: GitBranch },
    { id: 'test', label: '质量保障 (TDD)', icon: Bug },
    { id: 'automation', label: '端到端自动化', icon: Globe },
    { id: 'security', label: '代码安全审计', icon: Lock }
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
    <div className="space-y-5">
      {/* 顶部标题与轻量操作栏 */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-3 pb-3 border-b border-gray-100 dark:border-gray-800">
        <div>
          <div className="flex items-center gap-2.5">
            <h1 className="text-lg font-bold text-gray-900 dark:text-white">
              Agent 技能中心 (Skills)
            </h1>
            <span className="px-2 py-0.5 text-[11px] font-medium rounded-full bg-amber-50 dark:bg-amber-950/60 text-amber-700 dark:text-amber-300 border border-amber-200 dark:border-amber-800">
              agentskills.io 标准
            </span>
            {storageStatus && (
              <span className="px-2 py-0.5 text-[11px] font-medium rounded-full bg-gray-100 dark:bg-gray-800 text-gray-600 dark:text-gray-300">
                {storageStatus.driver === 's3' ? 'S3 存储就绪' : '本地存储'}
              </span>
            )}
          </div>
          <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">
            标准化智能体 SOP 作业包，支持在线审阅、直连高速下载与一键接入 Claude Code 等客户端。
          </p>
        </div>

        <div className="flex items-center gap-2">
          <button
            onClick={() => { setSelectedSkillForGuide(null); setClientGuideModalOpen(true); }}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-gray-700 dark:text-gray-200 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-750 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-700 transition-colors shadow-2xs"
          >
            <BookOpen className="w-3.5 h-3.5 text-amber-600 dark:text-amber-400" />
            客户端接入指南
          </button>
          <button
            onClick={() => setCreateModalOpen(true)}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-white bg-amber-600 hover:bg-amber-700 rounded-lg transition-colors shadow-2xs"
          >
            <Plus className="w-3.5 h-3.5" />
            新建技能
          </button>
        </div>
      </div>

      {/* 搜索与分类导航 */}
      <div className="flex flex-col md:flex-row items-stretch md:items-center justify-between gap-3 bg-white dark:bg-gray-800/80 p-2.5 rounded-xl border border-gray-200 dark:border-gray-800 shadow-2xs">
        <div className="flex items-center gap-1 overflow-x-auto pb-1 md:pb-0 scrollbar-none">
          {categories.map(cat => {
            const Icon = cat.icon;
            const active = selectedCategory === cat.id;
            return (
              <button
                key={cat.id}
                onClick={() => setSelectedCategory(cat.id)}
                className={`flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium rounded-lg whitespace-nowrap transition-all ${active ? 'bg-amber-600 text-white shadow-2xs' : 'text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700/60'}`}
              >
                <Icon className="w-3.5 h-3.5" />
                <span>{cat.label}</span>
              </button>
            );
          })}
        </div>

        <div className="relative min-w-[220px]">
          <Search className="w-3.5 h-3.5 text-gray-400 absolute left-2.5 top-2.5" />
          <input
            type="text"
            placeholder="搜索技能名称或工具..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="w-full pl-8 pr-3 py-1 text-xs rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900/60 text-gray-900 dark:text-white placeholder-gray-400 focus:outline-none focus:ring-1 focus:ring-amber-500"
          />
        </div>
      </div>

      {/* 技能卡片列表 */}
      {filteredSkills.length === 0 ? (
        <div className="p-10 text-center bg-white dark:bg-gray-800/50 rounded-xl border border-dashed border-gray-200 dark:border-gray-800">
          <Sparkles className="w-8 h-8 text-gray-300 dark:text-gray-600 mx-auto mb-2" />
          <p className="text-sm font-medium text-gray-700 dark:text-gray-300">未找到匹配的 Agent 技能</p>
          <p className="text-xs text-gray-400 mt-1">可在上方切换分类筛选或点击「新建技能」添加</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {filteredSkills.map(skill => {
            const isToggling = !!togglingSkill[skill.id];
            const isCustom = skill.id.startsWith('skill_') || skill.id.startsWith('custom_');

            return (
              <div
                key={skill.id}
                className={`flex flex-col justify-between p-4 rounded-xl border transition-all duration-150 ${skill.enabled ? 'border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-800/90 shadow-2xs hover:shadow-xs' : 'border-gray-200/60 dark:border-gray-800/60 bg-gray-50/60 dark:bg-gray-850/40 opacity-75'}`}
              >
                <div>
                  {/* 头部：图标、名称与启停开关 */}
                  <div className="flex items-start justify-between gap-3 mb-2">
                    <div className="flex items-center gap-2.5">
                      <div className="p-2 rounded-lg bg-amber-50 dark:bg-amber-950/60 text-amber-600 dark:text-amber-400 border border-amber-200/50 dark:border-amber-800/50 shrink-0">
                        {skill.id === 'git-workflow' ? <GitBranch className="w-4 h-4" /> :
                         skill.id === 'test-driven-development' ? <Bug className="w-4 h-4" /> :
                         skill.id === 'browser-automation' ? <Globe className="w-4 h-4" /> :
                         <Lock className="w-4 h-4" />}
                      </div>
                      <div className="min-w-0">
                        <div className="flex items-center gap-1.5">
                          <h3 className="text-sm font-semibold text-gray-900 dark:text-white truncate">
                            {skill.name}
                          </h3>
                          <span className="px-1.5 py-0.2 text-[10px] font-mono rounded bg-gray-100 dark:bg-gray-750 text-gray-500">
                            v{skill.version || '1.0.0'}
                          </span>
                        </div>
                        <div className="text-[11px] font-mono text-gray-400 truncate">
                          {skill.id}
                        </div>
                      </div>
                    </div>

                    <button
                      onClick={() => handleToggleSkill(skill.id, skill.enabled)}
                      disabled={isToggling}
                      className={`relative inline-flex h-4.5 w-8 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none ${skill.enabled ? 'bg-amber-600' : 'bg-gray-300 dark:bg-gray-600'}`}
                      title={skill.enabled ? '点击禁用' : '点击启用'}
                    >
                      <span
                        className={`pointer-events-none inline-block h-3.5 w-3.5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ${skill.enabled ? 'translate-x-3.5' : 'translate-x-0'}`}
                      />
                    </button>
                  </div>

                  {/* 简介 */}
                  <p className="text-xs text-gray-600 dark:text-gray-400 leading-relaxed line-clamp-2 mb-2.5">
                    {skill.description}
                  </p>

                  {/* 适用工具清单 */}
                  {skill.tools && skill.tools.length > 0 && (
                    <div className="flex items-center gap-1.5 flex-wrap mb-3 text-[11px]">
                      <span className="text-gray-400">工具:</span>
                      {skill.tools.slice(0, 5).map(t => (
                        <span
                          key={t}
                          className="px-1.5 py-0.5 font-mono text-[10px] bg-gray-50 dark:bg-gray-900 text-gray-600 dark:text-gray-300 rounded border border-gray-200/80 dark:border-gray-750"
                        >
                          {t}
                        </span>
                      ))}
                      {skill.tools.length > 5 && (
                        <span className="text-gray-400 text-[10px]">+{skill.tools.length - 5}</span>
                      )}
                    </div>
                  )}
                </div>

                {/* 卡片底部操作按钮 */}
                <div className="pt-2.5 border-t border-gray-100 dark:border-gray-800 flex items-center justify-between gap-1.5">
                  <div className="flex items-center gap-1.5 flex-wrap">
                    <a
                      href={`/api/v1/skills/${skill.id}/download`}
                      download={`${skill.id}.zip`}
                      className="px-2 py-1 text-xs font-medium text-emerald-700 dark:text-emerald-300 hover:bg-emerald-50 dark:hover:bg-emerald-950/60 rounded transition-colors inline-flex items-center gap-1 border border-emerald-200 dark:border-emerald-800/80"
                      title="直接下载 .zip 压缩包"
                    >
                      <DownloadCloud className="w-3.5 h-3.5" />
                      下载 .zip
                    </a>
                    <button
                      onClick={() => { setManifestSkill(skill); setManifestTab('skill_md'); }}
                      className="px-2 py-1 text-xs font-medium text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-750 rounded transition-colors inline-flex items-center gap-1 border border-gray-200 dark:border-gray-700"
                    >
                      <FileText className="w-3.5 h-3.5" />
                      查看 SOP
                    </button>
                    <button
                      onClick={() => { setSelectedSkillForGuide(skill); setClientGuideModalOpen(true); }}
                      className="px-2 py-1 text-xs font-medium text-indigo-600 dark:text-indigo-400 hover:bg-indigo-50 dark:hover:bg-indigo-950/50 rounded transition-colors inline-flex items-center gap-1"
                    >
                      <Terminal className="w-3.5 h-3.5" />
                      接入
                    </button>
                    <button
                      onClick={() => handleOpenTest(skill)}
                      className="px-2 py-1 text-xs font-medium text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 rounded transition-colors inline-flex items-center gap-1"
                    >
                      <Play className="w-3 h-3" />
                      演练
                    </button>
                  </div>

                  {isCustom && (
                    <button
                      onClick={() => handleDeleteSkill(skill.id)}
                      className="p-1 text-gray-400 hover:text-rose-500 rounded transition-colors"
                      title="删除自定义技能"
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
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm">
          <div className="bg-white dark:bg-gray-850 rounded-xl shadow-2xl border border-gray-200 dark:border-gray-750 max-w-2xl w-full p-6 space-y-4 max-h-[90vh] overflow-y-auto">
            <div className="flex items-center justify-between pb-3 border-b border-gray-200 dark:border-gray-800">
              <div className="flex items-center gap-2.5">
                <div className="p-2 bg-gradient-to-br from-amber-500 to-orange-600 text-white rounded-lg">
                  <Terminal className="w-5 h-5" />
                </div>
                <div>
                  <h3 className="text-base font-bold text-gray-900 dark:text-white">
                    智能体客户端接入示例
                  </h3>
                  <p className="text-xs text-gray-500 dark:text-gray-400">
                    {selectedSkillForGuide ? `将 [${selectedSkillForGuide.name}] 导入您的 Agent 编程环境` : '将 Agent Skills 规范集成到各大主流智能体客户端'}
                  </p>
                </div>
              </div>
              <button
                onClick={() => setClientGuideModalOpen(false)}
                className="p-1.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 rounded-lg"
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
                      <span>SOP 规范正文与客户端接入完全解耦</span>
                    </div>
                    <a
                      href={`/api/v1/skills/${currentId}/download`}
                      download={`${currentId}.zip`}
                      className="inline-flex items-center gap-1 px-2.5 py-1 text-[11px] font-semibold bg-emerald-600 hover:bg-emerald-700 text-white rounded transition-colors"
                    >
                      <DownloadCloud className="w-3.5 h-3.5" />
                      一键下载 {currentId}.zip
                    </a>
                  </div>
                  <p>
                    技能遵循 <strong>agentskills.io</strong> 开放标准以标准 ZIP 压缩包分发。<code className="font-mono bg-white/70 dark:bg-gray-800 px-1 py-0.5 rounded">SKILL.md</code> 内部保持纯净的 SOP 规范与规则契约，不掺杂任何特定客户端安装脚本。工具由本机系统环境或 MCP 协议广场提供，<strong>零网络下载等待</strong>。
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
                          <span>步骤 1：下载并解压标准技能包至 Claude Code 技能目录</span>
                          <button
                            onClick={() => handleCopyText(`mkdir -p .claude/skills && curl -sL http://localhost:8080/api/v1/skills/${currentId}/download -o ${currentId}.zip && unzip -q -o ${currentId}.zip -d .claude/skills/ && rm -f ${currentId}.zip`, 'cmd-claude-curl')}
                            className="text-amber-600 hover:underline inline-flex items-center gap-1 text-[11px]"
                          >
                            {copiedKey === 'cmd-claude-curl' ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                            复制一键命令
                          </button>
                        </div>
                        <pre className="p-2.5 bg-gray-900 text-gray-100 rounded font-mono text-[11px] overflow-x-auto">
{`# 一键下载解压标准包 (${currentId}.zip)
mkdir -p .claude/skills
curl -sL http://localhost:8080/api/v1/skills/${currentId}/download -o ${currentId}.zip
unzip -q -o ${currentId}.zip -d .claude/skills/
rm -f ${currentId}.zip`}
                        </pre>
                      </div>

                      <div className="p-3 bg-gray-50 dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800 space-y-2">
                        <div className="font-semibold text-gray-800 dark:text-gray-200">
                          步骤 2：启动 Claude Code，智能体将自动索引并渐进式激活该技能
                        </div>
                        <pre className="p-2.5 bg-gray-900 text-gray-100 rounded font-mono text-[11px]">
{`claude
# 体验：直接在对话中提出需求，Claude Code 会自动读取 .claude/skills/${currentId}/SKILL.md 标准流程执行！`}
                        </pre>
                      </div>
                    </div>
                  )}

                  {activeClientTab === 'opencode' && (
                    <div className="space-y-3 text-xs">
                      <div className="p-3 bg-gray-50 dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800 space-y-2">
                        <div className="font-semibold text-gray-800 dark:text-gray-200 flex items-center justify-between">
                          <span>步骤 1：下载并解压标准技能包至 OpenCode 技能目录</span>
                          <button
                            onClick={() => handleCopyText(`mkdir -p .opencode/skills && curl -sL http://localhost:8080/api/v1/skills/${currentId}/download -o ${currentId}.zip && unzip -q -o ${currentId}.zip -d .opencode/skills/ && rm -f ${currentId}.zip`, 'cmd-opencode-curl')}
                            className="text-amber-600 hover:underline inline-flex items-center gap-1 text-[11px]"
                          >
                            {copiedKey === 'cmd-opencode-curl' ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                            复制一键命令
                          </button>
                        </div>
                        <pre className="p-2.5 bg-gray-900 text-gray-100 rounded font-mono text-[11px] overflow-x-auto">
{`# 一键下载解压标准包 (${currentId}.zip)
mkdir -p .opencode/skills
curl -sL http://localhost:8080/api/v1/skills/${currentId}/download -o ${currentId}.zip
unzip -q -o ${currentId}.zip -d .opencode/skills/
rm -f ${currentId}.zip`}
                        </pre>
                      </div>

                      <div className="p-3 bg-gray-50 dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800 space-y-2">
                        <div className="font-semibold text-gray-800 dark:text-gray-200">
                          步骤 2：启动 OpenCode
                        </div>
                        <pre className="p-2.5 bg-gray-900 text-gray-100 rounded font-mono text-[11px]">
{`opencode
# OpenCode 原生兼容 agentskills.io 开放标准，遇到触发词自动载入 SOP 并调用工具`}
                        </pre>
                      </div>
                    </div>
                  )}

                  {activeClientTab === 'codex' && (
                    <div className="space-y-3 text-xs">
                      <div className="font-semibold text-gray-800 dark:text-gray-200 flex items-center justify-between">
                        <span>Python SDK (OpenAI / Codex API) 载入与网关调用示例：</span>
                        <button
                          onClick={() => handleCopyText(`import os\nimport urllib.request\nimport zipfile\nimport pathlib\nfrom openai import OpenAI\n\n# 1. 自动拉取技能包并解压\nskill_id = "${currentId}"\nzip_path = f"{skill_id}.zip"\nurllib.request.urlretrieve(f"http://localhost:8080/api/v1/skills/{skill_id}/download", zip_path)\nwith zipfile.ZipFile(zip_path, "r") as z:\n    z.extractall(".skills/")\nos.remove(zip_path)\n\n# 2. 读取解压后的纯净 SKILL.md 作为 System SOP\nskill_sop = pathlib.Path(f".skills/{skill_id}/SKILL.md").read_text(encoding="utf-8")\n\n# 3. 接入 airoute 统一网关\nclient = OpenAI(base_url="http://localhost:8080/v1", api_key="sk-airoute-key")\nresponse = client.chat.completions.create(\n    model="claude-3-7-sonnet",\n    messages=[\n        {"role": "system", "content": f"Follow this Skill SOP:\\n{skill_sop}"},\n        {"role": "user", "content": "请按照工作流规范执行当前任务"}\n    ]\n)\nprint(response.choices[0].message.content)`, 'cmd-codex')}
                          className="text-amber-600 hover:underline inline-flex items-center gap-1 text-[11px]"
                        >
                          {copiedKey === 'cmd-codex' ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                          复制代码
                        </button>
                      </div>
                      <pre className="p-3 bg-gray-900 text-gray-100 rounded-lg font-mono text-[11px] overflow-x-auto">
{`import os
import urllib.request
import zipfile
import pathlib
from openai import OpenAI

# 1. 自动拉取技能包并解压
skill_id = "${currentId}"
zip_path = f"{skill_id}.zip"
urllib.request.urlretrieve(f"http://localhost:8080/api/v1/skills/{skill_id}/download", zip_path)
with zipfile.ZipFile(zip_path, "r") as z:
    z.extractall(".skills/")
os.remove(zip_path)

# 2. 读取解压后的纯净 SKILL.md 作为 System SOP
skill_sop = pathlib.Path(f".skills/{skill_id}/SKILL.md").read_text(encoding="utf-8")

# 3. 接入 airoute 统一网关
client = OpenAI(base_url="http://localhost:8080/v1", api_key="sk-airoute-key")
response = client.chat.completions.create(
    model="claude-3-7-sonnet",
    messages=[
        {"role": "system", "content": f"Follow this Skill SOP:\\n{skill_sop}"},
        {"role": "user", "content": "请按照工作流规范执行当前任务"}
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
                          <span>Cursor / Cline 导入方案</span>
                          <button
                            onClick={() => handleCopyText(`mkdir -p .cursor/skills && curl -sL http://localhost:8080/api/v1/skills/${currentId}/download -o ${currentId}.zip && unzip -q -o ${currentId}.zip -d .cursor/skills/ && rm -f ${currentId}.zip`, 'cmd-cursor')}
                            className="text-amber-600 hover:underline inline-flex items-center gap-1 text-[11px]"
                          >
                            {copiedKey === 'cmd-cursor' ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                            复制一键命令
                          </button>
                        </div>
                        <pre className="p-2.5 bg-gray-900 text-gray-100 rounded font-mono text-[11px] overflow-x-auto">
{`# 方案 A: 解压至 Cursor 技能目录
mkdir -p .cursor/skills
curl -sL http://localhost:8080/api/v1/skills/${currentId}/download -o ${currentId}.zip
unzip -q -o ${currentId}.zip -d .cursor/skills/
rm -f ${currentId}.zip

# 方案 B: 直接追加至项目根目录的 .cursorrules
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
                关闭
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 技能详情 / SKILL.md / ZIP 归档包与元数据弹窗 */}
      {manifestSkill && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm">
          <div className="bg-white dark:bg-gray-850 rounded-xl shadow-2xl border border-gray-200 dark:border-gray-750 max-w-3xl w-full p-6 space-y-4 max-h-[90vh] overflow-y-auto">
            {/* 模态框头部 */}
            <div className="flex items-center justify-between pb-3 border-b border-gray-200 dark:border-gray-800">
              <div className="flex items-center gap-2.5">
                <div className="p-2 bg-amber-100 dark:bg-amber-900/50 text-amber-600 dark:text-amber-400 rounded-lg">
                  <FileText className="w-5 h-5" />
                </div>
                <div>
                  <h3 className="text-base font-bold text-gray-900 dark:text-white flex items-center gap-2">
                    {manifestSkill.name}
                    <span className="text-xs font-mono font-normal text-amber-600 bg-amber-50 dark:bg-amber-950/60 px-1.5 py-0.5 rounded border border-amber-200 dark:border-amber-800">
                      {manifestSkill.id}
                    </span>
                  </h3>
                  <p className="text-xs text-gray-500 dark:text-gray-400">
                    遵循 agentskills.io 开放标准的智能体 SOP 标准包与元数据契约
                  </p>
                </div>
              </div>
              <div className="flex items-center gap-2">
                <a
                  href={`/api/v1/skills/${manifestSkill.id}/download`}
                  download={`${manifestSkill.id}.zip`}
                  className="inline-flex items-center gap-1 px-2.5 py-1.5 text-xs font-semibold bg-emerald-600 hover:bg-emerald-700 text-white rounded-lg transition-colors shadow-2xs"
                  title="下载包含 SKILL.md 与脚本的 agentskills.io 标准 ZIP 压缩包"
                >
                  <DownloadCloud className="w-3.5 h-3.5" />
                  <span>下载 .zip 包</span>
                </a>
                <button
                  onClick={() => { setSelectedSkillForGuide(manifestSkill); setClientGuideModalOpen(true); }}
                  className="inline-flex items-center gap-1 px-2.5 py-1.5 text-xs font-medium text-indigo-700 dark:text-indigo-300 bg-indigo-50 dark:bg-indigo-950/60 border border-indigo-200 dark:border-indigo-800 rounded-lg hover:bg-indigo-100 dark:hover:bg-indigo-900/40 transition-colors"
                >
                  <Terminal className="w-3.5 h-3.5" />
                  <span>接入客户端</span>
                </button>
                <button
                  onClick={() => setManifestSkill(null)}
                  className="p-1.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 rounded-lg"
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
                      ? 'bg-amber-600 text-white shadow-xs'
                      : 'text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800'
                  }`}
                >
                  <FileText className="w-3.5 h-3.5" />
                  <span>SKILL.md (纯净 SOP)</span>
                </button>
                <button
                  onClick={() => setManifestTab('bundle_tree')}
                  className={`flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg transition-colors ${
                    manifestTab === 'bundle_tree'
                      ? 'bg-amber-600 text-white shadow-xs'
                      : 'text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800'
                  }`}
                >
                  <FolderTree className="w-3.5 h-3.5" />
                  <span>归档包目录树 (ZIP Tree)</span>
                </button>
                <button
                  onClick={() => setManifestTab('metadata')}
                  className={`flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg transition-colors ${
                    manifestTab === 'metadata'
                      ? 'bg-amber-600 text-white shadow-xs'
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
                  <span>{copiedKey === 'manifest-full' ? '已复制' : '复制 SOP'}</span>
                </button>
              )}
            </div>

            {/* Tab 1: SKILL.md */}
            {manifestTab === 'skill_md' && (
              <div className="space-y-2">
                <div className="flex items-center justify-between text-[11px] text-gray-500 dark:text-gray-400 px-1">
                  <span>纯业务 SOP 指令契约 (不包含客户端下载逻辑，三级渐进披露规范)</span>
                  <span className="font-mono">{manifestSkill.manifest.length} 字符</span>
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
                <div className="p-3 bg-gradient-to-r from-gray-50 to-slate-100 dark:from-gray-900 dark:to-gray-850 rounded-xl border border-gray-200 dark:border-gray-800 flex items-start justify-between gap-3 text-xs">
                  <div className="space-y-1">
                    <div className="font-semibold text-gray-800 dark:text-gray-200 flex items-center gap-1.5">
                      {storageStatus?.driver === 's3' ? (
                        <>
                          <Server className="w-4 h-4 text-emerald-600" />
                          <span>分布式对象存储已激活 (RustFS / S3: bucket <code>{storageStatus.s3_bucket || 'skills'}</code>)</span>
                        </>
                      ) : (
                        <>
                          <HardDrive className="w-4 h-4 text-slate-500" />
                          <span>本地文件驱动已激活 (Local: <code>{storageStatus?.local_path || 'data/skills_storage'}</code>)</span>
                        </>
                      )}
                    </div>
                    <p className="text-gray-500 dark:text-gray-400 text-[11px]">
                      {storageStatus?.driver === 's3'
                        ? '下载请求由网关直接签发 AWS SigV4 预签名临时 URL，302 重定向直连 RustFS 高速传输，完全释放网关带宽。'
                        : '归档包由网关本地磁盘存储提供流式下载与持久化。配置 STORAGE_DRIVER=s3 可无缝切换至 RustFS 对象存储。'}
                    </p>
                  </div>
                  <span className="px-2 py-0.5 rounded text-[10px] font-mono font-medium bg-emerald-100 dark:bg-emerald-950/60 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800 shrink-0">
                    S3 Key: skills/{manifestSkill.id}.zip
                  </span>
                </div>

                {/* 目录树预览 */}
                <div className="bg-gray-900 text-gray-100 p-4 rounded-xl font-mono text-xs border border-gray-800 space-y-1.5">
                  <div className="text-amber-400 font-bold flex items-center gap-1.5 pb-2 border-b border-gray-800">
                    <Folder className="w-4 h-4" />
                    <span>{manifestSkill.id}.zip (agentskills.io 标准封装包)</span>
                  </div>
                  <div className="pl-4 space-y-1 text-gray-300 text-[11px]">
                    <div className="text-blue-400 flex items-center gap-1.5">
                      <span>└── 📁 {manifestSkill.id}/</span>
                    </div>
                    <div className="pl-6 space-y-1 text-gray-300">
                      <div className="flex items-center justify-between text-emerald-300">
                        <span>├── 📄 SKILL.md</span>
                        <span className="text-gray-500 text-[10px]"># 核心 SOP 流程契约与三级执行阶段</span>
                      </div>
                      <div className="flex items-center justify-between text-yellow-300">
                        <span>├── 📄 metadata.json</span>
                        <span className="text-gray-500 text-[10px]"># 技能描述、版本与作者元数据</span>
                      </div>
                      <div className="text-blue-300">
                        <span>├── 📁 scripts/</span>
                      </div>
                      <div className="pl-6 flex items-center justify-between text-purple-300">
                        <span>│   └── ⚙️ run.sh</span>
                        <span className="text-gray-500 text-[10px]"># 可选本机自动化演练或 Hook 脚本</span>
                      </div>
                      <div className="text-blue-300">
                        <span>└── 📁 references/</span>
                      </div>
                      <div className="pl-6 flex items-center justify-between text-sky-300">
                        <span>    └── 📑 context.md</span>
                        <span className="text-gray-500 text-[10px]"># 补充上下文与规则详情</span>
                      </div>
                    </div>
                  </div>
                </div>

                {/* 快速拉取命令 */}
                <div className="p-3 bg-gray-50 dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 space-y-1.5 text-xs">
                  <div className="flex items-center justify-between text-gray-700 dark:text-gray-300 font-medium">
                    <span>终端下载并解压命令 (Terminal Quickstart)</span>
                    <button
                      onClick={() => handleCopyText(`curl -sL http://localhost:8080/api/v1/skills/${manifestSkill.id}/download -o ${manifestSkill.id}.zip && unzip -q -o ${manifestSkill.id}.zip -d skills/`, 'tree-curl')}
                      className="text-amber-600 hover:underline inline-flex items-center gap-1 text-[11px]"
                    >
                      {copiedKey === 'tree-curl' ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                      复制命令
                    </button>
                  </div>
                  <pre className="p-2 bg-gray-900 text-gray-200 rounded font-mono text-[11px] overflow-x-auto">
{`curl -sL http://localhost:8080/api/v1/skills/${manifestSkill.id}/download -o ${manifestSkill.id}.zip
unzip -q -o ${manifestSkill.id}.zip -d skills/`}
                  </pre>
                </div>
              </div>
            )}

            {/* Tab 3: metadata.json */}
            {manifestTab === 'metadata' && (
              <div className="space-y-2">
                <div className="flex items-center justify-between text-[11px] text-gray-500 dark:text-gray-400 px-1">
                  <span>agentskills.io 元数据声明契约</span>
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
                    className="inline-flex items-center gap-1 text-amber-600 hover:underline"
                  >
                    {copiedKey === 'meta-json' ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                    <span>{copiedKey === 'meta-json' ? '已复制' : '复制 JSON'}</span>
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
                关闭
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 模拟演练弹窗 */}
      {testModalSkill && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm">
          <div className="bg-white dark:bg-gray-850 rounded-xl shadow-2xl border border-gray-200 dark:border-gray-750 max-w-xl w-full p-6 space-y-4 max-h-[90vh] overflow-y-auto">
            <div className="flex items-center justify-between pb-3 border-b border-gray-200 dark:border-gray-800">
              <div className="flex items-center gap-2.5">
                <div className="p-2 bg-gradient-to-br from-amber-500 to-orange-600 text-white rounded-lg">
                  <Play className="w-5 h-5" />
                </div>
                <div>
                  <h3 className="text-base font-bold text-gray-900 dark:text-white">
                    模拟 SOP 执行演练: {testModalSkill.name}
                  </h3>
                  <p className="text-xs text-gray-500 dark:text-gray-400">
                    模拟智能体在此技能指引下的三级执行链路与输出
                  </p>
                </div>
              </div>
              <button
                onClick={() => setTestModalSkill(null)}
                className="p-1.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 rounded-lg"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <div className="space-y-3">
              <div>
                <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
                  用户意图与输入场景 (Prompt / Scenario)
                </label>
                <textarea
                  rows="3"
                  value={testPrompt}
                  onChange={(e) => setTestPrompt(e.target.value)}
                  className="w-full px-3 py-2 text-xs rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900 text-gray-900 dark:text-white focus:outline-none focus:ring-1 focus:ring-amber-500"
                />
              </div>

              <div className="flex justify-end">
                <button
                  onClick={handleExecuteTest}
                  disabled={testingSkill}
                  className="inline-flex items-center gap-1.5 px-4 py-2 text-xs font-semibold text-white bg-amber-600 rounded-lg hover:bg-amber-700 transition-colors shadow-sm disabled:opacity-50"
                >
                  <Play className={`w-3.5 h-3.5 ${testingSkill ? 'animate-pulse' : ''}`} />
                  <span>{testingSkill ? '执行演练中...' : '启动模拟演练'}</span>
                </button>
              </div>

              {testOutput && (
                <div className="space-y-2 pt-2 border-t border-gray-100 dark:border-gray-800">
                  <div className="flex items-center gap-1.5 text-xs font-semibold text-emerald-600 dark:text-emerald-400">
                    <CheckCircle2 className="w-4 h-4" />
                    <span>执行跟踪链路 (Execution Trace)</span>
                  </div>
                  <div className="bg-gray-900 text-gray-100 p-3.5 rounded-xl font-mono text-xs overflow-x-auto space-y-1.5 border border-gray-800">
                    {testOutput.trace?.map((line, idx) => (
                      <div key={idx} className="text-gray-300 text-[11px]">{line}</div>
                    ))}
                    <div className="mt-2 pt-2 border-t border-gray-800 text-emerald-400 font-bold">
                      {testOutput.verdict}
                    </div>
                  </div>
                </div>
              )}
            </div>

            <div className="flex justify-end pt-2 border-t border-gray-100 dark:border-gray-800">
              <button
                onClick={() => setTestModalSkill(null)}
                className="px-4 py-2 text-xs font-medium text-gray-700 dark:text-gray-300 bg-gray-100 dark:bg-gray-800 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-700"
              >
                关闭
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 新建自定义技能模态框 */}
      {createModalOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm">
          <div className="bg-white dark:bg-gray-850 rounded-xl shadow-2xl border border-gray-200 dark:border-gray-750 max-w-xl w-full p-6 space-y-4 max-h-[90vh] overflow-y-auto">
            <div className="flex items-center justify-between pb-3 border-b border-gray-200 dark:border-gray-800">
              <div className="flex items-center gap-2.5">
                <div className="p-2 bg-gradient-to-br from-amber-500 to-orange-600 text-white rounded-lg">
                  <Plus className="w-5 h-5" />
                </div>
                <div>
                  <h3 className="text-base font-bold text-gray-900 dark:text-white">
                    新建自定义 Agent 技能
                  </h3>
                  <p className="text-xs text-gray-500 dark:text-gray-400">
                    遵循 agentskills.io 规范编写自定义工作流 SOP
                  </p>
                </div>
              </div>
              <button
                onClick={() => setCreateModalOpen(false)}
                className="p-1.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 rounded-lg"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <form onSubmit={handleCreateSkill} className="space-y-4">
              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
                    技能 ID (如: api-designer) *
                  </label>
                  <input
                    type="text"
                    required
                    placeholder="如: api-designer"
                    value={newSkill.id}
                    onChange={(e) => setNewSkill({ ...newSkill, id: e.target.value })}
                    className="w-full px-3 py-2 text-xs rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900 text-gray-900 dark:text-white"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
                    技能名称 *
                  </label>
                  <input
                    type="text"
                    required
                    placeholder="如: OpenAPI 3.0 接口规范设计"
                    value={newSkill.name}
                    onChange={(e) => setNewSkill({ ...newSkill, name: e.target.value })}
                    className="w-full px-3 py-2 text-xs rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900 text-gray-900 dark:text-white"
                  />
                </div>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
                    分类领域
                  </label>
                  <select
                    value={newSkill.category}
                    onChange={(e) => setNewSkill({ ...newSkill, category: e.target.value })}
                    className="w-full px-3 py-2 text-xs rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900 text-gray-900 dark:text-white"
                  >
                    <option value="dev">研发工程 (dev)</option>
                    <option value="test">质量保障 (test)</option>
                    <option value="automation">端到端自动化 (automation)</option>
                    <option value="security">代码安全审计 (security)</option>
                  </select>
                </div>
                <div>
                  <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
                    依赖工具 (逗号分隔)
                  </label>
                  <input
                    type="text"
                    placeholder="如: run_command, view_file"
                    value={newSkill.tools}
                    onChange={(e) => setNewSkill({ ...newSkill, tools: e.target.value })}
                    className="w-full px-3 py-2 text-xs rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900 text-gray-900 dark:text-white font-mono"
                  />
                </div>
              </div>

              <div>
                <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
                  简要描述与触发时机 *
                </label>
                <textarea
                  rows="2"
                  required
                  placeholder="说明该技能的功能以及智能体在何时应当激活..."
                  value={newSkill.description}
                  onChange={(e) => setNewSkill({ ...newSkill, description: e.target.value })}
                  className="w-full px-3 py-2 text-xs rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900 text-gray-900 dark:text-white"
                />
              </div>

              <div>
                <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
                  SKILL.md 规范内容 (Markdown)
                </label>
                <textarea
                  rows="5"
                  placeholder={`---
name: ${newSkill.id || 'my-skill'}
description: 描述内容
allowed-tools:
  - run_command
---

# 技能执行指引
1. 第一步...
2. 第二步...`}
                  value={newSkill.manifest}
                  onChange={(e) => setNewSkill({ ...newSkill, manifest: e.target.value })}
                  className="w-full px-3 py-2 font-mono text-xs rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-900 text-gray-100"
                />
              </div>

              <div className="flex items-center justify-end gap-2.5 pt-3 border-t border-gray-200 dark:border-gray-800">
                <button
                  type="button"
                  onClick={() => setCreateModalOpen(false)}
                  className="px-4 py-2 text-xs font-medium text-gray-700 dark:text-gray-300 bg-gray-100 dark:bg-gray-800 rounded-lg hover:bg-gray-200"
                >
                  取消
                </button>
                <button
                  type="submit"
                  className="px-4 py-2 text-xs font-medium text-white bg-amber-600 rounded-lg hover:bg-amber-700 shadow-sm"
                >
                  创建技能
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
