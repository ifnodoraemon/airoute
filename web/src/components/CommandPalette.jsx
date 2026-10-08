import React, { useState, useEffect, useRef } from 'react';
import {
  Search,
  ArrowRight,
  BarChart3,
  History,
  Cpu,
  Server,
  DollarSign,
  Key,
  ShieldCheck,
  Wallet,
  Terminal,
  Bot,
  Plus,
  RefreshCw,
  Globe,
  LogOut,
  ExternalLink,
  Zap,
  CornerDownLeft,
  X,
  Sparkles
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
  showToast
}) {
  const [query, setQuery] = useState('');
  const [selectedIndex, setSelectedIndex] = useState(0);
  const inputRef = useRef(null);
  const listRef = useRef(null);

  const isAdmin = adminUser?.role === 'admin';

  useEffect(() => {
    if (isOpen) {
      setQuery('');
      setSelectedIndex(0);
      setTimeout(() => inputRef.current?.focus(), 50);
    }
  }, [isOpen]);

  // Build searchable items
  const navItems = [
    ...(isAdmin ? [
      { id: 'nav-dashboard', category: '页面导航', label: '用量与业务概览大盘', icon: BarChart3, action: () => { setCurrentTab('dashboard'); setViewMode('console'); onClose(); } },
      { id: 'nav-models', category: '页面导航', label: '模型路由与拓扑', icon: Cpu, action: () => { setCurrentTab('models'); setViewMode('console'); onClose(); } },
      { id: 'nav-channels', category: '页面导航', label: '上游服务商接入与体检', icon: Server, action: () => { setCurrentTab('channels'); setViewMode('console'); onClose(); } },
      { id: 'nav-pricing', category: '页面导航', label: '模型费率与阶梯定价', icon: DollarSign, action: () => { setCurrentTab('pricing'); setViewMode('console'); onClose(); } },
      { id: 'nav-users', category: '页面导航', label: '组织架构与用户管理', icon: ShieldCheck, action: () => { setCurrentTab('users'); setViewMode('console'); onClose(); } },
      { id: 'nav-skills', category: '页面导航', label: 'Skill Hub 智能体技能中心', icon: Sparkles, action: () => { setCurrentTab('skills'); setViewMode('console'); onClose(); } },
      { id: 'nav-mcp', category: '页面导航', label: 'MCP 协议广场与连接中心', icon: Cpu, action: () => { setCurrentTab('mcp'); setViewMode('console'); onClose(); } },
    ] : []),
    { id: 'nav-keys', category: '页面导航', label: 'API 访问密钥管理', icon: Key, action: () => { setCurrentTab('keys'); setViewMode('console'); onClose(); } },
    { id: 'nav-wallet', category: '页面导航', label: isAdmin ? '卡密生成与充值中心' : '我的钱包与充值卡密', icon: Wallet, action: () => { setCurrentTab('wallet'); setViewMode('console'); onClose(); } },
    { id: 'nav-logs', category: '页面导航', label: '实时调用日志与对话审计', icon: History, action: () => { setCurrentTab('logs'); setViewMode('console'); onClose(); } },
    { id: 'nav-playground', category: '页面导航', label: '多模态调试演练工作台', icon: Terminal, action: () => { setCurrentTab('playground'); setViewMode('console'); onClose(); } },
  ];

  const quickActionItems = [
    { id: 'act-new-key', category: '快捷操作', label: '新建 API 访问密钥', icon: Plus, action: () => { onClose(); setCurrentTab('keys'); onOpenNewKey && onOpenNewKey(); } },
    ...(isAdmin ? [
      { id: 'act-new-channel', category: '快捷操作', label: '接入新上游模型服务商', icon: Server, action: () => { onClose(); setCurrentTab('channels'); onOpenNewChannel && onOpenNewChannel(); } },
    ] : []),
    { id: 'act-status', category: '快捷操作', label: '查看公开服务健康状态页', icon: Zap, action: () => { setViewMode('status'); onClose(); } },
    { id: 'act-docs', category: '快捷操作', label: '打开交互式开发文档', icon: ExternalLink, action: () => { setViewMode('landing'); onClose(); setTimeout(() => document.getElementById('docs')?.scrollIntoView({ behavior: 'smooth' }), 100); } },
    { id: 'act-lang', category: '快捷操作', label: `切换界面语言 (当前: ${lang === 'zh' ? '简体中文' : 'English'})`, icon: Globe, action: () => { toggleLang && toggleLang(); onClose(); } },
    { id: 'act-logout', category: '快捷操作', label: '安全退出当前登录账号', icon: LogOut, action: () => { onClose(); onLogout && onLogout(); } },
  ];

  const modelItems = (models || []).slice(0, 15).map((m, idx) => ({
    id: `model-${idx}-${m}`,
    category: '挂载模型',
    label: m,
    icon: Sparkles,
    action: () => {
      setCurrentTab('playground');
      setViewMode('console');
      onClose();
      if (showToast) showToast(`已快速定位模型: ${m}`, 'info');
    }
  }));

  const allItems = [...navItems, ...quickActionItems, ...modelItems];

  const filteredItems = query.trim()
    ? allItems.filter(item =>
        item.label.toLowerCase().includes(query.trim().toLowerCase()) ||
        item.category.toLowerCase().includes(query.trim().toLowerCase())
      )
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
        className="w-full max-w-xl bg-white dark:bg-[#111726] rounded-3xl border border-slate-200/90 dark:border-slate-800 shadow-2xl overflow-hidden flex flex-col max-h-[75vh] animate-in zoom-in-95 duration-150"
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
            placeholder="搜索功能模块、操作指令或模型 (支持 ↑ ↓ 导航)..."
            className="w-full text-sm bg-transparent text-slate-900 dark:text-slate-100 placeholder-slate-400 focus:outline-none"
          />
          {query && (
            <button
              onClick={() => setQuery('')}
              className="text-slate-400 hover:text-slate-600 p-1 mr-2"
            >
              <X className="w-4 h-4" />
            </button>
          )}
          <kbd className="hidden sm:inline-flex items-center px-2 py-0.5 text-[10px] font-mono font-semibold text-slate-500 bg-slate-100 dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-lg">
            ESC
          </kbd>
        </div>

        {/* Results List */}
        <div ref={listRef} className="overflow-y-auto p-2 space-y-1 flex-1">
          {filteredItems.length === 0 ? (
            <div className="py-12 text-center text-slate-400 text-xs">
              未找到与 &quot;{query}&quot; 相关的导航或功能
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
                  className={`flex items-center justify-between px-3.5 py-2.5 rounded-2xl cursor-pointer text-xs transition ${
                    isSelected
                      ? 'bg-indigo-50 dark:bg-indigo-950/50 text-indigo-900 dark:text-indigo-200 font-semibold'
                      : 'text-slate-700 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-800/60'
                  }`}
                >
                  <div className="flex items-center space-x-3 min-w-0">
                    <div
                      className={`w-7 h-7 rounded-xl flex items-center justify-center shrink-0 border ${
                        isSelected
                          ? 'bg-indigo-600 text-white border-indigo-600 shadow-xs'
                          : 'bg-slate-100 dark:bg-slate-800 text-slate-500 border-slate-200/80 dark:border-slate-700'
                      }`}
                    >
                      <Icon className="w-3.5 h-3.5" />
                    </div>
                    <span className="truncate">{item.label}</span>
                  </div>

                  <div className="flex items-center space-x-2 shrink-0 ml-2">
                    <span className="text-[10px] font-medium text-slate-400 bg-slate-100 dark:bg-slate-800/80 px-2 py-0.5 rounded-md">
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
        <div className="px-5 py-2.5 bg-slate-50 dark:bg-slate-900/60 border-t border-slate-200/70 dark:border-slate-800 text-[11px] text-slate-400 flex items-center justify-between">
          <div className="flex items-center space-x-3">
            <span>↑ ↓ 移动光标</span>
            <span>↵ 确认跳转</span>
            <span>ESC 关闭</span>
          </div>
          <span className="font-mono text-slate-400">Airoute · 快捷控制中心</span>
        </div>
      </div>
    </div>
  );
}
