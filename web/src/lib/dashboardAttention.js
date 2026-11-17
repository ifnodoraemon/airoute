import { Server, AlertTriangle, AlertCircle, Flame } from 'lucide-react';
import { attentionScopeLabel } from './dashboardAttentionSummary';

export default function buildDashboardAttention({
  adminUser, attention, channels, isZh, onNavigateToErrors, setCurrentTab,
  setShowChannelModal
}) {
  const attentionItems = [];
  const scope = attentionScopeLabel(attention.scope, attention.timeframe, isZh);
  const codeScope = attentionScopeLabel(attention.codeScope, attention.timeframe, isZh);
  if (channels.length === 0) {
    attentionItems.push({
      key: 'no-channels',
      tone: 'danger',
      icon: AlertTriangle,
      title: isZh ? '尚未接入任何服务商' : 'No providers connected',
      detail: isZh ? '接入一个上游渠道后，请求才会开始被路由。' : 'Connect an upstream provider to start routing requests.',
      actionLabel: adminUser?.role === 'admin' && setShowChannelModal ? (isZh ? '接入服务商' : 'Add channel') : null,
      onAction: () => setShowChannelModal?.(true)
    });
  }
  if (attention.tripped > 0) {
    attentionItems.push({
      key: 'breaker',
      tone: 'danger',
      icon: Flame,
      title: isZh ? `${attention.tripped} 个渠道断路器已触发` : `${attention.tripped} channel breaker${attention.tripped > 1 ? 's' : ''} tripped`,
      detail: isZh ? '状态为 OPEN / HALF-OPEN，请求正在被降级或转移。' : 'State is OPEN / HALF-OPEN — traffic is failing over or degraded.',
      actionLabel: isZh ? '查看渠道' : 'View channels',
      onAction: () => setCurrentTab?.('channels')
    });
  }
  if (attention.disabled > 0) {
    attentionItems.push({
      key: 'disabled',
      tone: 'warn',
      icon: Server,
      title: isZh ? `${attention.disabled} 个渠道已停用` : `${attention.disabled} channel${attention.disabled > 1 ? 's' : ''} disabled`,
      detail: isZh ? '状态不是 active，不会参与路由和测速。' : 'Status is not active — excluded from routing and probes.',
      actionLabel: isZh ? '查看渠道' : 'View channels',
      onAction: () => setCurrentTab?.('channels')
    });
  }
  // Error codes, clustered. The histogram was computed and never rendered, so the
  // operator could see that requests failed but not with what status.
  const errorCodeEntries = Object.entries(attention.errorCountByCode)
    .map(([code, count]) => ({ code, count }))
    .sort((a, b) => b.count - a.count);
  if (errorCodeEntries.length > 0) {
    const top = errorCodeEntries.slice(0, 3);
    attentionItems.push({
      key: 'error-codes',
      tone: 'warn',
      icon: AlertTriangle,
      title: isZh
        ? `失败状态码分布：${top.map(e => `${e.code}×${e.count}`).join('、')}`
        : `Failure status codes: ${top.map(e => `${e.code}×${e.count}`).join(', ')}`,
      detail: isZh
        ? `覆盖${codeScope}，共 ${errorCodeEntries.length} 种失败状态码。`
        : `Covers ${codeScope} across ${errorCodeEntries.length} failure status codes.`,
      actionLabel: isZh ? '查看日志' : 'View logs',
      onAction: () => onNavigateToErrors ? onNavigateToErrors() : setCurrentTab?.('logs')
    });
  }

  if (attention.failed > 0) {
    attentionItems.push({
      key: 'failures',
      tone: 'warn',
      icon: AlertCircle,
      title: isZh
        ? `${attention.recentTotal} 条请求中有 ${attention.failed} 条失败`
        : `${attention.failed} of ${attention.recentTotal} requests failed`,
      detail: isZh ? `统计覆盖${scope}，点击查看异常记录。` : `Counts cover ${scope}. View failed requests.`,
      actionLabel: isZh ? '查看日志' : 'View logs',
      onAction: () => onNavigateToErrors ? onNavigateToErrors() : setCurrentTab?.('logs')
    });
  }

  if (attention.rateLimited > 0) {
    const rateScope = attentionScopeLabel(attention.rateLimitScope, attention.timeframe, isZh);
    attentionItems.push({
      key: 'rate-limits', tone: 'warn', icon: AlertTriangle,
      title: isZh ? `${attention.rateLimited} 条请求触发限流` : `${attention.rateLimited} requests rate limited`,
      detail: isZh ? `覆盖${rateScope}的 HTTP 429 响应。` : `HTTP 429 responses across ${rateScope}.`,
      actionLabel: isZh ? '查看日志' : 'View logs',
      onAction: () => onNavigateToErrors ? onNavigateToErrors() : setCurrentTab?.('logs'),
    });
  }

  const attentionTones = {
    danger: {
      card: 'border-rose-200/80 dark:border-rose-900/60 border-l-rose-500 bg-rose-50/50 dark:bg-rose-950/20',
      icon: 'text-rose-500 dark:text-rose-400',
      title: 'text-rose-900 dark:text-rose-100'
    },
    warn: {
      card: 'border-amber-200/80 dark:border-amber-900/60 border-l-amber-500 bg-amber-50/50 dark:bg-amber-950/20',
      icon: 'text-amber-500 dark:text-amber-400',
      title: 'text-amber-900 dark:text-amber-100'
    }
  };
  return {
    attentionItems, attentionTones
  };
}
