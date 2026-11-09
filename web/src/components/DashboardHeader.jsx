import { Layers, RefreshCw, Plus, FileText } from 'lucide-react';

export default function DashboardHeader({
  adminUser, copiedReport, handleExportExecutiveReport, handleRefresh, isZh, lastRefreshed, onOpenTopology,
  refreshing, setShowChannelModal
}) {
  return (
    <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
      <div>
        <h2 className="ui-page-title">
          {isZh ? '运营总览' : 'Dashboard'}
        </h2>
        <p className="ui-page-subtitle mt-1">
          {isZh
            ? '并发、首字延迟、模态分布与缓存节省'
            : 'Concurrency, TTFT, modality mix and cache savings'}
        </p>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        {onOpenTopology && (
          <button
            onClick={onOpenTopology}
            className="ui-btn-secondary"
            title={isZh ? '打开全链路动态路由与容灾演练' : 'Open visual routing & failover topology'}
          >
            <Layers className="w-3.5 h-3.5 text-[#656B75] dark:text-[#A1A7B0]" />
            <span>{isZh ? '运行时容灾拓扑' : 'Live Routing Topology'}</span>
          </button>
        )}

        {/* The executive report was written but never reachable: no button called
            it, so the generator and its copied-state indicator were both dead. */}
        <button
          onClick={handleExportExecutiveReport}
          className="ui-btn-secondary"
          title={isZh ? '复制一份包含 KPI、SLA 与建议的 Markdown 运营报告' : 'Copy a Markdown operations report with KPIs, SLA and recommendations'}
        >
          <FileText className="w-3.5 h-3.5 text-[#656B75] dark:text-[#A1A7B0]" />
          <span>
            {copiedReport
              ? (isZh ? '已复制报告' : 'Report copied')
              : (isZh ? '导出运营报告' : 'Export Report')}
          </span>
        </button>

        <div className="flex items-center gap-2">
          <button
            onClick={handleRefresh}
            disabled={refreshing}
            className="ui-btn-secondary"
            title={isZh ? '重新拉取统计、渠道与日志数据' : 'Re-fetch stats, channels and logs'}
          >
            <RefreshCw className={`w-3.5 h-3.5 text-[#656B75] dark:text-[#A1A7B0] ${refreshing ? 'animate-spin' : ''}`} />
            <span>{refreshing ? (isZh ? '刷新中…' : 'Refreshing…') : (isZh ? '刷新' : 'Refresh')}</span>
          </button>
          {lastRefreshed && (
            <span
              className="ui-meta font-mono tabular-nums"
              title={isZh ? '最近一次手动刷新的时间' : 'Time of the last manual refresh'}
            >
              {isZh ? '更新于 ' : 'Updated '}
              {lastRefreshed.toLocaleTimeString([], { hour12: false })}
            </span>
          )}
        </div>

        {adminUser?.role === 'admin' && setShowChannelModal && (
          <button
            onClick={() => setShowChannelModal(true)}
            className="ui-btn-primary"
          >
            <Plus className="w-3.5 h-3.5" />
            <span>{isZh ? '接入服务商' : 'Add Channel'}</span>
          </button>
        )}
      </div>
    </div>
  );
}
