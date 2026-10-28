// Console information architecture: how tabs are grouped in the sidebar and in
// the header breadcrumb.
//
// The console used to describe its navigation twice: two hand-written JSX trees
// (one for administrators, one for regular users) plus an independent
// `includes()` chain for the breadcrumb group label. Those two descriptions
// drifted. The admin group 「安全与资产 / Security」 had become a grab-bag that
// mixed credentials (keys), identity (users, orgs), spend governance (budgets)
// and finance (costs, wallet, ledger) — and the breadcrumb sent orgs, budgets,
// costs and ledger to 「演练与生态 / Ecosystem」 instead.
//
// The split below follows actual responsibility, not which team owns the screen:
//
//   monitoring   what is happening right now (overview, call logs)
//   gateway      what the gateway serves and how requests are routed
//   billing      money in every form: our list prices (together with the
//                supplier prices they are compared against), supplier
//                reconciliation, and the wallet credits
//   identity     who may call the gateway (credentials, accounts)
//   ecosystem    interactive exercise and third-party integrations
//
// This module is the single source of truth for BOTH the sidebar and the
// breadcrumb. Tab ids and their individual labels still live in App.jsx; this
// file only decides membership and order, so it stays pure data (no React, no
// JSX, no icon imports) and can be tested without a DOM.

export const ADMIN_NAV_GROUPS = [
  { id: 'monitoring', zh: '监控与分析', en: 'Monitoring', tabs: ['dashboard', 'logs'] },
  { id: 'gateway', zh: '模型网关与调度', en: 'Gateway', tabs: ['models', 'channels'] },
  {
    id: 'billing',
    zh: '计费与财务',
    en: 'Billing & Finance',
    // Money in every form: our selling prices (with the supplier prices they are
    // compared against on the same page), what suppliers charge us in aggregate,
    // and the wallet credits.
    tabs: ['pricing', 'reconciliation', 'wallet'],
  },
  {
    id: 'identity',
    zh: '身份与治理',
    en: 'Identity & Access',
    // Who may call the gateway: API credentials and the accounts behind them.
    tabs: ['keys', 'users'],
  },
  { id: 'ecosystem', zh: '演练与生态', en: 'Ecosystem', tabs: ['playground', 'skills', 'mcp'] },
];

export const USER_NAV_GROUPS = [
  { id: 'account', zh: '我的账户', en: 'My Account', tabs: ['dashboard', 'wallet', 'keys'] },
  { id: 'usage', zh: '使用与计费', en: 'Usage & Billing', tabs: ['playground', 'pricing', 'logs'] },
];

// `docs` is deliberately absent: it is the standalone bottom item in the
// sidebar and belongs to no group for either role.

/**
 * The group a tab belongs to for the given role.
 *
 * @param {string} tab console tab id
 * @param {boolean} isAdmin whether the signed-in user is an administrator
 * @returns {{id: string, zh: string, en: string, tabs: string[]} | null}
 *          the group, or null for `docs` and unknown tabs
 */
export function navGroupForTab(tab, isAdmin) {
  const groups = isAdmin ? ADMIN_NAV_GROUPS : USER_NAV_GROUPS;
  return groups.find((group) => group.tabs.includes(tab)) || null;
}

/**
 * The localized group header. A null group yields an empty string so callers
 * can fall back to a sensible label of their own.
 *
 * @param {{zh: string, en: string} | null | undefined} group
 * @param {boolean} isZh
 * @returns {string}
 */
export function navGroupLabel(group, isZh) {
  if (!group) return '';
  return isZh ? group.zh : group.en;
}

/**
 * Flat list of the tabs the sidebar renders (in group/render order), useful for
 * completeness tests and callers that just need the tab set.
 *
 * @param {boolean} isAdmin
 * @returns {string[]}
 */
export function navTabs(isAdmin) {
  return (isAdmin ? ADMIN_NAV_GROUPS : USER_NAV_GROUPS).flatMap((group) => group.tabs);
}

/**
 * Tabs that belong to no group but are still openable by every role.
 *
 * `docs` renders as the standalone bottom item of the sidebar (App.jsx), so it is
 * deliberately absent from both group configs above while remaining a valid
 * destination.
 */
const UNGROUPED_TABS = ['docs'];

/**
 * The complete set of tabs a role may open, in sidebar order.
 *
 * This is the ONE answer to "which tabs may this role see?". The console's role
 * guard (App.jsx), the deep-link resolver (lib/consoleRoute.js) and the tests all
 * read it instead of keeping their own copies. Three hand-maintained lists existed
 * before and they drifted: the role guard omitted `dashboard` while the sidebar,
 * the route resolver and the view renderer all offered it, so a regular user's
 * "Overview" item silently landed on the wallet page.
 *
 * The first element is also the fallback destination for an unknown or forbidden
 * route, which keeps "where do we send them instead?" derived from the same list.
 *
 * @param {boolean} isAdmin
 * @returns {string[]}
 */
export function allowedTabsFor(isAdmin) {
  return [...navTabs(isAdmin), ...UNGROUPED_TABS];
}
