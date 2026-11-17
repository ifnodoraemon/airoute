import { Globe, Moon, Server, Sun } from 'lucide-react';

export default function LandingNavbar({ isZh, isLoggedIn, onEnterConsole, onOpenLogin, onViewStatus, scrollToSection, theme, toggleTheme, toggleLang }) {
  const links = [['features', '核心特性', 'Features'], ['multimodal', '全模态矩阵', 'Multimodal'], ['docs', '接入指南', 'Guides'], ['pricing', '模型定价', 'Pricing']];
  return (
    <header className="sticky top-0 z-40 bg-page/90 backdrop-blur-md border-b border-light">
      <div className="max-w-7xl mx-auto px-6 h-16 flex items-center justify-between gap-3">
        <button type="button" onClick={() => window.scrollTo({ top: 0, behavior: 'smooth' })} className="flex items-center gap-3 cursor-pointer">
          <span className="w-8 h-8 rounded-lg bg-brand flex items-center justify-center text-white shadow-xs">⚡</span>
          <span className="hidden lg:inline font-bold text-lg tracking-tight">{isZh ? 'AI 路由器' : 'Airoute'}</span>
        </button>
        <nav className="hidden lg:flex items-center gap-8 text-xs font-medium text-secondary">
          {links.map(([id, zh, en]) => (
            <a key={id} href={`#${id}`} className="hover:text-brand-active transition" onClick={e => { e.preventDefault(); scrollToSection(id); }}>
              {isZh ? zh : en}
            </a>
          ))}
        </nav>
        <div className="flex items-center gap-2">
          {toggleTheme && <button type="button" onClick={toggleTheme} className="ui-icon-btn" aria-label={isZh ? '切换主题' : 'Toggle theme'}>
            {theme === 'dark' ? <Sun className="w-4 h-4" /> : <Moon className="w-4 h-4" />}
          </button>}
          {toggleLang && <button type="button" onClick={toggleLang} className="ui-btn-secondary ui-btn-sm">
            <Globe className="w-3.5 h-3.5 text-brand-active" /><span>{isZh ? 'EN' : '中'}</span>
          </button>}
          <button type="button" onClick={onViewStatus} className="ui-btn-secondary ui-btn-sm hidden sm:inline-flex">
            <span className="w-2 h-2 rounded-full bg-emerald-500" /><span>{isZh ? '服务状态' : 'Status'}</span>
          </button>
          <button type="button" onClick={isLoggedIn ? onEnterConsole : onOpenLogin} className="ui-btn-primary ui-btn-sm">
            <Server className="w-3.5 h-3.5" /><span>{isLoggedIn ? (isZh ? '进入工作台 →' : 'Console →') : (isZh ? '登录 / 注册' : 'Sign In / Register')}</span>
          </button>
        </div>
      </div>
    </header>
  );
}
