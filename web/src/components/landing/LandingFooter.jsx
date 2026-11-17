export default function LandingFooter({ isZh, isLoggedIn, onEnterConsole, onOpenLogin, onViewStatus }) {
  return (
    <footer className="mt-auto border-t border-light bg-surface py-10 px-6 text-xs text-secondary">
      <div className="max-w-7xl mx-auto flex flex-col sm:flex-row items-center justify-between gap-4">
        <div className="flex items-center gap-2">
          <span className="font-bold text-primary text-sm">Airoute</span>
          <span>· © {new Date().getFullYear()} {isZh ? '极简高性能 AI 路由器' : 'Minimalist High-Performance AI Router'}</span>
        </div>
        <div className="flex items-center gap-6">
          <button type="button" onClick={isLoggedIn ? onEnterConsole : onOpenLogin} className="text-brand-active hover:underline font-semibold cursor-pointer">
            {isLoggedIn ? (isZh ? '进入工作台' : 'Enter Console') : (isZh ? '登录 / 注册' : 'Sign In / Register')}
          </button>
          <button type="button" onClick={onViewStatus} className="hover:text-primary cursor-pointer">{isZh ? '服务运行状态 (SLA)' : 'Service Status (SLA)'}</button>
          <a href="https://github.com/ifnodoraemon/airoute" target="_blank" rel="noreferrer" className="hover:text-primary">{isZh ? 'GitHub 源码' : 'GitHub Source'}</a>
        </div>
      </div>
    </footer>
  );
}
