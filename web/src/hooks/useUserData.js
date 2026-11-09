import React, { useState, useEffect, useCallback } from 'react';

export default function useUserData({
  adminFetch, isZh, setLoading, setUsers, showToast, users
}) {
  const generateSecurePassword = () => {
    const chars = 'abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789!@#$';
    const out = [];
    const rnd = new Uint32Array(12);
    crypto.getRandomValues(rnd);
    for (let i = 0; i < 12; i++) {
      out.push(chars.charAt(rnd[i] % chars.length));
    }
    return out.join('');
  };

  const [pricingGroups, setPricingGroups] = useState([]);

  const fetchUsers = useCallback(async () => {
    if (!adminFetch) return;
    setLoading(true);
    try {
      const res = await adminFetch('/api/v1/admin/users');
      const data = await res.json();
      if (res.ok && data.code === 0) {
        setUsers(data.data || []);
      } else {
        if (showToast) showToast(data.error || (isZh ? '获取用户列表失败' : 'Failed to fetch user list'), 'error');
      }
    } catch (err) {
      if (showToast) showToast((isZh ? '请求异常: ' : 'Request error: ') + err.message, 'error');
    } finally {
      setLoading(false);
    }
  }, [adminFetch, setLoading, setUsers, showToast, isZh]);

  useEffect(() => {
    fetchUsers();
    if (adminFetch) {
      adminFetch('/api/v1/admin/pricing')
        .then(r => r.json())
        .then(d => {
          if (d.code === 0 && Array.isArray(d.data)) {
            const set = new Set();
            d.data.forEach(p => { if (p.group_name) set.add(p.group_name.trim().toLowerCase()); });
            setPricingGroups(Array.from(set));
          }
        })
        .catch(() => {});
    }
  }, [fetchUsers, adminFetch]);

  const allAvailableGroups = React.useMemo(() => {
    const set = new Set(['default']);
    (pricingGroups || []).forEach(g => set.add(g));
    users.forEach(u => { if (u.group_name) set.add(u.group_name.trim().toLowerCase()); });
    return Array.from(set);
  }, [pricingGroups, users]);
  return {
    allAvailableGroups, fetchUsers, generateSecurePassword
  };
}
