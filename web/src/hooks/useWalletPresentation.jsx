
export default function useWalletPresentation({
  isZh
}) {
  const renderOrderStatus = (status) => {
    const s = String(status || '').toLowerCase();
    const adjustments = {
      refunded: isZh ? '已全额退款' : 'Refunded',
      partially_refunded: isZh ? '已部分退款' : 'Partially refunded',
      refund_pending: isZh ? '退款处理中（额度预留）' : 'Refund pending (funds reserved)',
      disputed: isZh ? '支付争议（额度预留）' : 'Disputed (funds reserved)',
    };
    if (adjustments[s]) return <span className="text-rose-700 dark:text-rose-300 font-semibold">{adjustments[s]}</span>;
    if (s === 'paid') {
      return (
        <span className="px-2 py-0.5 rounded-full text-xs font-semibold whitespace-nowrap bg-emerald-500/10 dark:bg-emerald-500/20 text-emerald-700 dark:text-emerald-300 border border-emerald-500/30">
          {isZh ? '已支付入账' : 'Paid'}
        </span>
      );
    }
    if (s === 'expired') {
      return (
        <span className="px-2 py-0.5 rounded-full text-xs font-semibold whitespace-nowrap bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0] border border-[#E4E6E9] dark:border-[#2C3038]">
          {isZh ? '已过期' : 'Expired'}
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
  return {
    renderOrderStatus
  };
}
