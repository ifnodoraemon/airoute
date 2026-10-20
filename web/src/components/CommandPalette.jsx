import { useState, useEffect, useRef } from 'react';
import {
  Search,
  BarChart3,
  History,
  Cpu,
  Server,
  DollarSign,
  Key,
  ShieldCheck,
  Wallet,
  Terminal,
  Plus,
  Globe,
  LogOut,
  ExternalLink,
  Zap,
  CornerDownLeft,
  X,
  Sparkles,
  Coins
} from 'lucide-react';

export default function CommandPalette({
  isOpen,
  onClose,
  currentTab,
  setCurrentTab,
  setViewMode,
  models = [],
  channels = [],
  keys = [],
  adminUser,
  onOpenNewKey,
  onOpenNewChannel,
  toggleLang,
  lang,
  onLogout,
  showToast,
  onOpenCopilotWithQuery
}) {
  const [query, setQuery] = useState('');
  const [selectedIndex, setSelectedIndex] = useState(0);
  const inputRef = useRef(null);
  const listRef = useRef(null);

  const isZh = lang === 'zh';
  const isAdmin = adminUser?.role === 'admin';

  useEffect(() => {
    if (isOpen) {
      setQuery('');
      setSelectedIndex(0);
      setTimeout(() => inputRef.current?.focus(), 50);
    }
  }, [isOpen]);

  const catNav = isZh ? '页面导航' : 'Navigation';
  const catAction = isZh ? '快捷操作' : 'Actions';
  const catModels = isZh ? '挂载模型' : 'Models';

  // Build searchable items
  const navItems = [
    ...(isAdmin ? [
      { id: 'nav-dashboard', category: catNav, label: isZh ? '用量与业务概览大盘' : 'Telemetry & SLA Cockpit', icon: BarChart3, action: () => { setCurrentTab('dashboard'); setViewMode('console'); onClose(); } },
      { id: 'nav-models', category: catNav, label: isZh ? '模型路由与拓扑' : 'Model Routes & Topo', icon: Cpu, action: () => { setCurrentTab('models'); setViewMode('console'); onClose(); } },
      { id: 'nav-channels', category: catNav, label: isZh ? '上游服务商接入与体检' : 'Upstream Channels & Health', icon: Server, action: () => { setCurrentTab('channels'); setViewMode('console'); onClose(); } },
      { id: 'nav-reconciliation', category: catNav, label: isZh ? '供应商整体对账与结算' : 'Supplier Reconciliation & Settlement', icon: Coins, action: () => { setCurrentTab('reconciliation'); setViewMode('console'); onClose(); } },
      { id: 'nav-pricing', category: catNav, label: isZh ? '模型费率与阶梯定价' : 'Model Pricing & Rates', icon: DollarSign, action: () => { setCurrentTab('pricing'); setViewMode('console'); onClose(); } },
      { id: 'nav-users', category: catNav, label: isZh ? '组织架构与用户管理' : 'User Management', icon: ShieldCheck, action: () => { setCurrentTab('users'); setViewMode('console'); onClose(); } },
      { id: 'nav-skills', category: catNav, label: isZh ? 'Skill Hub 智能体技能中心' : 'Skill Hub', icon: Sparkles, action: () => { setCurrentTab('skills'); setViewMode('console'); onClose(); } },
      { id: 'nav-mcp', category: catNav, label: isZh ? 'MCP 协议广场与连接中心' : 'MCP Plaza & Connectors', icon: Cpu, action: () => { setCurrentTab('mcp'); setViewMode('console'); onClose(); } },
    ] : []),
    { id: 'nav-keys', category: catNav, label: isZh ? 'API 访问密钥管理' : 'API Key Management', icon: Key, action: () => { setCurrentTab('keys'); setViewMode('console'); onClose(); } },
    { id: 'nav-wallet', category: catNav, label: isAdmin ? (isZh ? '卡密生成与充值中心' : 'Card Codes & Billing') : (isZh ? '我的钱包与充值卡密' : 'Wallet & Top-up'), icon: Wallet, action: () => { setCurrentTab('wallet'); setViewMode('console'); onClose(); } },
    { id: 'nav-logs', category: catNav, label: isZh ? '实时调用日志与对话审计' : 'Audit Logs & Traces', icon: History, action: () => { setCurrentTab('logs'); setViewMode('console'); onClose(); } },
    { id: 'nav-playground', category: catNav, label: isZh ? '多模态调试演练工作台' : 'Multimodal Playground', icon: Terminal, action: () => { setCurrentTab('playground'); setViewMode('console'); onClose(); } },
  ];

  const quickActionItems = [
    { id: 'act-new-key', category: catAction, label: isZh ? '新建 API 访问密钥' : 'Create API Key', icon: Plus, action: () => { onClose(); setCurrentTab('keys'); onOpenNewKey && onOpenNewKey(); } },
    ...(isAdmin ? [
      { id: 'act-new-channel', category: catAction, label: isZh ? '接入新上游模型服务商' : 'Connect Upstream Provider', icon: Server, action: () => { onClose(); setCurrentTab('channels'); onOpenNewChannel && onOpenNewChannel(); } },
    ] : []),
    { id: 'act-status', category: catAction, label: isZh ? '查看公开服务健康状态页' : 'Public Status Page', icon: Zap, action: () => { setViewMode('status'); onClose(); } },
    { id: 'act-docs', category: catAction, label: isZh ? '打开交互式开发文档' : 'Interactive API Docs', icon: ExternalLink, action: () => { setViewMode('landing'); onClose(); setTimeout(() => document.getElementById('docs')?.scrollIntoView({ behavior: 'smooth' }), 100); } },
    { id: 'act-lang', category: catAction, label: isZh ? `切换界面语言 (当前: 简体中文)` : `Switch Interface Language (Current: English)`, icon: Globe, action: () => { toggleLang && toggleLang(); onClose(); } },
    { id: 'act-logout', category: catAction, label: isZh ? '安全退出当前登录账号' : 'Log Out Current Account', icon: LogOut, action: () => { onClose(); onLogout && onLogout(); } },
  ];

  const modelItems = (models || []).slice(0, 15).map((m, idx) => ({
    id: `model-${idx}-${m}`,
    category: catModels,
    label: m,
    icon: Sparkles,
    action: () => {
      setCurrentTab('playground');
      setViewMode('console');
      onClose();
      if (showToast) showToast(isZh ? `已快速定位模型: ${m}` : `Selected model: ${m}`, 'info');
    }
  }));

  const allItems = [...navItems, ...quickActionItems, ...modelItems];

  const rawFiltered = query.trim()
    ? allItems.filter(item =>
        item.label.toLowerCase().includes(query.trim().toLowerCase()) ||
        item.category.toLowerCase().includes(query.trim().toLowerCase())
      )
    : allItems;

  const aiCopilotItem = query.trim() ? [{
    id: 'ai-copilot-query',
    category: isZh ? 'AI 智能副驾驶' : 'AI Copilot',
    label: isZh ? `询问 AI 副驾驶: "${query.trim()}"` : `Ask AI Copilot: "${query.trim()}"`,
    icon: Sparkles,
    isAi: true,
    action: () => {
      onClose();
      if (onOpenCopilotWithQuery) {
        onOpenCopilotWithQuery(query.trim());
      }
    }
  }] : [];

  const filteredItems = query.trim()
    ? [...aiCopilotItem, ...rawFiltered]
    : allItems;

  useEffect(() => {
    setSelectedIndex(0);
  }, [query]);

  // Handle keyboard navigation
  const handleKeyDown = (e) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setSelectedIndex((prev) => (prev + 1) % (filteredItems.length || 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setSelectedIndex((prev) => (prev - 1 + (filteredItems.length || 1)) % (filteredItems.length || 1));
    } else if (e.key === 'Enter') {
      e.preventDefault();
      if (filteredItems[selectedIndex]) {
        filteredItems[selectedIndex].action();
      }
    } else if (e.key === 'Escape') {
      e.preventDefault();
      onClose();
    }
  };

  if (!isOpen) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center pt-20 px-4 bg-slate-900/50 backdrop-blur-sm animate-in fade-in duration-150"
      onClick={onClose}
    >
      <div
        className="w-full max-w-xl bg-white dark:bg-[#111726] rounded-2xl border border-slate-200/90 dark:border-slate-800 shadow-2xl overflow-hidden flex flex-col max-h-[75vh] animate-in zoom-in-95 duration-150"
        onClick={(e) => e.stopPropagation()}
        onKeyDown={handleKeyDown}
      >
        {/* Search Input Bar */}
        <div className="flex items-center px-5 py-4 border-b border-slate-200/80 dark:border-slate-800 relative">
          <Search className="w-5 h-5 text-slate-400 mr-3 shrink-0" />
          <input
            ref={inputRef}
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={isZh ? '搜索功能模块、操作指令或模型 (支持 ↑ ↓ 导航)...' : 'Search modules, commands, or models (↑ ↓ to navigate)...'}
            className="w-full text-sm bg-transparent text-slate-900 dark:text-slate-100 placeholder-slate-400 dark:placeholder-slate-500 focus:outline-none"
          />
          {query && (
            <button
              onClick={() => setQuery('')}
              className="text-slate-400 hover:text-slate-600 dark:hover:text-slate-300 p-1 mr-2"
            >
              <X className="w-4 h-4" />
            </button>
          )}
          <kbd className="hidden sm:inline-flex items-center px-2 py-0.5 text-xs font-mono font-semibold text-slate-500 bg-slate-100 dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-lg">
            ESC
          </kbd>
        </div>

        {/* Results List */}
        <div ref={listRef} className="overflow-y-auto p-2 space-y-1 flex-1">
          {filteredItems.length === 0 ? (
            <div className="py-10 text-center text-xs text-slate-500 dark:text-slate-400 flex flex-col items-center">
              <p className="mb-3">{isZh ? `未找到与 "${query}" 相关的系统导航` : `No system navigation matching "${query}"`}</p>
              {query && onOpenCopilotWithQuery && (
                <button
                  onClick={() => {
                    onClose();
                    onOpenCopilotWithQuery(query.trim());
                  }}
                  className="ui-btn-primary"
                >
                  <Sparkles className="w-3.5 h-3.5 text-amber-300" />
                  <span>{isZh ? `向 AI 智能副驾驶询问: "${query}"` : `Ask AI Copilot: "${query}"`}</span>
                </button>
              )}
            </div>
          ) : (
            filteredItems.map((item, idx) => {
              const Icon = item.icon;
              const isSelected = idx === selectedIndex;
              return (
                <div
                  key={item.id}
                  onClick={item.action}
                  onMouseEnter={() => setSelectedIndex(idx)}
                  className={`flex items-center justify-between px-3.5 py-2 rounded-lg cursor-pointer text-xs transition ${
                    item.isAi
                      ? (isSelected
                          ? 'bg-purple-50 dark:bg-purple-950/60 text-purple-900 dark:text-purple-200 font-semibold border border-purple-200/60 dark:border-purple-800'
                          : 'bg-indigo-50/50 dark:bg-indigo-950/30 text-indigo-900 dark:text-indigo-200 hover:bg-indigo-50 dark:hover:bg-indigo-950/50 border border-indigo-100/50 dark:border-indigo-900/40')
                      : (isSelected
                          ? 'bg-indigo-50 dark:bg-indigo-950/50 text-indigo-900 dark:text-indigo-200 font-semibold'
                          : 'text-slate-700 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-800/60')
                  }`}
                >
                  <div className="flex items-center space-x-3 min-w-0">
                    <div
                      className={`w-7 h-7 rounded-lg flex items-center justify-center shrink-0 border ${
                        item.isAi
                          ? 'bg-slate-900 dark:bg-indigo-600 text-white border-slate-900 shadow-sm'
                          : (isSelected
                              ? 'bg-slate-900 dark:bg-indigo-600 text-white border-slate-900 shadow-sm'
                              : 'bg-slate-100 dark:bg-slate-800 text-slate-500 border-slate-200/80 dark:border-slate-700')
                      }`}
                    >
                      <Icon className="w-3.5 h-3.5" />
                    </div>
                    <span className="truncate">{item.label}</span>
                  </div>

                  <div className="flex items-center space-x-2 shrink-0 ml-2">
                    <span className={`text-xs font-medium px-2 py-0.5 rounded-md ${
                      item.isAi
                        ? 'bg-purple-100 dark:bg-purple-900/60 text-purple-700 dark:text-purple-300 font-bold'
                        : 'text-slate-500 dark:text-slate-400 bg-slate-100 dark:bg-slate-800/80'
                    }`}>
                      {item.category}
                    </span>
                    {isSelected && (
                      <CornerDownLeft className="w-3.5 h-3.5 text-indigo-500" />
                    )}
                  </div>
                </div>
              );
            })
          )}
        </div>

        {/* Footer shortcuts info */}
        <div className="px-5 py-2.5 bg-slate-50 dark:bg-slate-900/60 border-t border-slate-200/70 dark:border-slate-800 text-xs text-slate-500 dark:text-slate-400 flex items-center justify-between">
          <div className="flex items-center space-x-3">
            <span>{isZh ? '↑ ↓ 移动光标' : '↑ ↓ Navigate'}</span>
            <span>{isZh ? '↵ 确认跳转' : '↵ Select'}</span>
            <span>{isZh ? 'ESC 关闭' : 'ESC Close'}</span>
          </div>
          <span className="font-mono text-slate-500 dark:text-slate-400">{isZh ? 'Airoute · 快捷控制中心' : 'Airoute · Command Center'}</span>
        </div>
      </div>
    </div>
  );
}
