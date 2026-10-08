import React, { useState, useEffect } from 'react';
import {
  Wallet,
  CreditCard,
  Gift,
  Plus,
  RefreshCw,
  CheckCircle2,
  AlertCircle,
  Copy,
  Trash2,
  ExternalLink,
  Zap,
  ArrowRight,
  ShieldCheck,
  Clock,
  Sparkles,
  DollarSign
} from 'lucide-react';

export default function WalletManagementView({ adminUser, adminFetch, showToast, onUserUpdated, onBalanceUpdate }) {
  const isAdmin = adminUser?.role === 'admin';
  const [walletData, setWalletData] = useState({
    balance: adminUser?.balance || 0,
    role: adminUser?.role || 'user',
    status: adminUser?.status || 'active',
    group_name: adminUser?.group_name || 'default',
    orders: []
  });
  const [loading, setLoading] = useState(false);

  // Tab: 'wallet' (User wallet & recharge) or 'redemptions' (Admin gift cards)
  const [activeTab, setActiveTab] = useState('wallet');

  // Recharge State
  const [rechargeAmount, setRechargeAmount] = useState(50);
  const [customAmount, setCustomAmount] = useState('');
  const [rechargeMethod, setRechargeMethod] = useState('stripe'); // 'stripe' (primary) or 'sandbox' (admin only)
  const [recharging, setRecharging] = useState(false);

  // Redeem State
  const [redeemCode, setRedeemCode] = useState('');
  const [redeeming, setRedeeming] = useState(false);

  // Admin Redemption Cards
  const [redemptions, setRedemptions] = useState([]);
  const [showGenModal, setShowGenModal] = useState(false);
  const [genCount, setGenCount] = useState(5);
  const [genAmount, setGenAmount] = useState(50);
  const [genName, setGenName] = useState('额度兑换卡');
  const [generating, setGenerating] = useState(false);

  const fetchWallet = async () => {
    if (!adminFetch) return;
    setLoading(true);
    try {
      const res = await adminFetch('/api/v1/user/wallet');
      const data = await res.json();
      if (res.ok && data.code === 0 && data.data) {
        setWalletData(data.data);
        if (onUserUpdated) {
          onUserUpdated(data.data);
        }
        if (onBalanceUpdate && data.data.balance !== undefined) {
          onBalanceUpdate(data.data.balance);
        }
      }
    } catch (err) {
      console.error('fetch wallet err:', err);
    } finally {
      setLoading(false);
    }
  };

  const fetchRedemptions = async () => {
    if (!adminFetch || !isAdmin) return;
    try {
      const res = await adminFetch('/api/v1/admin/redemptions');
      const data = await res.json();
      if (res.ok && data.code === 0) {
        setRedemptions(data.data || []);
      }
    } catch (err) {
      console.error('fetch redemptions err:', err);
    }
  };

  useEffect(() => {
    fetchWallet();
    if (isAdmin) {
      fetchRedemptions();
    }
  }, []);

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
  }, [adminUser?.balance, adminUser?.role, adminUser?.status, adminUser?.group_name]);

  const handleRedeem = async (e) => {
    if (e) e.preventDefault();
    if (!redeemCode.trim()) {
      showToast('请输入兑换码', 'warning');
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
        showToast(data.message || '兑换成功', 'success');
        setRedeemCode('');
        fetchWallet();
        if (isAdmin) fetchRedemptions();
      } else {
        showToast(data.error || '兑换失败', 'error');
      }
    } catch (err) {
      showToast('网络请求异常: ' + err.message, 'error');
    } finally {
      setRedeeming(false);
    }
  };

  const handleRecharge = async () => {
    const finalAmount = customAmount ? parseFloat(customAmount) : rechargeAmount;
    if (isNaN(finalAmount) || finalAmount <= 0) {
      showToast('请输入有效的充值金额', 'warning');
      return;
    }

    setRecharging(true);
    try {
      if (rechargeMethod === 'sandbox') {
        const res = await adminFetch('/api/v1/user/wallet/recharge/sandbox', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ amount: finalAmount })
        });
        const data = await res.json();
        if (res.ok && data.code === 0) {
          showToast(data.message || '充值成功！额度已实时到账', 'success');
          fetchWallet();
        } else {
          showToast(data.error || '充值处理失败', 'error');
        }
      } else {
        // Stripe Checkout
        const res = await adminFetch('/api/v1/user/wallet/recharge/stripe/session', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ amount: finalAmount, currency: 'CNY' })
        });
        const data = await res.json();
        if (res.ok && data.code === 0 && data.checkout_url) {
          if (data.mode === 'sandbox_simulation') {
            if (isAdmin || walletData?.is_admin) {
              showToast('Stripe 生产密钥未配置，已转入管理员沙箱测试通道', 'info');
              const simRes = await adminFetch('/api/v1/user/wallet/recharge/sandbox', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ order_no: data.order_no, amount: finalAmount })
              });
              const simData = await simRes.json();
              if (simRes.ok && simData.code === 0) {
                showToast(`[测试沙箱] 模拟充值成功！已入账 ¥${finalAmount.toFixed(2)}`, 'success');
                fetchWallet();
              }
            } else {
              showToast('企业未配置在线支付通道，请使用兑换码充值或联系管理员', 'warning');
            }
          } else {
            window.location.href = data.checkout_url;
          }
        } else {
          showToast(data.error || '创建 Stripe 支付会话失败', 'error');
        }
      }
    } catch (err) {
      showToast('充值请求异常: ' + err.message, 'error');
    } finally {
      setRecharging(false);
    }
  };

  const handleGenerateRedemptions = async (e) => {
    if (e) e.preventDefault();
    if (genAmount <= 0) {
      showToast('请输入有效的单卡金额', 'warning');
      return;
    }
    setGenerating(true);
    try {
      const res = await adminFetch('/api/v1/admin/redemptions/generate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          count: parseInt(genCount, 10) || 1,
          amount: parseFloat(genAmount),
          name: genName.trim() || '额度兑换卡'
        })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(data.message || '成功生成兑换卡', 'success');
        setShowGenModal(false);
        fetchRedemptions();
      } else {
        showToast(data.error || '生成失败', 'error');
      }
    } catch (err) {
      showToast('请求异常: ' + err.message, 'error');
    } finally {
      setGenerating(false);
    }
  };

  const handleDeleteRedemption = async (id) => {
    if (!window.confirm('确认删除该兑换卡记录吗？已删除后将不可兑换。')) return;
    try {
      const res = await adminFetch(`/api/v1/admin/redemptions/${id}`, {
        method: 'DELETE'
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast('兑换卡已删除', 'success');
        fetchRedemptions();
      } else {
        showToast(data.error || '删除失败', 'error');
      }
    } catch (err) {
      showToast('删除请求异常: ' + err.message, 'error');
    }
  };

  const copyText = (txt, label = '内容') => {
    navigator.clipboard.writeText(txt);
    showToast(`已复制 ${label} 到剪贴板`, 'success');
  };

  return (
    <div className="space-y-6">
      {/* Header Tabs for Admin */}
      {isAdmin && (
        <div className="flex items-center space-x-2 border-b border-slate-200 pb-3">
          <button
            onClick={() => setActiveTab('wallet')}
            className={`px-4 py-2 rounded-xl text-xs font-bold transition flex items-center space-x-2 ${
              activeTab === 'wallet'
                ? 'bg-indigo-600 text-white shadow-sm'
                : 'bg-white hover:bg-slate-100 text-slate-700 border border-slate-200'
            }`}
          >
            <Wallet className="w-4 h-4" />
            <span>我的钱包与充值</span>
          </button>
          <button
            onClick={() => setActiveTab('redemptions')}
            className={`px-4 py-2 rounded-xl text-xs font-bold transition flex items-center space-x-2 ${
              activeTab === 'redemptions'
                ? 'bg-indigo-600 text-white shadow-sm'
                : 'bg-white hover:bg-slate-100 text-slate-700 border border-slate-200'
            }`}
          >
            <Gift className="w-4 h-4" />
            <span>额度兑换卡管理 (管理员)</span>
          </button>
        </div>
      )}

      {/* TAB 1: User Wallet & Recharge */}
      {activeTab === 'wallet' && (
        <div className="space-y-6">
          {/* Summary Stat Cards */}
          <div className="grid grid-cols-1 md:grid-cols-3 gap-5">
            {/* Balance Card */}
            <div className="bg-gradient-to-br from-indigo-900 via-indigo-800 to-slate-900 rounded-3xl p-6 text-white shadow-xl relative overflow-hidden flex flex-col justify-between">
              <div className="absolute right-0 top-0 w-36 h-36 bg-indigo-500/10 rounded-full blur-2xl pointer-events-none"></div>
              <div>
                <div className="flex items-center justify-between text-indigo-200 text-xs font-medium">
                  <div className="flex items-center space-x-2">
                    <Wallet className="w-4 h-4 text-indigo-400" />
                    <span>钱包当前可用余额</span>
                  </div>
                  <button
                    onClick={fetchWallet}
                    className="p-1 hover:bg-white/10 rounded-lg text-indigo-200 hover:text-white transition"
                    title="刷新余额"
                  >
                    <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
                  </button>
                </div>
                <div className="mt-3 flex items-baseline space-x-2">
                  {isAdmin ? (
                    <span className="text-3xl font-extrabold tracking-tight text-white flex items-center space-x-2">
                      <span>¥ 无限额度</span>
                      <span className="text-xs px-2 py-0.5 rounded-full bg-indigo-500/30 border border-indigo-400/40 text-indigo-200 font-normal">
                        管理员豁免
                      </span>
                    </span>
                  ) : (
                    <span className="text-4xl font-black tracking-tight text-white font-mono">
                      ¥ {Number(walletData.balance || 0).toFixed(2)}
                    </span>
                  )}
                </div>
              </div>

              <div className="mt-6 pt-4 border-t border-indigo-700/60 flex items-center justify-between text-xs text-indigo-200">
                <span>用户角色: {isAdmin ? '超级管理员' : '普通用户'}</span>
              </div>
            </div>

            {/* Pricing Group Tier Card */}
            <div className="bg-white border border-slate-200/80 rounded-3xl p-6 shadow-xs flex flex-col justify-between">
              <div>
                <div className="text-xs font-semibold text-slate-500 flex items-center space-x-2">
                  <Sparkles className="w-4 h-4 text-amber-500" />
                  <span>账号保障等级 (Account Tier)</span>
                </div>
                <div className="mt-3">
                  <span className="inline-flex items-center px-3 py-1 rounded-xl text-sm font-bold bg-amber-50 text-amber-700 border border-amber-200">
                    {walletData.group_name === 'default'
                      ? '默认分组 (Standard)'
                      : walletData.group_name.toUpperCase() + ' 专属保障'}
                  </span>
                  <div className="text-[11px] text-slate-400 mt-1.5">
                    创建 API Key 时可使用该等级专属费率或选择标准默认组
                  </div>
                </div>
              </div>

              <div className="mt-4 pt-3 border-t border-slate-100 flex items-center justify-between text-xs text-slate-500">
                <span>账户状态:</span>
                <span
                  className={`font-semibold px-2 py-0.5 rounded-md ${
                    walletData.status === 'locked'
                      ? 'bg-rose-100 text-rose-700'
                      : 'bg-emerald-100 text-emerald-700'
                  }`}
                >
                  {walletData.status === 'locked' ? '已锁定' : '正常活跃'}
                </span>
              </div>
            </div>

            {/* Quick Redeem Card */}
            <div className="bg-white border border-slate-200/80 rounded-3xl p-6 shadow-xs flex flex-col justify-between">
              <div>
                <div className="text-xs font-semibold text-slate-500 flex items-center space-x-2">
                  <Gift className="w-4 h-4 text-purple-500" />
                  <span>兑换码快速充值</span>
                </div>
                <form onSubmit={handleRedeem} className="mt-3 space-y-2.5">
                  <input
                    type="text"
                    value={redeemCode}
                    onChange={(e) => setRedeemCode(e.target.value)}
                    placeholder="输入卡号 CARD-XXXX-XXXX"
                    className="w-full bg-slate-50 border border-slate-200 rounded-xl px-3 py-2 text-xs font-mono text-slate-900 focus:outline-none focus:border-indigo-500 focus:bg-white uppercase"
                  />
                  <button
                    type="submit"
                    disabled={redeeming}
                    className="w-full py-2 bg-gradient-to-r from-purple-600 to-indigo-600 hover:from-purple-700 hover:to-indigo-700 text-white rounded-xl text-xs font-bold transition flex items-center justify-center space-x-1.5 shadow-sm disabled:opacity-50 cursor-pointer"
                  >
                    {redeeming ? (
                      <RefreshCw className="w-3.5 h-3.5 animate-spin" />
                    ) : (
                      <>
                        <Zap className="w-3.5 h-3.5" />
                        <span>立即兑换入账</span>
                      </>
                    )}
                  </button>
                </form>
              </div>
            </div>
          </div>

          {/* Recharge Section */}
          <div className="bg-white border border-slate-200/80 rounded-3xl p-6 shadow-xs space-y-5">
            <div className="flex items-center justify-between border-b border-slate-100 pb-4">
              <div className="flex items-center space-x-3">
                <div className="w-10 h-10 rounded-2xl bg-indigo-50 border border-indigo-200 flex items-center justify-center text-indigo-600">
                  <CreditCard className="w-5 h-5" />
                </div>
                <div>
                  <h3 className="font-bold text-base text-slate-900">钱包余额在线充值</h3>
                </div>
              </div>
            </div>

            {/* Select Amount */}
            <div className="space-y-3">
              <label className="block text-xs font-bold text-slate-700">选择充值金额 (CNY)</label>
              <div className="grid grid-cols-2 sm:grid-cols-5 gap-3">
                {[10, 30, 50, 100, 200].map((amt) => (
                  <button
                    key={amt}
                    type="button"
                    onClick={() => {
                      setRechargeAmount(amt);
                      setCustomAmount('');
                    }}
                    className={`py-3 rounded-2xl border text-sm font-bold transition cursor-pointer flex flex-col items-center justify-center ${
                      rechargeAmount === amt && !customAmount
                        ? 'bg-indigo-50 border-indigo-500 text-indigo-700 shadow-sm'
                        : 'bg-slate-50/70 hover:bg-slate-100 border-slate-200 text-slate-700'
                    }`}
                  >
                    <span>¥ {amt}</span>
                  </button>
                ))}
              </div>

              <div className="flex items-center space-x-3 pt-1">
                <span className="text-xs text-slate-500">或自定义金额:</span>
                <div className="relative w-40">
                  <span className="absolute left-3 top-2 text-xs text-slate-400">¥</span>
                  <input
                    type="number"
                    min="1"
                    step="0.01"
                    value={customAmount}
                    onChange={(e) => setCustomAmount(e.target.value)}
                    placeholder="输入金额"
                    className="w-full bg-slate-50 border border-slate-200 rounded-xl pl-7 pr-3 py-1.5 text-xs text-slate-900 focus:outline-none focus:border-indigo-500 focus:bg-white"
                  />
                </div>
              </div>
            </div>

            {/* Select Channel */}
            <div className="space-y-3 pt-2">
              <label className="block text-xs font-bold text-slate-700">选择支付通道</label>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <button
                  type="button"
                  onClick={() => setRechargeMethod('stripe')}
                  className={`p-4 rounded-2xl border text-left transition cursor-pointer flex items-start space-x-3 ${
                    rechargeMethod === 'stripe'
                      ? 'bg-indigo-50/70 border-indigo-500 shadow-sm'
                      : 'bg-slate-50/60 border-slate-200 hover:bg-slate-100'
                  }`}
                >
                  <div className="w-8 h-8 rounded-xl bg-indigo-100 text-indigo-700 flex items-center justify-center shrink-0">
                    <CreditCard className="w-4 h-4" />
                  </div>
                  <div>
                    <div className="font-bold text-xs text-slate-900 flex items-center space-x-2">
                      <span>Stripe 国际信用卡 / 企业网银</span>
                      <span className="text-[10px] px-2 py-0.5 bg-indigo-100 text-indigo-800 rounded-full font-semibold">官方渠道</span>
                    </div>
                    <div className="text-[11px] text-slate-500 mt-1">支持 Visa, MasterCard, 微信/支付宝跨境结算</div>
                  </div>
                </button>

                {(isAdmin || walletData?.is_admin) && (
                  <button
                    type="button"
                    onClick={() => setRechargeMethod('sandbox')}
                    className={`p-4 rounded-2xl border text-left transition cursor-pointer flex items-start space-x-3 ${
                      rechargeMethod === 'sandbox'
                        ? 'bg-amber-50/70 border-amber-500 shadow-sm'
                        : 'bg-slate-50/60 border-slate-200 hover:bg-slate-100'
                    }`}
                  >
                    <div className="w-8 h-8 rounded-xl bg-amber-100 text-amber-700 flex items-center justify-center shrink-0">
                      <Zap className="w-4 h-4" />
                    </div>
                    <div>
                      <div className="font-bold text-xs text-slate-900 flex items-center space-x-2">
                        <span>管理员沙箱测试通道</span>
                        <span className="text-[10px] px-2 py-0.5 bg-amber-100 text-amber-800 rounded-full font-semibold">仅开发/管理</span>
                      </div>
                      <div className="text-[11px] text-slate-500 mt-1">仅超级管理员可在测试环境中用于快速联调额度</div>
                    </div>
                  </button>
                )}
              </div>
            </div>

            {/* Submit Recharge */}
            <div className="pt-2">
              <button
                type="button"
                onClick={handleRecharge}
                disabled={recharging}
                className="px-6 py-2.5 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-bold transition flex items-center space-x-2 shadow-md hover:shadow-lg disabled:opacity-50 cursor-pointer"
              >
                {recharging ? (
                  <RefreshCw className="w-4 h-4 animate-spin" />
                ) : (
                  <>
                    <span>立即支付 ¥ {customAmount ? parseFloat(customAmount).toFixed(2) : rechargeAmount.toFixed(2)}</span>
                    <ArrowRight className="w-4 h-4" />
                  </>
                )}
              </button>
            </div>
          </div>

          {/* Recharge History Table */}
          <div className="bg-white border border-slate-200/80 rounded-3xl p-6 shadow-xs space-y-4">
            <div className="flex items-center justify-between border-b border-slate-100 pb-3">
              <h3 className="font-bold text-sm text-slate-900 flex items-center space-x-2">
                <Clock className="w-4 h-4 text-slate-400" />
                <span>最近充值记录</span>
              </h3>
            </div>

            {walletData.orders && walletData.orders.length > 0 ? (
              <div className="overflow-x-auto">
                <table className="w-full text-left text-xs text-slate-600">
                  <thead className="bg-slate-50 text-slate-500 uppercase text-[10px] font-bold border-b border-slate-200">
                    <tr>
                      <th className="py-2.5 px-3">订单号</th>
                      <th className="py-2.5 px-3">金额</th>
                      <th className="py-2.5 px-3">支付渠道</th>
                      <th className="py-2.5 px-3">状态</th>
                      <th className="py-2.5 px-3">创建时间</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-slate-100">
                    {walletData.orders.map((o) => (
                      <tr key={o.id || o.order_no} className="hover:bg-slate-50/70">
                        <td className="py-2.5 px-3 font-mono font-medium text-slate-900">{o.order_no}</td>
                        <td className="py-2.5 px-3 font-bold text-slate-900">¥ {Number(o.amount).toFixed(2)}</td>
                        <td className="py-2.5 px-3">
                          <span className="px-2 py-0.5 rounded-md bg-slate-100 text-slate-700 text-[10px] uppercase font-semibold">
                            {o.channel}
                          </span>
                        </td>
                        <td className="py-2.5 px-3">
                          <span
                            className={`px-2 py-0.5 rounded-full text-[10px] font-bold ${
                              o.status === 'paid'
                                ? 'bg-emerald-100 text-emerald-800'
                                : 'bg-amber-100 text-amber-800'
                            }`}
                          >
                            {o.status === 'paid' ? '已支付入账' : '处理中'}
                          </span>
                        </td>
                        <td className="py-2.5 px-3 text-slate-400">
                          {o.created_at ? new Date(o.created_at).toLocaleString('zh-CN') : '-'}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <div className="py-8 text-center text-xs text-slate-400">
                暂无充值流水记录
              </div>
            )}
          </div>
        </div>
      )}

      {/* TAB 2: Admin Redemption Cards Management */}
      {isAdmin && activeTab === 'redemptions' && (
        <div className="space-y-6">
          <div className="bg-white border border-slate-200/80 rounded-3xl p-6 shadow-xs space-y-4">
            <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 border-b border-slate-100 pb-4">
              <div>
                <h3 className="font-bold text-base text-slate-900">额度兑换卡批次管理</h3>
              </div>
              <div className="flex items-center space-x-2">
                <button
                  onClick={fetchRedemptions}
                  className="p-2 border border-slate-200 rounded-xl hover:bg-slate-50 text-slate-600 transition"
                  title="刷新列表"
                >
                  <RefreshCw className="w-4 h-4" />
                </button>
                <button
                  onClick={() => setShowGenModal(true)}
                  className="px-3.5 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-bold transition flex items-center space-x-1.5 shadow-sm cursor-pointer"
                >
                  <Plus className="w-4 h-4" />
                  <span>批量生成兑换卡</span>
                </button>
              </div>
            </div>

            {/* Redemptions Table */}
            {redemptions.length > 0 ? (
              <div className="overflow-x-auto">
                <table className="w-full text-left text-xs text-slate-600">
                  <thead className="bg-slate-50 text-slate-500 uppercase text-[10px] font-bold border-b border-slate-200">
                    <tr>
                      <th className="py-2.5 px-3">兑换卡密 (Code)</th>
                      <th className="py-2.5 px-3">名称</th>
                      <th className="py-2.5 px-3">面额</th>
                      <th className="py-2.5 px-3">状态</th>
                      <th className="py-2.5 px-3">使用用户</th>
                      <th className="py-2.5 px-3">使用时间</th>
                      <th className="py-2.5 px-3 text-right">操作</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-slate-100">
                    {redemptions.map((r) => (
                      <tr key={r.id} className="hover:bg-slate-50/70">
                        <td className="py-2.5 px-3 font-mono font-bold text-indigo-600 flex items-center space-x-2">
                          <span>{r.code}</span>
                          <button
                            onClick={() => copyText(r.code, '卡密')}
                            className="p-1 hover:bg-slate-200 rounded text-slate-400 hover:text-slate-600 transition"
                            title="复制卡密"
                          >
                            <Copy className="w-3.5 h-3.5" />
                          </button>
                        </td>
                        <td className="py-2.5 px-3 text-slate-900 font-medium">{r.name || '额度兑换卡'}</td>
                        <td className="py-2.5 px-3 font-bold text-slate-900">¥ {Number(r.amount).toFixed(2)}</td>
                        <td className="py-2.5 px-3">
                          <span
                            className={`px-2 py-0.5 rounded-full text-[10px] font-bold ${
                              r.status === 'active'
                                ? 'bg-emerald-100 text-emerald-800'
                                : 'bg-slate-100 text-slate-600'
                            }`}
                          >
                            {r.status === 'active' ? '未使用 (可用)' : '已兑换使用'}
                          </span>
                        </td>
                        <td className="py-2.5 px-3 font-medium text-slate-700">{r.used_by || '-'}</td>
                        <td className="py-2.5 px-3 text-slate-400">
                          {r.used_at ? new Date(r.used_at).toLocaleString('zh-CN') : '-'}
                        </td>
                        <td className="py-2.5 px-3 text-right">
                          <button
                            onClick={() => handleDeleteRedemption(r.id)}
                            className="p-1.5 hover:bg-rose-50 rounded-lg text-slate-400 hover:text-rose-600 transition"
                            title="删除兑换卡"
                          >
                            <Trash2 className="w-4 h-4" />
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <div className="py-12 text-center text-xs text-slate-400 space-y-3">
                <Gift className="w-8 h-8 text-slate-300 mx-auto" />
                <p>暂无兑换卡记录，点击右上角「批量生成兑换卡」即可立即创建卡密。</p>
              </div>
            )}
          </div>
        </div>
      )}

      {/* MODAL: Generate Redemption Cards */}
      {showGenModal && (
        <div className="fixed inset-0 bg-slate-900/60 flex items-center justify-center p-4 z-50 backdrop-blur-sm animate-in fade-in">
          <div className="bg-white border border-slate-200 rounded-3xl p-6 max-w-md w-full shadow-2xl space-y-5 animate-in zoom-in-95 duration-150">
            <div className="flex items-center justify-between border-b border-slate-100 pb-3">
              <div className="flex items-center space-x-2">
                <Gift className="w-5 h-5 text-indigo-600" />
                <h3 className="font-bold text-base text-slate-900">批量生成额度兑换卡</h3>
              </div>
              <button
                onClick={() => setShowGenModal(false)}
                className="p-1 hover:bg-slate-100 rounded-lg text-slate-400 hover:text-slate-600"
              >
                ✕
              </button>
            </div>

            <form onSubmit={handleGenerateRedemptions} className="space-y-4">
              <div>
                <label className="block text-xs font-semibold text-slate-700 mb-1">兑换卡名称</label>
                <input
                  type="text"
                  value={genName}
                  onChange={(e) => setGenName(e.target.value)}
                  placeholder="例如: 2026 新年礼品卡"
                  className="w-full bg-slate-50 border border-slate-200 rounded-xl px-3 py-2 text-xs text-slate-900 focus:outline-none focus:border-indigo-500 focus:bg-white"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-700 mb-1">单张充值面额 (CNY)</label>
                <input
                  type="number"
                  min="0.1"
                  step="0.1"
                  required
                  value={genAmount}
                  onChange={(e) => setGenAmount(e.target.value)}
                  placeholder="50"
                  className="w-full bg-slate-50 border border-slate-200 rounded-xl px-3 py-2 text-xs text-slate-900 focus:outline-none focus:border-indigo-500 focus:bg-white font-mono"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-700 mb-1">生成数量 (张)</label>
                <input
                  type="number"
                  min="1"
                  max="100"
                  required
                  value={genCount}
                  onChange={(e) => setGenCount(e.target.value)}
                  placeholder="5"
                  className="w-full bg-slate-50 border border-slate-200 rounded-xl px-3 py-2 text-xs text-slate-900 focus:outline-none focus:border-indigo-500 focus:bg-white font-mono"
                />
                <p className="text-[11px] text-slate-400 mt-1">一次最多可生成 100 张随机加密卡密。</p>
              </div>

              <div className="pt-3 flex items-center justify-end space-x-2">
                <button
                  type="button"
                  onClick={() => setShowGenModal(false)}
                  className="px-4 py-2 border border-slate-200 hover:bg-slate-50 text-slate-700 rounded-xl text-xs font-medium"
                >
                  取消
                </button>
                <button
                  type="submit"
                  disabled={generating}
                  className="px-5 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-bold transition flex items-center space-x-1.5 shadow-sm disabled:opacity-50 cursor-pointer"
                >
                  {generating ? (
                    <RefreshCw className="w-3.5 h-3.5 animate-spin" />
                  ) : (
                    <>
                      <Sparkles className="w-3.5 h-3.5" />
                      <span>确认生成</span>
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
