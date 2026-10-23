import { useState, useEffect, useCallback, useRef } from 'react';
import {
  Wallet,
  CreditCard,
  Gift,
  Plus,
  RefreshCw,
  Copy,
  Trash2,
  Zap,
  ArrowRight,
  Clock,
  Sparkles
} from 'lucide-react';
import { copyToClipboard } from '../config';
import useModalA11y from '../hooks/useModalA11y';

export default function WalletManagementView({ adminUser, adminFetch, showToast, onUserUpdated, onBalanceUpdate, lang = 'zh', t }) {
  const isZh = lang === 'zh';
  const isAdmin = adminUser?.role === 'admin';
  const [walletData, setWalletData] = useState({
    balance: adminUser?.balance || 0,
    role: adminUser?.role || 'user',
    status: adminUser?.status || 'active',
    group_name: adminUser?.group_name || 'default',
    stripe_enabled: false,
    orders: []
  });
  const [loading, setLoading] = useState(false);
  const [walletError, setWalletError] = useState('');
  const [redemptionsError, setRedemptionsError] = useState('');

  // Tab: 'wallet' (User wallet & recharge) or 'redemptions' (Admin gift cards)
  const [activeTab, setActiveTab] = useState('wallet');

  // Recharge State
  const [rechargeAmount, setRechargeAmount] = useState(50);
  const [customAmount, setCustomAmount] = useState('');
  const [recharging, setRecharging] = useState(false);
  const [checkoutReturn] = useState(() => {
    const params = new URLSearchParams(window.location.search);
    return { state: params.get('recharge'), order: params.get('order_no') || '' };
  });
  const [checkoutMessage, setCheckoutMessage] = useState('');

  // Redeem State
  const [redeemCode, setRedeemCode] = useState('');
  const [redeeming, setRedeeming] = useState(false);

  // Admin Redemption Cards
  const [redemptions, setRedemptions] = useState([]);
  const [showGenModal, setShowGenModal] = useState(false);
  useModalA11y(showGenModal, () => setShowGenModal(false));
  const [genCount, setGenCount] = useState(5);
  const [genAmount, setGenAmount] = useState(50);
  const [genName, setGenName] = useState(() => (isZh ? '额度兑换卡' : 'Gift Card'));
  const [generating, setGenerating] = useState(false);

  const onUserUpdatedRef = useRef(onUserUpdated);
  const onBalanceUpdateRef = useRef(onBalanceUpdate);
  useEffect(() => {
    onUserUpdatedRef.current = onUserUpdated;
    onBalanceUpdateRef.current = onBalanceUpdate;
  });

  const fetchWallet = useCallback(async () => {
    if (!adminFetch) return;
    setLoading(true);
    try {
      const res = await adminFetch('/api/v1/user/wallet');
      const data = await res.json();
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
      setWalletError(isZh ? '余额和充值记录刷新失败，当前显示可能不是最新数据。请重试。' : 'Could not refresh your wallet. Displayed balances may be outdated. Please retry.');
      return false;
    } finally {
      setLoading(false);
    }
  }, [adminFetch, isZh]);

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
  }, [isAdmin, adminFetch, isZh]);

  useEffect(() => {
    fetchWallet();
    if (isAdmin) {
      fetchRedemptions();
    }
  }, [fetchWallet, fetchRedemptions, isAdmin]);

  useEffect(() => {
    const refresh = () => { if (document.visibilityState !== 'hidden') fetchWallet(); };
    window.addEventListener('focus', refresh);
    document.addEventListener('visibilitychange', refresh);
    return () => {
      window.removeEventListener('focus', refresh);
      document.removeEventListener('visibilitychange', refresh);
    };
  }, [fetchWallet]);

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
          if (status === 'paid') {
            const refreshed = await fetchWallet();
            if (cancelled) return;
            setCheckoutMessage(refreshed
              ? (isZh ? '充值已到账，余额已更新。' : 'Payment credited. Your balance is updated.')
              : (isZh ? '充值已到账，余额暂未刷新，请点击刷新余额。' : 'Payment credited. Refresh your balance to see the updated amount.'));
          } else if (['expired', 'failed', 'cancelled'].includes(status)) {
            setCheckoutMessage(isZh ? '本次支付未完成，请查看订单状态后重新发起充值。' : 'Payment was not completed. Check the order status before starting another checkout.');
            await fetchWallet();
          }
          if (['paid', 'expired', 'failed', 'cancelled'].includes(status)) {
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
  }, [checkoutReturn, adminFetch, fetchWallet, isZh]);

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
  }, [adminUser]);

  const handleRedeem = async (e) => {
    if (e) e.preventDefault();
    if (!redeemCode.trim()) {
      showToast(isZh ? '请输入兑换码' : 'Please enter gift card code', 'warning');
      return;
    }
    setRedeeming(true);
    try {
      const res = await adminFetch('/api/v1/user/wallet/redeem', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ code: redeemCode.trim() })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(data.message || (isZh ? '兑换成功' : 'Redeemed successfully'), 'success');
        setRedeemCode('');
        fetchWallet();
        if (isAdmin) fetchRedemptions();
      } else {
        showToast(data.error || (isZh ? '兑换失败' : 'Redemption failed'), 'error');
      }
    } catch (err) {
      showToast((isZh ? '网络请求异常: ' : 'Network error: ') + err.message, 'error');
    } finally {
      setRedeeming(false);
    }
  };

  const handleRecharge = async () => {
    if (!walletData.stripe_enabled) return;
    const finalAmount = customAmount ? parseFloat(customAmount) : rechargeAmount;
    if (!Number.isFinite(finalAmount) || finalAmount < 0.01 || finalAmount > 999999.99 || Math.abs(finalAmount * 100 - Math.round(finalAmount * 100)) > 0.000001) {
      showToast(isZh ? '请输入 0.01 至 999999.99 元的金额，最多两位小数' : 'Enter an amount from 0.01 to 999999.99 with at most two decimal places', 'warning');
      return;
    }

    setRecharging(true);
    try {
      // Stripe Checkout
      const res = await adminFetch('/api/v1/user/wallet/recharge/stripe/session', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ amount: finalAmount, currency: 'CNY' })
      });
      const data = await res.json();
      if (res.ok && data.code === 0 && data.checkout_url) {
        window.location.href = data.checkout_url;
      } else {
        showToast(data.error || (isZh ? '在线支付通道暂不可用，请使用额度兑换券进行充值' : 'Online payment unavailable, please use gift cards to redeem balance'), 'error');
      }
    } catch (err) {
      showToast((isZh ? '充值请求异常: ' : 'Recharge error: ') + err.message, 'error');
    } finally {
      setRecharging(false);
    }
  };

  const handleGenerateRedemptions = async (e) => {
    if (e) e.preventDefault();
    const amount = Number(genAmount);
    const count = Number(genCount);
    if (!Number.isFinite(amount) || amount < 0.01 || amount > 999999.99 || Math.abs(amount * 100 - Math.round(amount * 100)) > 0.000001) {
      showToast(isZh ? '单卡金额须为 0.01 至 999999.99 元，最多两位小数' : 'Enter an amount from 0.01 to 999999.99 with at most two decimal places', 'warning');
      return;
    }
    if (!Number.isInteger(count) || count < 1 || count > 100) {
      showToast(isZh ? '每批须生成 1 至 100 张兑换卡' : 'Generate between 1 and 100 cards per batch', 'warning');
      return;
    }
    setGenerating(true);
    try {
      const res = await adminFetch('/api/v1/admin/redemptions/generate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          count,
          amount,
          name: genName.trim() || (isZh ? '额度兑换卡' : 'Gift Card')
        })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(data.message || (isZh ? '成功生成兑换卡' : 'Gift cards generated successfully'), 'success');
        setShowGenModal(false);
        fetchRedemptions();
      } else {
        showToast(data.error || (isZh ? '生成失败' : 'Generation failed'), 'error');
      }
    } catch (err) {
      showToast((isZh ? '请求异常: ' : 'Request error: ') + err.message, 'error');
    } finally {
      setGenerating(false);
    }
  };

  const handleDeleteRedemption = async (id) => {
    if (!window.confirm(isZh ? '确认删除该兑换卡记录吗？已删除后将不可兑换。' : 'Confirm delete this gift card? It cannot be redeemed once deleted.')) return;
    try {
      const res = await adminFetch(`/api/v1/admin/redemptions/${id}`, {
        method: 'DELETE'
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(isZh ? '兑换卡已删除' : 'Gift card deleted', 'success');
        fetchRedemptions();
      } else {
        showToast(data.error || (isZh ? '删除失败' : 'Failed to delete card'), 'error');
      }
    } catch (err) {
      showToast((isZh ? '删除请求异常: ' : 'Delete error: ') + err.message, 'error');
    }
  };

  const copyText = async (txt, label = isZh ? '内容' : 'Content') => {
    await copyToClipboard(txt);
    showToast(isZh ? `已复制 ${label} 到剪贴板` : `Copied ${label} to clipboard`, 'success');
  };

  const renderOrderStatus = (status) => {
    const s = String(status || '').toLowerCase();
    if (s === 'paid') {
      return (
        <span className="px-2 py-0.5 rounded-full text-xs font-semibold whitespace-nowrap bg-emerald-500/10 dark:bg-emerald-500/20 text-emerald-700 dark:text-emerald-300 border border-emerald-500/30">
          {isZh ? '已支付入账' : 'Paid'}
        </span>
      );
    }
    if (s === 'cancelled' || s === 'canceled' || s === 'expired') {
      return (
        <span className="px-2 py-0.5 rounded-full text-xs font-semibold whitespace-nowrap bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0] border border-[#E4E6E9] dark:border-[#2C3038]">
          {s === 'expired' ? (isZh ? '已过期' : 'Expired') : (isZh ? '已取消' : 'Cancelled')}
        </span>
      );
    }
    if (s === 'failed') {
      return (
        <span className="px-2 py-0.5 rounded-full text-xs font-semibold whitespace-nowrap bg-rose-500/10 dark:bg-rose-500/20 text-rose-700 dark:text-rose-300 border border-rose-500/30">
          {isZh ? '支付失败' : 'Failed'}
        </span>
      );
    }
    return (
      <span className="px-2 py-0.5 rounded-full text-xs font-semibold whitespace-nowrap bg-amber-500/10 dark:bg-amber-500/20 text-amber-700 dark:text-amber-300 border border-amber-500/30">
        {isZh ? '处理中' : 'Processing'}
      </span>
    );
  };

  return (
    <div className="space-y-6">
      {checkoutMessage && <div role="status" className="ui-panel p-4 text-sm">{checkoutMessage}{checkoutReturn.order && <div className="ui-meta mt-1 font-mono">{checkoutReturn.order}</div>}</div>}
      {walletError && activeTab === 'wallet' && (
        <div role="alert" className="ui-panel p-4 text-sm flex items-center justify-between gap-3">
          <span>{walletError}</span>
          <button onClick={fetchWallet} disabled={loading} className="ui-btn-secondary ui-btn-sm">{isZh ? '重试' : 'Retry'}</button>
        </div>
      )}
      {/* Page Title & Subtitle */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h2 className="ui-page-title flex items-center space-x-2">
            <Wallet className="w-5 h-5 text-[#355CFF] dark:text-[#91A7FF]" />
            <span>{isZh ? '钱包与额度充值' : 'Wallet & Top-up'}</span>
          </h2>
          <p className="ui-page-subtitle">
            {isZh ? '查看可用余额、在线充值账户额度及兑换礼品卡' : 'Manage account balance, online recharge and gift card redemptions'}
          </p>
        </div>

        {/* Header Tabs for Admin */}
        {isAdmin && (
          <div className="flex items-center space-x-2 p-1 bg-[#F0F1F2] dark:bg-[#22252B] rounded-xl border border-[#E4E6E9] dark:border-[#2C3038]">
            <button
              onClick={() => setActiveTab('wallet')}
              className={`px-3 py-1.5 rounded-lg text-xs font-medium transition flex items-center space-x-1.5 cursor-pointer ${
                activeTab === 'wallet'
                  ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-xs'
                  : 'text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'
              }`}
            >
              <Wallet className="w-3.5 h-3.5" />
              <span>{isZh ? '我的钱包' : 'My Wallet'}</span>
            </button>
            <button
              onClick={() => setActiveTab('redemptions')}
              className={`px-3 py-1.5 rounded-lg text-xs font-medium transition flex items-center space-x-1.5 cursor-pointer ${
                activeTab === 'redemptions'
                  ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-xs'
                  : 'text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'
              }`}
            >
              <Gift className="w-3.5 h-3.5" />
              <span>{isZh ? '兑换卡管理 (管理员)' : 'Gift Cards (Admin)'}</span>
            </button>
          </div>
        )}
      </div>

      {/* TAB 1: User Wallet & Recharge */}
      {activeTab === 'wallet' && (
        <div className="space-y-6">
          {/* Summary Stat Cards */}
          <div className="grid grid-cols-1 md:grid-cols-3 gap-5">
            {/* Balance Card */}
            <div className="ui-card p-6 flex flex-col justify-between bg-gradient-to-br from-[#1E293B] to-[#0F172A] dark:from-[#191B1F] dark:to-[#111214] border-[#334155] dark:border-[#2C3038] text-white relative overflow-hidden">
              <div className="absolute -right-8 -top-8 w-32 h-32 bg-[#355CFF]/20 rounded-full blur-2xl pointer-events-none"></div>
              <div>
                <div className="flex items-center justify-between text-slate-300 text-xs font-medium">
                  <div className="flex items-center space-x-2">
                    <Wallet className="w-4 h-4 text-[#91A7FF]" />
                    <span>{isZh ? '钱包当前可用余额' : 'Available Balance'}</span>
                  </div>
                  <button
                    onClick={fetchWallet}
                    className="p-1.5 hover:bg-white/10 rounded-lg text-slate-300 hover:text-white transition cursor-pointer"
                    title={isZh ? '刷新余额' : 'Refresh balance'}
                  >
                    <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
                  </button>
                </div>
                <div className="mt-3 flex items-baseline space-x-2">
                  {isAdmin ? (
                    <span className="text-3xl font-extrabold tracking-tight text-white flex items-center space-x-2">
                      <span>{isZh ? '¥ 无限额度' : '¥ Unlimited'}</span>
                      <span className="text-xs px-2 py-0.5 rounded-full bg-[#355CFF]/30 border border-[#91A7FF]/40 text-[#91A7FF] font-normal">
                        {isZh ? '管理员豁免' : 'Admin Exemption'}
                      </span>
                    </span>
                  ) : (
                    <span className="text-3xl font-bold tracking-tight text-white font-mono tabular-nums">
                      ¥ {Number(walletData.balance || 0).toFixed(2)}
                    </span>
                  )}
                </div>
              </div>

              <div className="mt-5 pt-3 border-t border-slate-700/60 flex items-center justify-between text-xs text-slate-300">
                <span>{isZh ? '用户角色:' : 'Role:'} {isAdmin ? (isZh ? '超级管理员' : 'Administrator') : (isZh ? '普通用户' : 'Regular User')}</span>
              </div>
            </div>

            {/* Pricing Group Tier Card */}
            <div className="ui-card p-6 flex flex-col justify-between">
              <div>
                <div className="text-xs font-semibold text-[#656B75] dark:text-[#A1A7B0] flex items-center space-x-2">
                  <Sparkles className="w-4 h-4 text-amber-500" />
                  <span>{isZh ? '账号保障等级' : 'Account Tier'}</span>
                </div>
                <div className="mt-3">
                  <span className="inline-flex items-center px-3 py-1 rounded-lg text-xs font-semibold bg-amber-500/10 text-amber-700 dark:text-amber-300 border border-amber-500/30">
                    {walletData.group_name === 'default'
                      ? (isZh ? '默认分组 (Standard)' : 'Default (Standard)')
                      : `${walletData.group_name.toUpperCase()} ${isZh ? '专属保障' : 'Tier'}`}
                  </span>
                  <div className="ui-meta mt-2">
                    {isZh ? '创建 API Key 时可使用该等级专属费率或选择标准默认组' : 'API Keys can use this tier rate or standard default group'}
                  </div>
                </div>
              </div>

              <div className="mt-4 pt-3 border-t border-[#E4E6E9] dark:border-[#2C3038] flex items-center justify-between text-xs text-[#656B75] dark:text-[#A1A7B0]">
                <span>{isZh ? '账户状态:' : 'Status:'}</span>
                <span
                  className={`font-medium px-2 py-0.5 rounded-md text-xs ${
                    walletData.status === 'locked'
                      ? 'bg-rose-500/10 text-rose-700 dark:text-rose-300 border border-rose-500/30'
                      : 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 border border-emerald-500/30'
                  }`}
                >
                  {walletData.status === 'locked' ? (isZh ? '已锁定' : 'Locked') : (isZh ? '正常活跃' : 'Active')}
                </span>
              </div>
            </div>

            {/* Quick Redeem Card */}
            <div className="ui-card p-6 flex flex-col justify-between">
              <div>
                <div className="text-xs font-semibold text-[#656B75] dark:text-[#A1A7B0] flex items-center space-x-2">
                  <Gift className="w-4 h-4 text-[#355CFF] dark:text-[#91A7FF]" />
                  <span>{isZh ? '兑换码快速充值' : 'Redeem Gift Card'}</span>
                </div>
                <form onSubmit={handleRedeem} className="mt-3 space-y-2.5">
                  <input
                    type="text"
                    value={redeemCode}
                    onChange={(e) => setRedeemCode(e.target.value)}
                    placeholder={isZh ? '输入卡号 CARD-XXXX-XXXX' : 'Enter code CARD-XXXX-XXXX'}
                    className="ui-field font-mono uppercase"
                  />
                  <button
                    type="submit"
                    disabled={redeeming}
                    className="ui-btn-primary w-full"
                  >
                    {redeeming ? (
                      <RefreshCw className="w-3.5 h-3.5 animate-spin" />
                    ) : (
                      <>
                        <Zap className="w-3.5 h-3.5" />
                        <span>{isZh ? '立即兑换入账' : 'Redeem Now'}</span>
                      </>
                    )}
                  </button>
                </form>
              </div>
            </div>
          </div>

          {/* Recharge Section */}
          <div className="ui-card p-6 space-y-5">
            <div className="flex items-center justify-between border-b border-[#E4E6E9] dark:border-[#2C3038] pb-4">
              <div className="flex items-center space-x-3">
                <div className="w-9 h-9 rounded-lg bg-[#EEF2FF] dark:bg-[#242D48] border border-[#355CFF]/30 dark:border-[#91A7FF]/30 flex items-center justify-center text-[#355CFF] dark:text-[#91A7FF]">
                  <CreditCard className="w-4 h-4" />
                </div>
                <div>
                  <h3 className="font-semibold text-sm text-[#202124] dark:text-[#F1F2F4]">{isZh ? '钱包余额在线充值' : 'Online Balance Top-up'}</h3>
                  <p className="ui-meta">{isZh ? '选择预设面额或输入自定义金额在线充值' : 'Select a preset amount or enter a custom amount'}</p>
                </div>
              </div>
            </div>

            {/* Select Amount */}
            <div className="space-y-3">
              <label className="block text-xs font-semibold text-[#202124] dark:text-[#F1F2F4]">{isZh ? '选择充值金额 (CNY)' : 'Select Recharge Amount (CNY)'}</label>
              <div className="grid grid-cols-2 sm:grid-cols-5 gap-3">
                {[10, 30, 50, 100, 200].map((amt) => (
                  <button
                    key={amt}
                    type="button"
                    onClick={() => {
                      setRechargeAmount(amt);
                      setCustomAmount('');
                    }}
                    className={`py-3 rounded-lg border text-sm font-semibold transition cursor-pointer flex flex-col items-center justify-center ${
                      rechargeAmount === amt && !customAmount
                        ? 'bg-[#EEF2FF] dark:bg-[#242D48] border-[#355CFF] text-[#355CFF] dark:text-[#91A7FF] shadow-xs'
                        : 'bg-white dark:bg-[#191B1F] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] border-[#E4E6E9] dark:border-[#2C3038] text-[#202124] dark:text-[#F1F2F4]'
                    }`}
                  >
                    <span className="font-mono tabular-nums">¥ {amt}</span>
                  </button>
                ))}
              </div>

              <div className="flex items-center space-x-3 pt-1">
                <span className="ui-meta">{isZh ? '或自定义金额:' : 'Or custom amount:'}</span>
                <div className="relative w-44">
                  <span className="absolute left-3 top-2 text-xs text-[#656B75] dark:text-[#A1A7B0] font-mono">¥</span>
                  <input
                    type="number"
                    min="1"
                    step="0.01"
                    value={customAmount}
                    onChange={(e) => setCustomAmount(e.target.value)}
                    placeholder={isZh ? '输入金额' : 'Enter amount'}
                    className="ui-field pl-7 py-1.5 font-mono"
                  />
                </div>
              </div>
            </div>

            {/* Select Channel */}
            <div className="space-y-3 pt-2">
              <label className="block text-xs font-semibold text-[#202124] dark:text-[#F1F2F4]">{isZh ? '支付通道' : 'Payment Gateway'}</label>
              <div className="max-w-md">
                <div
                  className="p-4 rounded-xl border border-[#355CFF] bg-[#EEF2FF]/70 dark:bg-[#242D48]/60 ring-1 ring-[#355CFF]/30 flex items-start space-x-3"
                >
                  <div className="w-8 h-8 rounded-lg bg-[#EEF2FF] dark:bg-[#242D48] text-[#355CFF] dark:text-[#91A7FF] flex items-center justify-center shrink-0">
                    <CreditCard className="w-4 h-4" />
                  </div>
                  <div>
                    <div className="font-semibold text-xs text-[#202124] dark:text-[#F1F2F4] flex items-center space-x-2">
                      <span>{isZh ? 'Stripe 信用卡支付' : 'Stripe Cards & Global Pay'}</span>
                      <span className="text-xs px-2 py-0.5 bg-[#EEF2FF] dark:bg-[#242D48] text-[#355CFF] dark:text-[#91A7FF] rounded-md font-medium whitespace-nowrap">{isZh ? '官方渠道' : 'Official'}</span>
                    </div>
                    <div className="ui-meta mt-1">{walletData.stripe_enabled ? (isZh ? '通过 Stripe 使用银行卡支付' : 'Pay by card through Stripe') : (isZh ? '在线支付尚未开通，请使用兑换券或联系管理员' : 'Online payment is unavailable. Use a gift card or contact support.')}</div>
                  </div>
                </div>
              </div>
            </div>

            {/* Submit Recharge */}
            <div className="pt-2">
              <button
                type="button"
                onClick={handleRecharge}
                disabled={recharging || !walletData.stripe_enabled}
                className="ui-btn-primary"
              >
                {recharging ? (
                  <RefreshCw className="w-4 h-4 animate-spin" />
                ) : (
                  <>
                    <span>{isZh ? '立即支付' : 'Pay Now'} ¥ {customAmount ? parseFloat(customAmount).toFixed(2) : rechargeAmount.toFixed(2)}</span>
                    <ArrowRight className="w-4 h-4" />
                  </>
                )}
              </button>
            </div>
          </div>

          {/* Recharge History Table */}
          <div className="ui-card p-6 space-y-4">
            <div className="flex items-center justify-between border-b border-[#E4E6E9] dark:border-[#2C3038] pb-3">
              <h3 className="font-semibold text-sm text-[#202124] dark:text-[#F1F2F4] flex items-center space-x-2">
                <Clock className="w-4 h-4 text-[#656B75] dark:text-[#A1A7B0]" />
                <span>{isZh ? '最近充值记录' : 'Recent Recharge History'}</span>
              </h3>
            </div>

            {walletData.orders && walletData.orders.length > 0 ? (
              <div className="overflow-x-auto">
                <table className="w-full text-left text-xs text-[#202124] dark:text-[#F1F2F4]">
                  <thead className="bg-[#F0F1F2]/60 dark:bg-[#22252B]/60 text-[#656B75] dark:text-[#A1A7B0] text-xs font-semibold border-b border-[#E4E6E9] dark:border-[#2C3038]">
                    <tr>
                      <th className="ui-th">{isZh ? '订单号' : 'Order No'}</th>
                      <th className="ui-th">{isZh ? '金额' : 'Amount'}</th>
                      <th className="ui-th">{isZh ? '支付渠道' : 'Channel'}</th>
                      <th className="ui-th">{isZh ? '状态' : 'Status'}</th>
                      <th className="ui-th">{isZh ? '创建时间' : 'Created At'}</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-[#E4E6E9] dark:divide-[#2C3038]">
                    {walletData.orders.map((o) => (
                      <tr key={o.id || o.order_no} className="ui-tr-hover">
                        <td className="ui-td font-mono font-medium text-[#202124] dark:text-[#F1F2F4]">{o.order_no}</td>
                        <td className="ui-td font-semibold text-[#202124] dark:text-[#F1F2F4] font-mono tabular-nums">¥ {Number(o.amount).toFixed(2)}</td>
                        <td className="ui-td">
                          <span className="px-2 py-0.5 rounded-md bg-[#F0F1F2] dark:bg-[#22252B] text-[#202124] dark:text-[#F1F2F4] text-xs uppercase font-medium">
                            {o.channel}
                          </span>
                        </td>
                        <td className="ui-td">
                          {renderOrderStatus(o.status)}
                        </td>
                        <td className="ui-td text-[#656B75] dark:text-[#A1A7B0]">
                          {o.created_at ? new Date(o.created_at).toLocaleString(isZh ? 'zh-CN' : 'en-US') : '-'}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : !walletError && (
              <div className="py-8 text-center ui-meta">
                {isZh ? '暂无充值流水记录' : 'No recharge history records found'}
              </div>
            )}
          </div>
        </div>
      )}

      {/* TAB 2: Admin Redemption Cards Management */}
      {isAdmin && activeTab === 'redemptions' && (
        <div className="space-y-6">
          <div className="ui-card p-6 space-y-4">
            <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 border-b border-[#E4E6E9] dark:border-[#2C3038] pb-4">
              <div>
                <h3 className="font-semibold text-sm text-[#202124] dark:text-[#F1F2F4]">{isZh ? '额度兑换卡批次管理' : 'Gift Cards Batch Management'}</h3>
                <p className="ui-meta mt-0.5">{isZh ? '生成、分发及注销平台额度礼品卡密' : 'Generate, distribute and manage platform gift card codes'}</p>
              </div>
              <div className="flex items-center space-x-2">
                <button
                  onClick={fetchRedemptions}
                  className="ui-btn-secondary ui-btn-sm"
                  title={isZh ? '刷新列表' : 'Refresh list'}
                >
                  <RefreshCw className="w-3.5 h-3.5" />
                </button>
                <button
                  onClick={() => setShowGenModal(true)}
                  className="ui-btn-primary ui-btn-sm"
                >
                  <Plus className="w-3.5 h-3.5" />
                  <span>{isZh ? '批量生成兑换卡' : 'Generate Gift Cards'}</span>
                </button>
              </div>
            </div>

            {/* Redemptions Table */}
            {redemptionsError && <div role="alert" className="ui-panel p-4 text-sm">{redemptionsError}</div>}
            {redemptions.length > 0 ? (
              <div className="overflow-x-auto">
                <table className="w-full text-left text-xs text-[#202124] dark:text-[#F1F2F4]">
                  <thead className="bg-[#F0F1F2]/60 dark:bg-[#22252B]/60 text-[#656B75] dark:text-[#A1A7B0] text-xs font-semibold border-b border-[#E4E6E9] dark:border-[#2C3038]">
                    <tr>
                      <th className="ui-th">{isZh ? '兑换卡密 (Code)' : 'Card Code'}</th>
                      <th className="ui-th">{isZh ? '名称' : 'Name'}</th>
                      <th className="ui-th">{isZh ? '面额' : 'Amount'}</th>
                      <th className="ui-th">{isZh ? '状态' : 'Status'}</th>
                      <th className="ui-th">{isZh ? '使用用户' : 'Redeemed By'}</th>
                      <th className="ui-th">{isZh ? '使用时间' : 'Redeemed At'}</th>
                      <th className="ui-th text-right">{isZh ? '操作' : 'Actions'}</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-[#E4E6E9] dark:divide-[#2C3038]">
                    {redemptions.map((r) => (
                      <tr key={r.id} className="ui-tr-hover">
                        <td className="ui-td font-mono font-semibold text-[#355CFF] dark:text-[#91A7FF] flex items-center space-x-2">
                          <span>{r.code}</span>
                          <button
                            onClick={() => copyText(r.code, isZh ? '卡密' : 'Card Code')}
                            className="ui-icon-btn"
                            title={isZh ? '复制卡密' : 'Copy code'}
                          >
                            <Copy className="w-3.5 h-3.5" />
                          </button>
                        </td>
                        <td className="ui-td text-[#202124] dark:text-[#F1F2F4] font-medium">{r.name || (isZh ? '额度兑换卡' : 'Gift Card')}</td>
                        <td className="ui-td font-semibold font-mono tabular-nums text-[#202124] dark:text-[#F1F2F4]">¥ {Number(r.amount).toFixed(2)}</td>
                        <td className="ui-td">
                          <span
                            className={`px-2 py-0.5 rounded-full text-xs font-medium whitespace-nowrap ${
                              r.status === 'active'
                                ? 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 border border-emerald-500/30'
                                : 'bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0] border border-[#E4E6E9] dark:border-[#2C3038]'
                            }`}
                          >
                            {r.status === 'active' ? (isZh ? '未使用 (可用)' : 'Unused (Available)') : (isZh ? '已兑换使用' : 'Redeemed')}
                          </span>
                        </td>
                        <td className="ui-td font-medium text-[#202124] dark:text-[#F1F2F4]">{r.used_by || '-'}</td>
                        <td className="ui-td text-[#656B75] dark:text-[#A1A7B0] font-mono text-xs">
                          {r.used_at ? new Date(r.used_at).toLocaleString(isZh ? 'zh-CN' : 'en-US') : '-'}
                        </td>
                        <td className="ui-td text-right">
                          <button
                            onClick={() => handleDeleteRedemption(r.id)}
                            disabled={r.status !== 'active'}
                            className="p-1.5 hover:bg-rose-500/10 rounded-lg text-[#656B75] dark:text-[#A1A7B0] hover:text-rose-600 dark:hover:text-rose-400 transition cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed"
                            title={r.status === 'active' ? (isZh ? '删除兑换卡' : 'Delete card') : (isZh ? '已兑换记录保留用于核账' : 'Redeemed records are retained for reconciliation')}
                          >
                            <Trash2 className="w-4 h-4" />
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : !redemptionsError && (
              <div className="py-12 text-center ui-meta space-y-3">
                <Gift className="w-8 h-8 text-[#656B75]/40 mx-auto" />
                <p>{isZh ? '暂无兑换卡记录，点击右上角「批量生成兑换卡」即可立即创建卡密。' : 'No gift card records yet. Click "Generate Gift Cards" in the top right to create cards.'}</p>
              </div>
            )}
          </div>
        </div>
      )}

      {/* MODAL: Generate Redemption Cards */}
      {showGenModal && (
        <div
          className="fixed inset-0 bg-slate-900/60 flex items-center justify-center p-4 z-50 backdrop-blur-sm animate-in fade-in"
          onClick={(e) => {
            if (e.target === e.currentTarget) setShowGenModal(false);
          }}
        >
          <div className="bg-white dark:bg-[#191B1F] border border-[#E4E6E9] dark:border-[#2C3038] rounded-2xl p-6 max-w-md w-full shadow-2xl space-y-5 animate-in zoom-in-95 duration-150">
            <div className="flex items-center justify-between border-b border-[#E4E6E9] dark:border-[#2C3038] pb-3">
              <div className="flex items-center space-x-2">
                <div className="w-8 h-8 rounded-lg bg-[#EEF2FF] dark:bg-[#242D48] text-[#355CFF] dark:text-[#91A7FF] flex items-center justify-center">
                  <Gift className="w-4 h-4" />
                </div>
                <h3 className="font-semibold text-sm text-[#202124] dark:text-[#F1F2F4]">{isZh ? '批量生成额度兑换卡' : 'Batch Generate Gift Cards'}</h3>
              </div>
              <button
                onClick={() => setShowGenModal(false)}
                className="ui-icon-btn"
              >
                ✕
              </button>
            </div>

            <form onSubmit={handleGenerateRedemptions} className="space-y-4 text-xs">
              <div>
                <label className="ui-label">{isZh ? '兑换卡名称' : 'Card Name'}</label>
                <input
                  type="text"
                  value={genName}
                  onChange={(e) => setGenName(e.target.value)}
                  placeholder={isZh ? '例如: 2026 新年礼品卡' : 'e.g. 2026 Promo Gift Card'}
                  className="ui-field"
                />
              </div>

              <div>
                <label className="ui-label">{isZh ? '单张充值面额 (CNY)' : 'Amount per Card (CNY)'}</label>
                <input
                  type="number"
                  min="0.1"
                  step="0.1"
                  required
                  value={genAmount}
                  onChange={(e) => setGenAmount(e.target.value)}
                  placeholder="50"
                  className="ui-field font-mono tabular-nums"
                />
              </div>

              <div>
                <label className="ui-label">{isZh ? '生成数量 (张)' : 'Quantity'}</label>
                <input
                  type="number"
                  min="1"
                  max="100"
                  required
                  value={genCount}
                  onChange={(e) => setGenCount(e.target.value)}
                  placeholder="5"
                  className="ui-field font-mono tabular-nums"
                />
                <p className="ui-meta mt-1">{isZh ? '一次最多可生成 100 张随机加密卡密。' : 'Up to 100 encrypted gift codes can be generated per batch.'}</p>
              </div>

              <div className="pt-3 flex items-center justify-end space-x-2 border-t border-[#E4E6E9] dark:border-[#2C3038]">
                <button
                  type="button"
                  onClick={() => setShowGenModal(false)}
                  className="ui-btn-secondary"
                >
                  {isZh ? '取消' : 'Cancel'}
                </button>
                <button
                  type="submit"
                  disabled={generating}
                  className="ui-btn-primary"
                >
                  {generating ? (
                    <RefreshCw className="w-3.5 h-3.5 animate-spin" />
                  ) : (
                    <>
                      <Sparkles className="w-3.5 h-3.5" />
                      <span>{isZh ? '确认生成' : 'Generate Cards'}</span>
                    </>
                  )}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
