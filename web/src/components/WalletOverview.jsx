import { Wallet, CreditCard, Gift, RefreshCw, Zap, ArrowRight, Clock, Sparkles } from 'lucide-react';

export default function WalletOverview({
  customAmount, fetchWallet, handleRecharge, handleRedeem, isAdmin, isZh, loading, rechargeAmount, recharging,
  redeemCode, redeeming, renderOrderStatus, setCustomAmount, setRechargeAmount, setRedeemCode, walletData,
  walletError
}) {
  return (
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
                min={walletData.stripe_min_amount || 10}
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
  );
}
