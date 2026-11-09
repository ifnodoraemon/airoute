import { apiMutation } from '../api/mutation';
import { copyToClipboard } from '../config';

export default function useWalletActions({
  adminFetch, clearCheckoutIntent, customAmount, fetchRedemptions, fetchWallet, genAmount, genCount, genName,
  intentStorageKey, isAdmin, isZh, rechargeAmount, rechargeInFlightRef, redeemCode, setGenerating,
  setRecharging, setRedeemCode, setRedeeming, setShowGenModal, showToast, walletData
}) {
  const handleRedeem = async (e) => {
    if (e) e.preventDefault();
    if (!redeemCode.trim()) {
      showToast(isZh ? '请输入兑换码' : 'Please enter gift card code', 'warning');
      return;
    }
    setRedeeming(true);
    try {
      const { data, ok: requestSucceeded } = await apiMutation('/api/v1/user/wallet/redeem', 'POST', { code: redeemCode.trim() }, adminFetch);

      if (requestSucceeded) {
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
    if (!walletData.stripe_enabled || rechargeInFlightRef.current) return;
    const finalAmount = customAmount ? parseFloat(customAmount) : rechargeAmount;
    const minimum = walletData.stripe_min_amount || 10;
    if (!Number.isFinite(finalAmount) || finalAmount < minimum || finalAmount > 999999.99 || Math.abs(finalAmount * 100 - Math.round(finalAmount * 100)) > 0.000001) {
      showToast(isZh ? `请输入 ${minimum} 至 999999.99 元的金额，最多两位小数` : `Enter an amount from ${minimum} to 999999.99 with at most two decimal places`, 'warning');
      return;
    }

    rechargeInFlightRef.current = true;
    setRecharging(true);
    try {
      const cents = Math.round(finalAmount * 100);
      let intent;
      try { intent = JSON.parse(sessionStorage.getItem(intentStorageKey) || 'null'); } catch { /* Replace malformed local state. */ }
      if (!intent || typeof intent.key !== 'string' || !/^[A-Za-z0-9_-]{8,128}$/.test(intent.key) || intent.cents !== cents) {
        intent = { key: crypto.randomUUID(), cents };
        sessionStorage.setItem(intentStorageKey, JSON.stringify(intent));
      }
      // Stripe Checkout
      const res = await adminFetch('/api/v1/user/wallet/recharge/stripe/session', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Idempotency-Key': intent.key },
        body: JSON.stringify({ amount: finalAmount })
      });
      const data = await res.json();
      if (data.order_no) sessionStorage.setItem(intentStorageKey, JSON.stringify({ ...intent, order: data.order_no }));
      if (res.ok && data.code === 0 && data.checkout_url) {
        window.location.href = data.checkout_url;
      } else if (res.ok && data.code === 0 && data.status) {
        clearCheckoutIntent();
        await fetchWallet();
        showToast(isZh ? '该订单已处理，请查看最新订单状态。' : 'This order has been processed. Check its current status.', 'info');
      } else {
        showToast(data.error || (isZh ? '在线支付通道暂不可用，请使用额度兑换券进行充值' : 'Online payment unavailable, please use gift cards to redeem balance'), 'error');
      }
    } catch (err) {
      showToast((isZh ? '充值请求异常: ' : 'Recharge error: ') + err.message, 'error');
    } finally {
      rechargeInFlightRef.current = false;
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
      const { data, ok: requestSucceeded } = await apiMutation('/api/v1/admin/redemptions/generate', 'POST', {
          count,
          amount,
          name: genName.trim() || (isZh ? '额度兑换卡' : 'Gift Card')
        }, adminFetch);

      if (requestSucceeded) {
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
      const { data, ok: requestSucceeded } = await apiMutation(`/api/v1/admin/redemptions/${id}`, 'DELETE', undefined, adminFetch);

      if (requestSucceeded) {
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
  return {
    copyText, handleDeleteRedemption, handleGenerateRedemptions, handleRecharge, handleRedeem
  };
}
