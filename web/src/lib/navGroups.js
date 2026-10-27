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
//   billing      money in every form: prices, supplier reconciliation,
//                cost & margin analysis, wallet credits and their ledger
//   identity     who may call the gateway and within which limits
//                (credentials, accounts, cost centres, budgets)
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
    // Money in every form: list prices, what suppliers charge, what the margin
    // is, and the wallet credits plus the ledger that records their movement.
    tabs: ['pricing', 'reconciliation', 'costs', 'wallet', 'ledger'],
  },
  {
    id: 'identity',
    zh: '身份与治理',
    en: 'Identity & Access',
    // Who may call the gateway (keys, users, organisations) and who may spend
    // how much (budgets).
    tabs: ['keys', 'users', 'orgs', 'budgets'],
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
