import { useEffect, useCallback, useRef } from 'react';

export default function useWalletData({
  adminFetch, adminUser, checkoutReturn, clearCheckoutIntent, isAdmin, isZh, onBalanceUpdate, onUserUpdated,
  setCheckoutMessage, setLoading, setRedemptions, setRedemptionsError, setWalletData, setWalletError
}) {
  const onUserUpdatedRef = useRef(onUserUpdated);
  const onBalanceUpdateRef = useRef(onBalanceUpdate);
  useEffect(() => {
    onUserUpdatedRef.current = onUserUpdated;
    onBalanceUpdateRef.current = onBalanceUpdate;
  });

  // Wallet loads are sequenced and cancellable. A slot-filling refresh can be
  // overtaken by an older one (the tab-return gesture used to fire two), and an old
  // snapshot landing last would show a balance that is no longer true — the exact
  // number a user decides a top-up on.
  const walletRequestSeqRef = useRef(0);
  const walletAbortRef = useRef(null);

  const fetchWallet = useCallback(async () => {
    if (!adminFetch) return false;
    const seq = ++walletRequestSeqRef.current;
    walletAbortRef.current?.abort();
    const controller = new AbortController();
    walletAbortRef.current = controller;
    const isCurrent = () => seq === walletRequestSeqRef.current && !controller.signal.aborted;
    setLoading(true);
    try {
      const res = await adminFetch('/api/v1/user/wallet', { signal: controller.signal });
      const data = await res.json();
      if (!isCurrent()) return false;
      if (res.ok && data.code === 0 && data.data) {
        setWalletData((previous) => ({
          ...previous, ...data.data,
          group_name: data.data.group_name || 'default',
          orders: Array.isArray(data.data.orders) ? data.data.orders : [],
        }));
        if (onUserUpdatedRef.current) {
          onUserUpdatedRef.current(data.data);
        }
        if (onBalanceUpdateRef.current && data.data.balance !== undefined) {
          onBalanceUpdateRef.current(data.data.balance);
        }
        setWalletError('');
        return true;
      }
      throw new Error('Wallet unavailable');
    } catch {
      if (!isCurrent()) return false;
      setWalletError(isZh ? '余额和充值记录刷新失败，当前显示可能不是最新数据。请重试。' : 'Could not refresh your wallet. Displayed balances may be outdated. Please retry.');
      return false;
    } finally {
      if (seq === walletRequestSeqRef.current) setLoading(false);
      if (walletAbortRef.current === controller) walletAbortRef.current = null;
    }
  }, [adminFetch, isZh, setLoading, setWalletData, setWalletError]);

  const fetchRedemptions = useCallback(async () => {
    if (!adminFetch || !isAdmin) return;
    try {
      const res = await adminFetch('/api/v1/admin/redemptions');
      const data = await res.json();
      if (res.ok && data.code === 0) {
        setRedemptions(Array.isArray(data.data) ? data.data : []);
        setRedemptionsError('');
      } else {
        throw new Error('Gift cards unavailable');
      }
    } catch {
      setRedemptionsError(isZh ? '兑换卡列表加载失败，请刷新重试。' : 'Could not load gift cards. Please refresh to retry.');
    }
  }, [adminFetch, isAdmin, setRedemptions, setRedemptionsError, isZh]);

  useEffect(() => {
    fetchWallet();
    if (isAdmin) {
      fetchRedemptions();
    }
  }, [fetchWallet, fetchRedemptions, isAdmin]);

  // Coming back to the tab fires BOTH `focus` and `visibilitychange` for one user
  // gesture. Treat that gesture as a single refresh: a short coalescing window keeps
  // the second event from issuing a duplicate request, and the loaders above make
  // any straggler harmless anyway.
  useEffect(() => {
    let lastRefreshAt = 0;
    const GESTURE_WINDOW_MS = 500;
    const refresh = () => {
      if (document.visibilityState === 'hidden') return;
      const now = Date.now();
      if (now - lastRefreshAt < GESTURE_WINDOW_MS) return;
      lastRefreshAt = now;
      fetchWallet();
    };
    window.addEventListener('focus', refresh);
    document.addEventListener('visibilitychange', refresh);
    return () => {
      window.removeEventListener('focus', refresh);
      document.removeEventListener('visibilitychange', refresh);
    };
  }, [fetchWallet]);

  // Cancel anything still in the air when the view goes away.
  useEffect(() => () => {
    walletRequestSeqRef.current += 1;
    walletAbortRef.current?.abort();
    walletAbortRef.current = null;
  }, []);

  useEffect(() => {
    if (!checkoutReturn.state) return;
    if (checkoutReturn.state === 'cancel') {
      setCheckoutMessage(isZh ? '已返回钱包，尚未确认付款。订单状态以支付平台通知为准。' : 'Returned to wallet without payment confirmation. Order status follows the payment provider.');
      return;
    }
    if (checkoutReturn.state !== 'success' || !checkoutReturn.order || !adminFetch) return;
    let cancelled = false;
    let timer;
    let attempts = 0;
    const controller = new AbortController();
    setCheckoutMessage(isZh ? '正在确认支付结果，到账后会自动更新余额…' : 'Confirming payment. Your balance will update when credited…');
    const poll = async () => {
      attempts += 1;
      try {
        const res = await adminFetch(`/api/v1/user/wallet/orders/${encodeURIComponent(checkoutReturn.order)}`, { signal: controller.signal });
        const data = await res.json();
        if (cancelled) return;
        if (res.ok && data.code === 0 && data.data) {
          const status = data.data.status;
          const terminal = ['paid', 'expired', 'failed', 'refunded', 'partially_refunded', 'refund_pending', 'disputed'].includes(status);
          if (status === 'paid') {
            const refreshed = await fetchWallet();
            if (cancelled) return;
            setCheckoutMessage(refreshed
              ? (isZh ? '充值已到账，余额已更新。' : 'Payment credited. Your balance is updated.')
              : (isZh ? '充值已到账，余额暂未刷新，请点击刷新余额。' : 'Payment credited. Refresh your balance to see the updated amount.'));
          } else if (['expired', 'failed'].includes(status)) {
            setCheckoutMessage(isZh ? '本次支付未完成，请查看订单状态后重新发起充值。' : 'Payment was not completed. Check the order status before starting another checkout.');
            await fetchWallet();
          } else if (['refunded', 'partially_refunded', 'refund_pending', 'disputed'].includes(status)) {
            setCheckoutMessage(isZh ? '该订单有退款或争议，余额已按当前可用额度调整。' : 'This order has a refund or dispute. Your available balance has been adjusted.');
            await fetchWallet();
          }
          if (terminal) {
            clearCheckoutIntent(checkoutReturn.order);
            const url = new URL(window.location.href);
            url.searchParams.delete('recharge');
            url.searchParams.delete('order_no');
            window.history.replaceState(window.history.state, '', url);
            return;
          }
        } else if (res.status === 404 || res.status === 401 || res.status === 403) {
          setCheckoutMessage(isZh ? '无法查询该订单，请确认当前登录账号或联系管理员。' : 'Unable to access this order. Check your account or contact support.');
          return;
        }
      } catch {
        // Temporary network errors are retried within the same bounded window.
      }
      if (cancelled) return;
      if (attempts >= 60) {
        setCheckoutMessage(isZh ? '支付结果仍在确认中。请稍后刷新或凭订单号联系管理员，避免重复支付。' : 'Payment is still being confirmed. Refresh later or contact support with the order number before paying again.');
        return;
      }
      timer = setTimeout(poll, 2000);
    };
    poll();
    return () => { cancelled = true; clearTimeout(timer); controller.abort(); };
  }, [checkoutReturn, adminFetch, fetchWallet, isZh, clearCheckoutIntent, setCheckoutMessage]);

  useEffect(() => {
    if (adminUser) {
      setWalletData(prev => ({
        ...prev,
        balance: adminUser.balance !== undefined ? adminUser.balance : prev.balance,
        role: adminUser.role || prev.role,
        status: adminUser.status || prev.status,
        group_name: adminUser.group_name || prev.group_name
      }));
    }
  }, [adminUser, setWalletData]);
  return {
    fetchRedemptions, fetchWallet
  };
}
