import { consoleTabTitle } from '../lib/consoleNavigation';
import { useConsoleContext } from './ConsoleContext';
import { Search, Globe, LogOut, User, Sun, Moon, Menu, Megaphone } from 'lucide-react';
import { navGroupForTab, navGroupLabel } from '../lib/navGroups';

import NotificationBell from '../components/NotificationBell';

export default function ConsoleHeader() {
  const {
  adminUser, currentTab, handleLogout, isZh, lang, mobileMenuOpen, notificationCounts, setAccountModalTab,
  setCurrentTab, setEditingAnnouncementId, setMobileMenuOpen, setNotificationKind, setShowAccountModal,
  setShowAnnouncementModal, setShowCommandPalette, t, theme, toggleLang, toggleTheme
} = useConsoleContext();
  return (
    <header className="h-16 bg-[#F7F7F5]/90 dark:bg-[#111214]/90 backdrop-blur-md border-b border-[#E4E6E9] dark:border-[#2C3038] flex items-center justify-between px-4 sm:px-6 lg:px-8 xl:px-10 sticky top-0 z-20">
      <div className="flex items-center space-x-2 sm:space-x-3 min-w-0 flex-1">
        {/* Mobile Hamburger Toggle */}
        <button
          onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
          className="md:hidden p-2 rounded-lg text-[#656B75] hover:text-[#202124] dark:hover:text-[#F1F2F4] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] transition-colors cursor-pointer"
          title={isZh ? '打开导航栏' : 'Toggle navigation menu'}
        >
          <Menu className="w-5 h-5" />
        </button>

        {/* Clean Breadcrumb */}
        <div className="flex items-center sm:space-x-2 text-xs min-w-0">
          <span className="hidden xl:inline text-[#656B75] dark:text-[#A1A7B0] font-normal whitespace-nowrap">
            {/* The group label is derived from the same config as the
                sidebar, so what a tab is filed under can no longer differ
                between the two. `docs` is group-less by design and falls
                back to its own label. */}
            {navGroupLabel(navGroupForTab(currentTab, adminUser?.role === 'admin'), isZh)
              || (isZh ? '文档' : 'Docs')}
          </span>
          <span className="hidden xl:inline text-[#E4E6E9] dark:text-[#2C3038]">/</span>
          <span className="font-semibold text-[#202124] dark:text-[#F1F2F4] truncate whitespace-nowrap">
            {consoleTabTitle(currentTab, t, isZh, adminUser?.role === 'admin')}
          </span>
        </div>
      </div>

      <div className="flex items-center space-x-1.5 sm:space-x-2.5 shrink-0">
        {/* Command Palette Trigger */}
        <button
          onClick={() => setShowCommandPalette(true)}
          className="hidden xl:flex items-center space-x-2 px-3 py-1.5 rounded-lg bg-white dark:bg-[#191B1F] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] border border-[#E4E6E9] dark:border-[#2C3038] text-xs text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4] transition-colors cursor-pointer shadow-2xs"
          title={isZh ? '全局快捷检索与指令面板 (⌘K)' : 'Command Palette (⌘K)'}
        >
          <Search className="w-3.5 h-3.5 text-[#656B75] dark:text-[#A1A7B0]" />
          <span>{isZh ? '快速搜索...' : 'Search...'}</span>
          <kbd className="text-[10px] bg-[#F0F1F2] dark:bg-[#22252B] border border-[#E4E6E9] dark:border-[#2C3038] rounded px-1.5 py-0.5 font-mono text-[#656B75] dark:text-[#A1A7B0]">
            ⌘K
          </kbd>
        </button>

        {/* Theme Toggle Button */}
        <button
          onClick={toggleTheme}
          className="p-2 rounded-lg bg-white dark:bg-[#191B1F] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] text-[#202124] dark:text-[#F1F2F4] transition-colors border border-[#E4E6E9] dark:border-[#2C3038] shadow-2xs cursor-pointer"
          title={theme === 'dark' ? (isZh ? '切换为浅色模式' : 'Switch to Light Mode') : (isZh ? '切换为深色模式' : 'Switch to Dark Mode')}
        >
          {theme === 'dark' ? <Sun className="w-3.5 h-3.5 text-amber-400" /> : <Moon className="w-3.5 h-3.5 text-[#656B75]" />}
        </button>

        {/* Language Switcher Pill */}
        <button
          onClick={toggleLang}
          className="text-xs px-2.5 py-1.5 rounded-lg bg-white dark:bg-[#191B1F] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] text-[#202124] dark:text-[#F1F2F4] font-medium flex items-center space-x-1.5 transition-colors border border-[#E4E6E9] dark:border-[#2C3038] shadow-2xs cursor-pointer"
          title={lang === 'zh' ? 'Switch to English' : '切换为简体中文'}
        >
          <Globe className="w-3.5 h-3.5 text-[#656B75] dark:text-[#A1A7B0]" />
          <span>{t.langToggle}</span>
        </button>

        <NotificationBell counts={notificationCounts} lang={lang} onClick={() => {
          setNotificationKind(notificationCounts?.message_unread_count > 0 ? 'message' : 'announcement');
          setCurrentTab('notifications');
        }} />

        {/* Admin Announcement Trigger */}
        {adminUser?.role === 'admin' && (
          <button
            onClick={() => { setEditingAnnouncementId(null); setShowAnnouncementModal(true); }}
            className="hidden lg:flex items-center space-x-1.5 text-xs text-[#202124] dark:text-[#F1F2F4] font-medium px-2.5 py-1.5 rounded-lg bg-white dark:bg-[#191B1F] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] border border-[#E4E6E9] dark:border-[#2C3038] shadow-2xs transition-colors cursor-pointer"
            title={isZh ? '发布全站公告' : 'Publish announcement'}
          >
            <Megaphone className="w-3.5 h-3.5 text-[#355CFF]" />
            <span>{isZh ? '发布公告' : 'Announcement'}</span>
          </button>
        )}

        {/* Account info & actions */}
        <div className="flex items-center space-x-2 pl-2 border-l border-[#E4E6E9] dark:border-[#2C3038]">
          <button
            onClick={() => { setAccountModalTab('profile'); setShowAccountModal(true); }}
            className="flex items-center space-x-1.5 text-xs text-[#202124] dark:text-[#F1F2F4] font-medium px-2.5 py-1.5 rounded-lg bg-white dark:bg-[#191B1F] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] border border-[#E4E6E9] dark:border-[#2C3038] shadow-2xs transition-colors cursor-pointer"
            title={isZh ? '账号设置' : 'Account Settings'}
          >
            <User className="w-3.5 h-3.5 text-[#656B75] dark:text-[#A1A7B0]" />
            <span className="hidden sm:inline max-w-28 truncate">{adminUser?.username || 'admin'}</span>
          </button>

          <button
            onClick={handleLogout}
            className="p-1.5 rounded-lg hover:bg-rose-50 dark:hover:bg-rose-950/40 text-[#656B75] hover:text-rose-600 dark:hover:text-rose-400 transition-colors cursor-pointer"
            title={isZh ? '退出登录' : 'Logout'}
          >
            <LogOut className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>
    </header>
  );
}
