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
  ArrowRight
} from 'lucide-react';

export default function SkillHubView({ adminFetch, onCopy, showToast }) {
  const [skills, setSkills] = useState([]);
  const [loading, setLoading] = useState(false);
  const [selectedCategory, setSelectedCategory] = useState('all');
  const [searchQuery, setSearchQuery] = useState('');
  const [togglingSkill, setTogglingSkill] = useState({});

  // Modals
  const [manifestSkill, setManifestSkill] = useState(null);
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

  useEffect(() => {
    fetchSkills();
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

  const getToolTypeBadge = (skill) => {
    if (skill.id === 'browser-automation') {
      return (
        <span className="inline-flex items-center gap-1 px-2 py-0.5 text-[11px] font-medium rounded-full bg-blue-50 dark:bg-blue-950/40 text-blue-700 dark:text-blue-300 border border-blue-200 dark:border-blue-800">
          <Globe className="w-3 h-3" />
          MCP 协议扩展工具 (无需下载，MCP挂载)
        </span>
      );
    }
    return (
      <span className="inline-flex items-center gap-1 px-2 py-0.5 text-[11px] font-medium rounded-full bg-emerald-50 dark:bg-emerald-950/40 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800">
        <Terminal className="w-3 h-3" />
        宿主原生执行工具 (无需下载，调用本机CLI)
      </span>
    );
  };

  return (
    <div className="space-y-6">
      {/* 顶部标题与说明 */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 pb-2 border-b border-gray-100 dark:border-gray-800">
        <div>
          <div className="flex items-center gap-3">
            <div className="p-2.5 bg-gradient-to-br from-amber-500 to-orange-600 rounded-xl text-white shadow-md shadow-amber-500/20">
              <Sparkles className="w-6 h-6" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <h1 className="text-xl font-bold text-gray-900 dark:text-white">
                  Agent Skill Hub (智能体技能中心)
                </h1>
                <span className="px-2.5 py-0.5 text-xs font-semibold rounded-full bg-amber-50 dark:bg-amber-900/40 text-amber-700 dark:text-amber-300 border border-amber-200 dark:border-amber-800/50">
                  agentskills.io 开放标准
                </span>
                <span className="px-2 py-0.5 text-[11px] font-medium rounded-full bg-purple-50 dark:bg-purple-900/30 text-purple-600 dark:text-purple-400 border border-purple-200 dark:border-purple-800/40">
                  零下载 · 三级渐进披露
                </span>
              </div>
              <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">
                专业智能体 SOP 标准作业流程：包含 Git 规范、TDD 测试驱动、Playwright 自动化与代码审计，支持一键接入 Claude Code、OpenCode、Codex 与 Cursor。
              </p>
            </div>
          </div>
        </div>

        <div className="flex items-center gap-2.5 flex-wrap">
          <button
            onClick={() => { setSelectedSkillForGuide(null); setClientGuideModalOpen(true); }}
            className="inline-flex items-center gap-1.5 px-3 py-2 text-xs font-medium text-amber-800 dark:text-amber-200 bg-amber-50 dark:bg-amber-950/60 border border-amber-300 dark:border-amber-800 rounded-lg hover:bg-amber-100 dark:hover:bg-amber-900/50 transition-colors shadow-sm"
          >
            <BookOpen className="w-4 h-4 text-amber-600 dark:text-amber-400" />
            接入 Claude Code / OpenCode / Codex
          </button>
          <button
            onClick={() => setCreateModalOpen(true)}
            className="inline-flex items-center gap-1.5 px-3.5 py-2 text-xs font-medium text-white bg-gradient-to-r from-amber-600 to-orange-600 rounded-lg hover:from-amber-700 hover:to-orange-700 transition-all shadow-md shadow-amber-500/20"
          >
            <Plus className="w-4 h-4" />
            新建自定义技能
          </button>
        </div>
      </div>

      {/* 核心原理解析横幅：解决用户关于“工具是否同步下载”的疑问 */}
      <div className="p-4 rounded-xl border border-blue-200/80 dark:border-blue-900/50 bg-gradient-to-r from-blue-50/70 via-indigo-50/50 to-white dark:from-blue-950/30 dark:via-gray-800 dark:to-gray-800 shadow-sm">
        <div className="flex items-start gap-3.5">
          <div className="p-2 bg-blue-500 text-white rounded-lg shadow-sm mt-0.5">
            <HelpCircle className="w-5 h-5" />
          </div>
          <div className="space-y-1.5 text-xs">
            <div className="font-bold text-gray-900 dark:text-white flex items-center gap-2">
              <span>Skill 与 Tool 的核心运行机制揭秘：工具需要同步下载吗？</span>
              <span className="px-1.5 py-0.2 bg-emerald-100 text-emerald-800 dark:bg-emerald-900/60 dark:text-emerald-300 rounded font-normal text-[10px]">
                完全不需要下载
              </span>
            </div>
            <div className="text-gray-600 dark:text-gray-300 leading-relaxed space-y-1">
              <p>
                <strong>1. Skill 是智能体的「大脑指南 (SOP)」</strong>：Skill 本质是遵循 <code className="bg-white/80 dark:bg-gray-700 px-1 py-0.5 rounded text-blue-600 dark:text-blue-400 font-mono">SKILL.md</code> 规范的作业指引文件，告诉智能体遇到特定场景时「怎么思考、按什么步骤检查与执行」。
              </p>
              <p>
                <strong>2. 工具由环境原生提供或通过 MCP 挂载</strong>：Skill 内部声明的工具并不是让网关去编译下载，而是调用<strong>宿主环境原生工具</strong>（如 Claude Code / OpenCode 内置的终端命令与文件改写工具）或<strong>MCP 广场中的远程协议工具</strong>（如 Playwright / GitHub MCP），零下载负担、零网络等待。
              </p>
              <p>
                <strong>3. 渐进式披露 (Progressive Disclosure)</strong>：Agent 启动时仅加载几十 tokens 的元数据，绝不撑爆上下文；只有用户触发该场景时，才按需将完整 SOP 读入上下文执行。
              </p>
            </div>
          </div>
        </div>
      </div>

      {/* 搜索与分类导航 */}
      <div className="flex flex-col md:flex-row items-stretch md:items-center justify-between gap-3 bg-white dark:bg-gray-800/80 p-3 rounded-xl border border-gray-200 dark:border-gray-800 shadow-sm">
        <div className="flex items-center gap-1 overflow-x-auto pb-1 md:pb-0 scrollbar-none">
          {categories.map(cat => {
            const Icon = cat.icon;
            const active = selectedCategory === cat.id;
            return (
              <button
                key={cat.id}
                onClick={() => setSelectedCategory(cat.id)}
                className={`flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg whitespace-nowrap transition-all ${active ? 'bg-amber-600 text-white shadow-sm' : 'text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700/60'}`}
              >
                <Icon className="w-3.5 h-3.5" />
                <span>{cat.label}</span>
              </button>
            );
          })}
        </div>

        <div className="relative min-w-[240px]">
          <Search className="w-4 h-4 text-gray-400 absolute left-3 top-2.5" />
          <input
            type="text"
            placeholder="搜索技能名称、SOP 工具或适用场景..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="w-full pl-9 pr-3 py-1.5 text-xs rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900/60 text-gray-900 dark:text-white placeholder-gray-400 focus:outline-none focus:ring-1 focus:ring-amber-500"
          />
        </div>
      </div>

      {/* 技能卡片列表 */}
      {filteredSkills.length === 0 ? (
        <div className="p-12 text-center bg-white dark:bg-gray-800/50 rounded-xl border border-dashed border-gray-200 dark:border-gray-800">
          <Sparkles className="w-10 h-10 text-gray-300 dark:text-gray-600 mx-auto mb-3" />
          <p className="text-sm font-medium text-gray-700 dark:text-gray-300">未找到匹配的 Agent 技能</p>
          <p className="text-xs text-gray-400 mt-1">可在上方切换分类筛选，或点击右上角「新建自定义技能」添加您的专属 SOP</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
          {filteredSkills.map(skill => {
            const isToggling = !!togglingSkill[skill.id];
            const isCustom = skill.id.startsWith('skill_') || skill.id.startsWith('custom_');

            return (
              <div
                key={skill.id}
                className={`flex flex-col justify-between p-5 rounded-xl border transition-all duration-200 ${skill.enabled ? 'border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-800/90 shadow-sm hover:shadow-md' : 'border-gray-200/60 dark:border-gray-800/60 bg-gray-50/70 dark:bg-gray-850/50 opacity-80'}`}
              >
                <div>
                  {/* 头部标题与开关 */}
                  <div className="flex items-start justify-between gap-3 mb-2.5">
                    <div className="flex items-center gap-3">
                      <div className="p-2.5 rounded-xl bg-amber-50 dark:bg-amber-950/60 text-amber-600 dark:text-amber-400 border border-amber-200/50 dark:border-amber-800/50">
                        {skill.id === 'git-workflow' ? <GitBranch className="w-5 h-5" /> :
                         skill.id === 'test-driven-development' ? <Bug className="w-5 h-5" /> :
                         skill.id === 'browser-automation' ? <Globe className="w-5 h-5" /> :
                         <Lock className="w-5 h-5" />}
                      </div>
                      <div>
                        <h3 className="text-sm font-bold text-gray-900 dark:text-white flex items-center gap-1.5">
                          {skill.name}
                        </h3>
                        <div className="text-[11px] font-mono text-gray-400 dark:text-gray-500">
                          {skill.id} · v{skill.version || '1.0.0'}
                        </div>
                      </div>
                    </div>

                    <button
                      onClick={() => handleToggleSkill(skill.id, skill.enabled)}
                      disabled={isToggling}
                      className={`relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none ${skill.enabled ? 'bg-amber-600' : 'bg-gray-300 dark:bg-gray-600'}`}
                      title={skill.enabled ? '点击禁用' : '点击启用'}
                    >
                      <span
                        className={`pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ${skill.enabled ? 'translate-x-4' : 'translate-x-0'}`}
                      />
                    </button>
                  </div>

                  {/* 工具运行方式标识 */}
                  <div className="mb-3">
                    {getToolTypeBadge(skill)}
                  </div>

                  {/* 描述信息 */}
                  <p className="text-xs text-gray-600 dark:text-gray-400 leading-relaxed mb-4">
                    {skill.description}
                  </p>

                  {/* 调用的工具清单 */}
                  <div className="mb-4 bg-gray-50 dark:bg-gray-900/50 p-3 rounded-lg border border-gray-100 dark:border-gray-800">
                    <div className="flex items-center justify-between text-[11px] font-medium text-gray-500 dark:text-gray-400 mb-1.5">
                      <span>包含执行工具 ({skill.tools ? skill.tools.length : 0})</span>
                      <span className="text-[10px] text-gray-400">无需额外下载</span>
                    </div>
                    <div className="flex flex-wrap gap-1.5">
                      {(skill.tools || []).map(t => (
                        <span
                          key={t}
                          className="px-2 py-0.5 text-[11px] font-mono bg-white dark:bg-gray-800 text-gray-700 dark:text-gray-300 rounded border border-gray-200 dark:border-gray-700 shadow-2xs"
                        >
                          {t}
                        </span>
                      ))}
                    </div>
                  </div>
                </div>

                {/* 卡片底部操作按钮 */}
                <div className="pt-3 border-t border-gray-100 dark:border-gray-800 flex items-center justify-between gap-2">
                  <div className="flex items-center gap-1.5 flex-wrap">
                    <button
                      onClick={() => setManifestSkill(skill)}
                      className="px-2.5 py-1 text-xs font-medium text-amber-700 dark:text-amber-300 hover:bg-amber-50 dark:hover:bg-amber-950/60 rounded-md transition-colors flex items-center gap-1"
                    >
                      <FileText className="w-3.5 h-3.5" />
                      查看 SKILL.md
                    </button>
                    <button
                      onClick={() => { setSelectedSkillForGuide(skill); setClientGuideModalOpen(true); }}
                      className="px-2.5 py-1 text-xs font-medium text-indigo-600 dark:text-indigo-400 hover:bg-indigo-50 dark:hover:bg-indigo-950/60 rounded-md transition-colors flex items-center gap-1"
                    >
                      <Terminal className="w-3.5 h-3.5" />
                      接入客户端
                    </button>
                    <button
                      onClick={() => handleOpenTest(skill)}
                      className="px-2.5 py-1 text-xs font-medium text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-md transition-colors flex items-center gap-1"
                    >
                      <Play className="w-3.5 h-3.5" />
                      模拟演练
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
            <div className="space-y-3">
              {activeClientTab === 'claude-code' && (
                <div className="space-y-3 text-xs">
                  <div className="p-3 bg-gray-50 dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800 space-y-2">
                    <div className="font-semibold text-gray-800 dark:text-gray-200 flex items-center justify-between">
                      <span>步骤 1：在项目根目录创建 Skill 目录并放置 SKILL.md</span>
                      <button
                        onClick={() => handleCopyText(`mkdir -p .claude/skills/${selectedSkillForGuide?.id || 'git-workflow'}`, 'cmd-claude-mkdir')}
                        className="text-amber-600 hover:underline inline-flex items-center gap-1 text-[11px]"
                      >
                        {copiedKey === 'cmd-claude-mkdir' ? <Check className="w-3 h-3" /> : <Copy className="w-3 h-3" />}
                        复制命令
                      </button>
                    </div>
                    <pre className="p-2.5 bg-gray-900 text-gray-100 rounded font-mono text-[11px]">
{`mkdir -p .claude/skills/${selectedSkillForGuide?.id || 'git-workflow'}
# 将该技能的 SKILL.md 写入该目录`}
                    </pre>
                  </div>

                  <div className="p-3 bg-gray-50 dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800 space-y-2">
                    <div className="font-semibold text-gray-800 dark:text-gray-200">
                      步骤 2：启动 Claude Code，智能体将自动索引并渐进式激活
                    </div>
                    <pre className="p-2.5 bg-gray-900 text-gray-100 rounded font-mono text-[11px]">
{`claude
# 体验：在对话中提出对应任务，Claude Code 会自动调用该技能的 SOP 指引！`}
                    </pre>
                  </div>
                </div>
              )}

              {activeClientTab === 'opencode' && (
                <div className="space-y-3 text-xs">
                  <div className="p-3 bg-gray-50 dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800 space-y-2">
                    <div className="font-semibold text-gray-800 dark:text-gray-200 flex items-center justify-between">
                      <span>步骤 1：保存 SKILL.md 至 OpenCode 技能目录</span>
                      <button
                        onClick={() => handleCopyText(`mkdir -p .opencode/skills/${selectedSkillForGuide?.id || 'git-workflow'}`, 'cmd-opencode-mkdir')}
                        className="text-amber-600 hover:underline inline-flex items-center gap-1 text-[11px]"
                      >
                        {copiedKey === 'cmd-opencode-mkdir' ? <Check className="w-3 h-3" /> : <Copy className="w-3 h-3" />}
                        复制命令
                      </button>
                    </div>
                    <pre className="p-2.5 bg-gray-900 text-gray-100 rounded font-mono text-[11px]">
{`mkdir -p .opencode/skills/${selectedSkillForGuide?.id || 'git-workflow'}
# 放置 SKILL.md 文件`}
                    </pre>
                  </div>

                  <p className="text-gray-500 text-[11px]">
                    OpenCode 原生兼容 <code className="text-amber-600 font-mono">agentskills.io</code> 开放标准，启动时只读取 L1 元数据，任务触发时自动载入 L2 SOP 流程并调用环境工具。
                  </p>
                </div>
              )}

              {activeClientTab === 'codex' && (
                <div className="space-y-3 text-xs">
                  <div className="font-semibold text-gray-800 dark:text-gray-200 flex items-center justify-between">
                    <span>Python SDK (OpenAI / Codex API) 载入示例：</span>
                    <button
                      onClick={() => handleCopyText(`import pathlib\nfrom openai import OpenAI\n\n# 读取 SKILL.md 规范作为 System Instructions\nskill_content = pathlib.Path(".skills/${selectedSkillForGuide?.id || 'git-workflow'}/SKILL.md").read_text()\n\nclient = OpenAI()\nresponse = client.chat.completions.create(\n    model="gpt-4o",\n    messages=[\n        {"role": "system", "content": f"Follow this Skill SOP:\\n{skill_content}"},\n        {"role": "user", "content": "请按照工作流规范检查代码改动并生成标准 PR"}\n    ]\n)`, 'cmd-codex')}
                      className="text-amber-600 hover:underline inline-flex items-center gap-1 text-[11px]"
                    >
                      {copiedKey === 'cmd-codex' ? <Check className="w-3 h-3" /> : <Copy className="w-3 h-3" />}
                      复制代码
                    </button>
                  </div>
                  <pre className="p-3 bg-gray-900 text-gray-100 rounded-lg font-mono text-[11px] overflow-x-auto">
{`import pathlib
from openai import OpenAI

# 读取 SKILL.md 规范作为 System Instructions
skill_path = pathlib.Path(".skills/${selectedSkillForGuide?.id || 'git-workflow'}/SKILL.md")
skill_content = skill_path.read_text(encoding="utf-8")

client = OpenAI()
response = client.chat.completions.create(
    model="gpt-4o",
    messages=[
        {"role": "system", "content": f"Follow this Skill SOP:\\n{skill_content}"},
        {"role": "user", "content": "请按照工作流规范检查代码改动并生成标准 PR"}
    ]
)`}
                  </pre>
                </div>
              )}

              {activeClientTab === 'cursor' && (
                <div className="space-y-3 text-xs">
                  <div className="p-3 bg-gray-50 dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800 space-y-2">
                    <div className="font-semibold text-gray-800 dark:text-gray-200">
                      Cursor / Roo-Code 项目规则配置
                    </div>
                    <p className="text-gray-500 text-[11px]">
                      在项目根目录创建 <code className="text-amber-600 font-mono">.cursorrules</code> 或放置在 <code className="text-amber-600 font-mono">.cursor/skills/</code> 中，Cursor 会自动遵循该 SOP 指令。
                    </p>
                  </div>
                </div>
              )}
            </div>

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

      {/* SKILL.md 完整规范弹窗 */}
      {manifestSkill && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm">
          <div className="bg-white dark:bg-gray-850 rounded-xl shadow-2xl border border-gray-200 dark:border-gray-750 max-w-3xl w-full p-6 space-y-4 max-h-[90vh] overflow-y-auto">
            <div className="flex items-center justify-between pb-3 border-b border-gray-200 dark:border-gray-800">
              <div className="flex items-center gap-2.5">
                <div className="p-2 bg-amber-100 dark:bg-amber-900/50 text-amber-600 dark:text-amber-400 rounded-lg">
                  <FileText className="w-5 h-5" />
                </div>
                <div>
                  <h3 className="text-base font-bold text-gray-900 dark:text-white flex items-center gap-2">
                    {manifestSkill.name}
                    <span className="text-xs font-mono font-normal text-amber-600">SKILL.md</span>
                  </h3>
                  <p className="text-xs text-gray-500 dark:text-gray-400">
                    遵循 agentskills.io 开放标准定义的三级渐进式 SOP 指令包
                  </p>
                </div>
              </div>
              <div className="flex items-center gap-2">
                <button
                  onClick={() => handleCopyText(manifestSkill.manifest, 'manifest-full')}
                  className="inline-flex items-center gap-1 px-2.5 py-1.5 text-xs font-medium bg-amber-50 dark:bg-amber-950/60 text-amber-700 dark:text-amber-300 border border-amber-200 dark:border-amber-800 rounded-lg hover:bg-amber-100 transition-colors"
                >
                  {copiedKey === 'manifest-full' ? <Check className="w-3.5 h-3.5 text-emerald-500" /> : <Copy className="w-3.5 h-3.5" />}
                  <span>{copiedKey === 'manifest-full' ? '已复制' : '复制 SKILL.md'}</span>
                </button>
                <button
                  onClick={() => setManifestSkill(null)}
                  className="p-1.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 rounded-lg"
                >
                  <X className="w-5 h-5" />
                </button>
              </div>
            </div>

            <div className="bg-gray-900 text-gray-100 p-4 rounded-xl font-mono text-xs overflow-x-auto max-h-[60vh] border border-gray-800">
              <pre>{manifestSkill.manifest}</pre>
            </div>

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
