import { Gift, Plus, RefreshCw, Copy, Trash2 } from 'lucide-react';

export default function WalletRedemptionCards({
  copyText, fetchRedemptions, handleDeleteRedemption, isZh, redemptions, redemptionsError, setShowGenModal
}) {
  return (
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
  );
}
