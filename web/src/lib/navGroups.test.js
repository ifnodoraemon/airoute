import { describe, it, expect } from 'vitest';
import {
  ADMIN_NAV_GROUPS,
  USER_NAV_GROUPS,
  navGroupForTab,
  navGroupLabel,
  navTabs,
} from './navGroups';

// The console's complete tab vocabulary, written out independently of the
// implementation so an omission or a typo in navGroups.js fails here rather
// than silently dropping a button from the sidebar. `docs` is the standalone
// bottom item and is intentionally not part of any group.
const ADMIN_TABS = [
  'dashboard', 'logs',
  'models', 'channels',
  'pricing', 'reconciliation', 'wallet',
  'keys', 'users',
  'playground', 'skills', 'mcp',
];
const USER_TABS = ['dashboard', 'wallet', 'keys', 'playground', 'pricing', 'logs'];
const DOCS_TAB = 'docs';

function flatten(groups) {
  return groups.flatMap((group) => group.tabs);
}

describe('navGroups', () => {
  it('files every admin tab under exactly one group, with no omissions', () => {
    const grouped = flatten(ADMIN_NAV_GROUPS);

    // No tab twice: a duplicate would render the same button in two groups.
    expect(new Set(grouped).size).toBe(grouped.length);
    // No tab missing and no invented tab: the sidebar must cover exactly the
    // console's tab set.
    expect(new Set(grouped)).toEqual(new Set(ADMIN_TABS));
    for (const tab of ADMIN_TABS) {
      expect(grouped.filter((entry) => entry === tab)).toHaveLength(1);
    }
  });

  it('files every regular-user tab under exactly one group, with no omissions', () => {
    const grouped = flatten(USER_NAV_GROUPS);

    expect(new Set(grouped).size).toBe(grouped.length);
    expect(new Set(grouped)).toEqual(new Set(USER_TABS));
    for (const tab of USER_TABS) {
      expect(grouped.filter((entry) => entry === tab)).toHaveLength(1);
    }
  });

  it('keeps the responsibility split and group order', () => {
    // Responsibilities that used to share the grab-bag 「安全与资产 / Security」
    // now sit in the group that owns them.
    expect(ADMIN_NAV_GROUPS.map(({ id, tabs }) => ({ id, tabs }))).toEqual([
      { id: 'monitoring', tabs: ['dashboard', 'logs'] },
      { id: 'gateway', tabs: ['models', 'channels'] },
      { id: 'billing', tabs: ['pricing', 'reconciliation', 'wallet'] },
      { id: 'identity', tabs: ['keys', 'users'] },
      { id: 'ecosystem', tabs: ['playground', 'skills', 'mcp'] },
    ]);
    expect(USER_NAV_GROUPS.map(({ id, tabs }) => ({ id, tabs }))).toEqual([
      { id: 'account', tabs: ['dashboard', 'wallet', 'keys'] },
      { id: 'usage', tabs: ['playground', 'pricing', 'logs'] },
    ]);
  });

  it('uses unique group ids per role', () => {
    for (const groups of [ADMIN_NAV_GROUPS, USER_NAV_GROUPS]) {
      const ids = groups.map((group) => group.id);
      expect(new Set(ids).size).toBe(ids.length);
    }
  });

  it('keeps docs out of every group', () => {
    for (const groups of [ADMIN_NAV_GROUPS, USER_NAV_GROUPS]) {
      expect(flatten(groups)).not.toContain(DOCS_TAB);
    }
    expect(navGroupForTab(DOCS_TAB, true)).toBeNull();
    expect(navGroupForTab(DOCS_TAB, false)).toBeNull();
  });

  it('resolves each tab to the group it is filed under for that role', () => {
    for (const groups of [ADMIN_NAV_GROUPS, USER_NAV_GROUPS]) {
      const isAdmin = groups === ADMIN_NAV_GROUPS;
      for (const group of groups) {
        for (const tab of group.tabs) {
          expect(navGroupForTab(tab, isAdmin)).toBe(group);
        }
      }
    }

    // The role matters: the same id may live in different groups.
    expect(navGroupForTab('dashboard', true).id).toBe('monitoring');
    expect(navGroupForTab('dashboard', false).id).toBe('account');
    expect(navGroupForTab('wallet', true).id).toBe('billing');
    expect(navGroupForTab('wallet', false).id).toBe('account');
    // Admin-only tabs have no home in the regular-user navigation.
    expect(navGroupForTab('models', false)).toBeNull();
    expect(navGroupForTab('users', false)).toBeNull();
  });

  it('returns null for an unknown tab', () => {
    expect(navGroupForTab('not-a-tab', true)).toBeNull();
    expect(navGroupForTab('', false)).toBeNull();
  });

  it('lists the rendered tabs in group order', () => {
    expect(navTabs(true)).toEqual(ADMIN_TABS);
    expect(navTabs(false)).toEqual(USER_TABS);
  });

  it('provides non-empty labels in both languages', () => {
    for (const groups of [ADMIN_NAV_GROUPS, USER_NAV_GROUPS]) {
      for (const group of groups) {
        expect(group.zh.trim()).not.toBe('');
        expect(group.en.trim()).not.toBe('');
        expect(navGroupLabel(group, true)).toBe(group.zh);
        expect(navGroupLabel(group, false)).toBe(group.en);
      }
    }
  });

  it('guards the label helper against a missing group', () => {
    // The breadcrumb calls this with whatever navGroupForTab returned, which is
    // null for docs and unknown tabs.
    expect(navGroupLabel(null, true)).toBe('');
    expect(navGroupLabel(undefined, false)).toBe('');
  });
});
