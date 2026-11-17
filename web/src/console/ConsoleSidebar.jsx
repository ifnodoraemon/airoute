import { useConsoleContext } from './ConsoleContext';
import { CONSOLE_NAV as SIDEBAR_ITEMS } from '../lib/consoleNavigation';
import { Zap, BookOpen, PanelLeftClose, PanelLeft } from 'lucide-react';
import { ADMIN_NAV_GROUPS, USER_NAV_GROUPS, navGroupLabel } from '../lib/navGroups';

export default function ConsoleSidebar() {
  const {
  adminUser, channels, collapseSidebar, currentTab, expandSidebar, isZh, keys, lang, logs, mobileMenuOpen,
  models, notificationCounts, setCurrentTab, setViewMode, sidebarCollapsed, t
} = useConsoleContext();
  const trippedCount = (channels || []).filter(ch => ['OPEN', 'HALF-OPEN'].includes(String(ch.breaker_status || '').toUpperCase())).length;
  const disabledCount = (channels || []).filter(ch => ch.status && ch.status !== 'active').length;
  const healthDot = trippedCount > 0 ? 'bg-rose-500' : disabledCount > 0 ? 'bg-amber-500' : 'bg-emerald-500';
  const healthLabel = trippedCount > 0
    ? (isZh ? `${trippedCount} 个熔断` : `${trippedCount} tripped`)
    : disabledCount > 0
      ? (isZh ? `${disabledCount} 个停用` : `${disabledCount} disabled`)
      : t.statusOperationalBadge;
  return (
    <aside
      className={`bg-surface border-r border-light flex flex-col shrink-0 transition-all duration-200 z-30 ${
        sidebarCollapsed ? 'w-16' : 'w-56'
      } ${
        mobileMenuOpen ? 'fixed inset-y-0 left-0 w-56 shadow-2xl flex' : 'hidden md:flex'
      }`}
    >
      <div className="h-16 px-4 border-b border-light flex items-center justify-between">
        <div
          onClick={() => setViewMode('landing')}
          className={`flex items-center ${sidebarCollapsed ? 'justify-center w-full' : 'space-x-2.5'} cursor-pointer group transition hover:opacity-90`}
          title={lang === 'zh' ? '点击返回门户首页' : 'Return to product portal'}
        >
          <div className="w-8 h-8 rounded-lg bg-brand flex items-center justify-center text-white shrink-0 shadow-xs">
            <Zap className="w-4 h-4 fill-white text-white" />
          </div>
          {!sidebarCollapsed && (
            <div className="min-w-0">
              <h1 className="font-semibold text-sm text-primary tracking-tight leading-none">
                Airoute
              </h1>
              <span className="text-[10px] text-secondary font-mono tracking-tight block mt-1">
                AI Gateway
              </span>
            </div>
          )}
        </div>

        {!sidebarCollapsed && (
          <button type="button"
            onClick={collapseSidebar}
            className="p-1 rounded-md text-secondary hover:text-primary hover:bg-subtle transition-colors cursor-pointer"
            title={isZh ? '收起侧边栏' : 'Collapse sidebar'}
          >
            <PanelLeftClose className="w-4 h-4" />
          </button>
        )}
      </div>

      {sidebarCollapsed && (
        <div className="p-2 border-b border-light flex justify-center">
          <button type="button"
            onClick={expandSidebar}
            className="p-1.5 rounded-md text-secondary hover:text-primary hover:bg-subtle transition-colors cursor-pointer"
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
            ? 'bg-brand-selected text-brand-active font-semibold'
            : 'text-secondary hover:bg-subtle hover:text-primary'
        }`;
        const navIconColor = (tab) => (currentTab === tab ? 'text-brand-active' : 'text-secondary');

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
                    <span className="px-2.5 text-[10px] font-semibold tracking-wider text-secondary uppercase block mb-1">
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
                        <button type="button"
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
                            <span className={`text-xs font-mono ${currentTab === tab ? 'text-brand-active' : 'text-secondary'}`}>
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

            <div className="pt-2 border-t border-light">
              <button type="button"
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
      <div className="p-3 border-t border-light text-xs">
        <button
          type="button"
          onClick={() => setViewMode('status')}
          className={`w-full flex items-center ${sidebarCollapsed ? 'justify-center' : 'space-x-2'} text-secondary hover:text-primary transition-colors cursor-pointer`}
          title={isZh ? '查看公开服务状态' : 'Public Status'}
        >
          <span className={`w-2 h-2 rounded-full ${healthDot} shrink-0`}></span>
          {!sidebarCollapsed && <span className="text-[11px] font-mono">{healthLabel}</span>}
        </button>
      </div>
    </aside>
  );
}
