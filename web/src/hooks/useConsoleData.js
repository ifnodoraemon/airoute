// Owns the console's tenant-scoped data and the loader that refreshes it.
//
// Extracted from App.jsx, where eight state slices and a seventy-line loader sat
// in the middle of a 2800-line component. Two things motivated the move: the
// loader's role gating had already caused a production incident (a permanent 403
// storm for regular users), and the reset that is supposed to clear tenant data on
// logout silently skipped three of the slices it owned.
//
// The state lives here rather than in App.jsx so the loader can be exercised
// against a stubbed fetch without rendering the whole console.

import { useCallback, useEffect, useRef, useState } from 'react';
import { apiFetch } from '../api/client';
import { getStoredUser } from '../api/session';
import { inferClientModality } from '../lib/modelModality';

export function useConsoleData({ adminUser }) {
  const [channels, setChannels] = useState([]);
  const [keys, setKeys] = useState([]);
  const [stats, setStats] = useState({});
  const [prices, setPrices] = useState([]);
  const [pricingGroups, setPricingGroups] = useState(['default']);
  const [groupMultipliers, setGroupMultipliers] = useState({ default: 1.0, vip: 0.8, enterprise: 1.5 });
  const [modelRoutes, setModelRoutes] = useState([]);
  const [models, setModels] = useState([]);
  const [playModel, setPlayModel] = useState('');

  // refresh can run in the same tick as the session state update, before the new
  // adminUser is visible here — which is why the loader also falls back to the
  // stored session below.
  const adminUserRef = useRef(adminUser);
  useEffect(() => {
    adminUserRef.current = adminUser;
  }, [adminUser]);

  // The session generation. Clearing state is not enough on its own: a request
  // already in flight still resolves, and without this it would write the previous
  // account's channels, keys and stats straight back into the cleared state — which
  // is exactly the cross-account leak the reset exists to prevent. Every loader
  // captures the generation it started in and discards its result if reset ran
  // while it was in the air.
  const generationRef = useRef(0);

  // Clear every piece of tenant-scoped data so a following session (or React
  // DevTools) cannot observe the previous account's channels, keys, stats or
  // pricing.
  //
  // stats, prices and pricingGroups used to be left behind: the comment above the
  // original promised it cleared "every piece of tenant-scoped data", but three of
  // the slices it owned survived a logout until the next refresh happened to
  // overwrite them.
  const reset = useCallback(() => {
    // Invalidate anything already in flight before clearing.
    generationRef.current += 1;
    setChannels([]);
    setKeys([]);
    setStats({});
    setPrices([]);
    setPricingGroups(['default']);
    setGroupMultipliers({ default: 1.0, vip: 0.8, enterprise: 1.5 });
    setModelRoutes([]);
    setModels([]);
    setPlayModel('');
  }, []);

  // Load backend data.
  // Returns true when every endpoint answered, false when something failed
  // (network error, unparseable body, or a 5xx). Callers such as the Dashboard
  // refresh button use this to report the real outcome instead of assuming
  // success. Client-side permission errors (401/403) are not treated as a
  // failure so a limited account does not see spurious "refresh failed" toasts.
  const refresh = useCallback(async (overrideToken) => {
    const opts = overrideToken ? { token: overrideToken } : {};
    // The generation this load belongs to. If reset runs while it is in flight the
    // result is stale and must be dropped rather than written into the cleared
    // state.
    const generation = generationRef.current;
    let failed = false;
    const getJSON = (url) =>
      apiFetch(url, opts)
        .then((r) => {
          if (r.status >= 500) failed = true;
          return r.json();
        })
        .catch(() => {
          failed = true;
          return { code: 1 };
        });

    try {
      // /admin/channels and /admin/models/routes are super-admin only. Calling
      // them from a regular user's 8s poll produced a permanent 403 storm, so
      // only request them when the session actually holds that role. The role
      // is read from storage as a fallback because refresh can run in the same
      // tick as the user state update, before that value is visible here.
      const currentUser = adminUserRef.current;
      const role = currentUser?.role || getStoredUser()?.role;
      const isAdminSession = role === 'admin';
      const skipped = Promise.resolve({ code: 0, data: [] });
      const [chRes, keyRes, statsRes, mRes, routesRes, pricingRes] = await Promise.all([
        isAdminSession ? getJSON('/api/v1/admin/channels') : skipped,
        getJSON('/api/v1/admin/keys'),
        getJSON('/api/v1/admin/stats/overview'),
        getJSON('/api/v1/admin/models'),
        isAdminSession ? getJSON('/api/v1/admin/models/routes') : skipped,
        getJSON('/api/v1/admin/pricing'),
      ]);

      if (generation !== generationRef.current) {
        // The session was cleared (or replaced) while these requests were in the
        // air. Applying them would put the previous account's data back.
        return false;
      }

      if (chRes.code === 0) setChannels(chRes.data || []);
      if (keyRes.code === 0) setKeys(keyRes.data || []);
      if (statsRes.code === 0) setStats(statsRes.data || {});
      if (pricingRes.code === 0 && Array.isArray(pricingRes.data)) {
        setPrices(pricingRes.data);
        const grpSet = new Set(['default']);
        if (pricingRes.group_multipliers && typeof pricingRes.group_multipliers === 'object') {
          setGroupMultipliers(pricingRes.group_multipliers);
          Object.keys(pricingRes.group_multipliers).forEach((g) => {
            if (g) grpSet.add(g.trim().toLowerCase());
          });
        }
        pricingRes.data.forEach((p) => {
          if (p.group_name) grpSet.add(p.group_name.trim().toLowerCase());
        });
        if (currentUser?.group_name) {
          grpSet.add(currentUser.group_name.trim().toLowerCase());
        }
        setPricingGroups(Array.from(grpSet));
      }
      if (routesRes.code === 0) setModelRoutes(routesRes.data || []);
      if (mRes.code === 0 && Array.isArray(mRes.data)) {
        const allM = mRes.data;
        const routes = routesRes.data || [];
        setModels(allM);

        const getInferred = (mod) => allM.filter((m) => inferClientModality(m) === mod);
        const getModModels = (mod) => {
          const matched = routes.filter((r) => r.modality === mod).map((r) => r.model);
          if (matched.length > 0) return matched;
          return getInferred(mod);
        };

        const effChat = getModModels('chat');
        setPlayModel((prev) => ((prev && effChat.includes(prev)) ? prev : (effChat[0] || '')));
      }
    } catch (e) {
      failed = true;
      console.error('Fetch data failed:', e);
    }
    return !failed;
  }, []);

  return {
    channels,
    keys,
    stats,
    prices,
    pricingGroups,
    groupMultipliers,
    setGroupMultipliers,
    modelRoutes,
    models,
    playModel,
    setPlayModel,
    refresh,
    reset,
  };
}
