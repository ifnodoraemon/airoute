import useWalletPresentation from '../hooks/useWalletPresentation';
import useWalletActions from '../hooks/useWalletActions';
import useWalletData from '../hooks/useWalletData';
import useWalletState from '../hooks/useWalletState';
import WalletOverview from './WalletOverview';
import WalletRedemptionCards from './WalletRedemptionCards';
import WalletGenerateModal from './WalletGenerateModal';

import { Wallet, Gift } from 'lucide-react';

export default function WalletManagementView({ adminUser, adminFetch, showToast, onUserUpdated, onBalanceUpdate, lang = 'zh' }) {
  const isZh = lang === 'zh';
  const isAdmin = adminUser?.role === 'admin';
  const {
    activeTab, checkoutMessage, checkoutReturn, clearCheckoutIntent, customAmount, genAmount, genCount, genName,
    generating, intentStorageKey, loading, rechargeAmount, rechargeInFlightRef, recharging, redeemCode,
    redeeming, redemptions, redemptionsError, setActiveTab, setCheckoutMessage, setCustomAmount, setGenAmount,
    setGenCount, setGenName, setGenerating, setLoading, setRechargeAmount, setRecharging, setRedeemCode,
    setRedeeming, setRedemptions, setRedemptionsError, setShowGenModal, setWalletData, setWalletError,
    showGenModal, walletData, walletError
  } = useWalletState({
    adminUser, isZh
  });

  const {
    fetchRedemptions, fetchWallet
  } = useWalletData({
    adminFetch, adminUser, checkoutReturn, clearCheckoutIntent, isAdmin, isZh, onBalanceUpdate, onUserUpdated,
    setCheckoutMessage, setLoading, setRedemptions, setRedemptionsError, setWalletData, setWalletError
  });

  const {
    copyText, handleDeleteRedemption, handleGenerateRedemptions, handleRecharge, handleRedeem
  } = useWalletActions({
    adminFetch, clearCheckoutIntent, customAmount, fetchRedemptions, fetchWallet, genAmount, genCount, genName,
    intentStorageKey, isAdmin, isZh, rechargeAmount, rechargeInFlightRef, redeemCode, setGenerating,
    setRecharging, setRedeemCode, setRedeeming, setShowGenModal, showToast, walletData
  });

  const {
    renderOrderStatus
  } = useWalletPresentation({
    isZh
  });

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
        <WalletOverview {...{
          customAmount, fetchWallet, handleRecharge, handleRedeem, isAdmin, isZh, loading, rechargeAmount, recharging,
          redeemCode, redeeming, renderOrderStatus, setCustomAmount, setRechargeAmount, setRedeemCode, walletData,
          walletError
        }} />
      )}

      {/* TAB 2: Admin Redemption Cards Management */}
      {isAdmin && activeTab === 'redemptions' && (
        <WalletRedemptionCards {...{
          copyText, fetchRedemptions, handleDeleteRedemption, isZh, redemptions, redemptionsError, setShowGenModal
        }} />
      )}

      {/* MODAL: Generate Redemption Cards */}
      {showGenModal && (
        <WalletGenerateModal {...{
          genAmount, genCount, genName, generating, handleGenerateRedemptions, isZh, setGenAmount, setGenCount,
          setGenName, setShowGenModal
        }} />
      )}
    </div>
  );
}
