import { useState, useCallback, useRef } from 'react';
import useModalA11y from './useModalA11y';

export default function useWalletState({
  adminUser, isZh
}) {
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
  const rechargeInFlightRef = useRef(false);
  const intentStorageKey = `airoute-checkout:${adminUser?.username || ''}`;
  const clearCheckoutIntent = useCallback((order) => {
    if (order) {
      try {
        const current = JSON.parse(sessionStorage.getItem(intentStorageKey) || 'null');
        if (current?.order && current.order !== order) return;
      } catch { /* Clear malformed local state. */ }
    }
    sessionStorage.removeItem(intentStorageKey);
  }, [intentStorageKey]);
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
  return {
    activeTab, checkoutMessage, checkoutReturn, clearCheckoutIntent, customAmount, genAmount, genCount, genName,
    generating, intentStorageKey, loading, rechargeAmount, rechargeInFlightRef, recharging, redeemCode,
    redeeming, redemptions, redemptionsError, setActiveTab, setCheckoutMessage, setCustomAmount, setGenAmount,
    setGenCount, setGenName, setGenerating, setLoading, setRechargeAmount, setRecharging, setRedeemCode,
    setRedeeming, setRedemptions, setRedemptionsError, setShowGenModal, setWalletData, setWalletError,
    showGenModal, walletData, walletError
  };
}
