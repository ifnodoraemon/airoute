import { useConsoleContext } from './ConsoleContext';
import { CONSOLE_NAV as SIDEBAR_ITEMS } from '../lib/consoleNavigation';
import { Zap, BookOpen, PanelLeftClose, PanelLeft } from 'lucide-react';
import { ADMIN_NAV_GROUPS, USER_NAV_GROUPS, navGroupLabel } from '../lib/navGroups';

export default function ConsoleSidebar() {
  const {
  adminUser, channels, collapseSidebar, currentTab, expandSidebar, isZh, keys, lang, logs, mobileMenuOpen,
  models, notificationCounts, setCurrentTab, setViewMode, sidebarCollapsed, t
} = useConsoleContext();
  return (
    <aside
      className={`bg-white dark:bg-[#191B1F] border-r border-[#E4E6E9] dark:border-[#2C3038] flex flex-col shrink-0 transition-all duration-200 z-30 ${
        sidebarCollapsed ? 'w-16' : 'w-56'
      } ${
        mobileMenuOpen ? 'fixed inset-y-0 left-0 w-56 shadow-2xl flex' : 'hidden md:flex'
      }`}
    >
      <div className="h-16 px-4 border-b border-[#E4E6E9] dark:border-[#2C3038] flex items-center justify-between">
        <div
          onClick={() => setViewMode('landing')}
          className={`flex items-center ${sidebarCollapsed ? 'justify-center w-full' : 'space-x-2.5'} cursor-pointer group transition hover:opacity-90`}
          title={lang === 'zh' ? '点击返回门户首页' : 'Return to product portal'}
        >
          <div className="w-8 h-8 rounded-lg bg-[#355CFF] flex items-center justify-center text-white shrink-0 shadow-xs">
            <Zap className="w-4 h-4 fill-white text-white" />
          </div>
          {!sidebarCollapsed && (
            <div className="min-w-0">
              <h1 className="font-semibold text-sm text-[#202124] dark:text-[#F1F2F4] tracking-tight leading-none">
                Airoute
              </h1>
              <span className="text-[10px] text-[#656B75] dark:text-[#A1A7B0] font-mono tracking-tight block mt-1">
                AI Gateway
              </span>
            </div>
          )}
        </div>

        {!sidebarCollapsed && (
          <button
            onClick={collapseSidebar}
            className="p-1 rounded-md text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] transition-colors cursor-pointer"
            title={isZh ? '收起侧边栏' : 'Collapse sidebar'}
          >
            <PanelLeftClose className="w-4 h-4" />
          </button>
        )}
      </div>

      {sidebarCollapsed && (
        <div className="p-2 border-b border-[#E4E6E9] dark:border-[#2C3038] flex justify-center">
          <button
            onClick={expandSidebar}
            className="p-1.5 rounded-md text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] transition-colors cursor-pointer"
            title={isZh ? '展开侧边栏' : 'Expand sidebar'}
          >
            <PanelLeft className="w-4 h-4" />
          </button>
        </div>
      )}

      {/* Sidebar Nav */}
      {(() => {
        const navBtnClass = (tab) => `w-full flex items-center ${sidebarCollapsed ? 'justify-center px-2' : 'justify-between px-2.5'} py-2 rounded-lg text-xs font-medium transition-colors duration-150 cursor-pointer ${
          currentTab === tab
            ? 'bg-[#EEF2FF] dark:bg-[#242D48] text-[#355CFF] dark:text-[#91A7FF] font-semibold'
            : 'text-[#656B75] dark:text-[#A1A7B0] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] hover:text-[#202124] dark:hover:text-[#F1F2F4]'
        }`;
        const navIconColor = (tab) => (currentTab === tab ? 'text-[#355CFF] dark:text-[#91A7FF]' : 'text-[#656B75] dark:text-[#A1A7B0]');

        // One grouped navigation for both roles. Membership and order come from
        // lib/navGroups.js (the same config the breadcrumb reads); appearance
        // comes from SIDEBAR_ITEMS. The admin and regular-user trees used to be
        // hand-written separately, which is how the breadcrumb drifted.
        const isAdmin = adminUser?.role === 'admin';
        const groups = isAdmin ? ADMIN_NAV_GROUPS : USER_NAV_GROUPS;
        const counts = {
          logs: logs.length,
          models: models.length,
          channels: channels.length,
          keys: keys.length,
          notifications: notificationCounts?.unread_count || 0,
        };

        return (
          <nav className="flex-1 p-2 space-y-4 overflow-y-auto">
            <div className="space-y-4">
              {groups.map((group) => (
                <div key={group.id}>
                  {!sidebarCollapsed && (
                    <span className="px-2.5 text-[10px] font-semibold tracking-wider text-[#656B75] dark:text-[#A1A7B0] uppercase block mb-1">
                      {navGroupLabel(group, isZh)}
                    </span>
                  )}
                  <div className="space-y-0.5">
                    {group.tabs.map((tab) => {
                      const item = SIDEBAR_ITEMS[tab];
                      const Icon = item.icon;
                      const label = item.label(t, isZh, isAdmin);
                      const title = item.title ? item.title(t, isZh, isAdmin) : label;
                      const count = item.count ? item.count(counts, isAdmin) : 0;
                      return (
                        <button
                          key={tab}
                          onClick={() => setCurrentTab(tab)}
                          title={title}
                          className={navBtnClass(tab)}
                        >
                          <div className="flex items-center space-x-2.5">
                            <Icon className={`w-4 h-4 shrink-0 transition-colors duration-150 ${navIconColor(tab)}`} />
                            {!sidebarCollapsed && <span>{label}</span>}
                          </div>
                          {!sidebarCollapsed && count > 0 && (
                            <span className={`text-xs font-mono ${currentTab === tab ? 'text-[#355CFF] dark:text-[#91A7FF]' : 'text-[#656B75] dark:text-[#A1A7B0]'}`}>
                              {count}
                            </span>
                          )}
                        </button>
                      );
                    })}
                  </div>
                </div>
              ))}
            </div>

            <div className="pt-2 border-t border-[#E4E6E9] dark:border-[#2C3038]">
              <button
                onClick={() => setCurrentTab('docs')}
                className={navBtnClass('docs')}
                title={isZh ? '查阅开发文档' : 'Documentation'}
              >
                <div className="flex items-center space-x-2.5">
                  <BookOpen className={`w-4 h-4 shrink-0 transition-colors duration-150 ${navIconColor('docs')}`} />
                  {!sidebarCollapsed && <span>{t.navViewDocs || (isZh ? '开发文档' : 'Docs')}</span>}
                </div>
              </button>
            </div>
          </nav>
        );
      })()}

      {/* Sidebar bottom status */}
      <div className="p-3 border-t border-[#E4E6E9] dark:border-[#2C3038] text-xs">
        <button
          type="button"
          onClick={() => setViewMode('status')}
          className={`w-full flex items-center ${sidebarCollapsed ? 'justify-center' : 'space-x-2'} text-[#656B75] hover:text-[#202124] dark:hover:text-[#F1F2F4] transition-colors cursor-pointer`}
          title={isZh ? '查看公开服务状态' : 'Public Status'}
        >
          <span className="w-2 h-2 rounded-full bg-emerald-500 shrink-0"></span>
          {!sidebarCollapsed && <span className="text-[11px] font-mono">{t.statusOperationalBadge}</span>}
        </button>
      </div>
    </aside>
  );
}
