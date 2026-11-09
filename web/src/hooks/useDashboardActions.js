import { useState } from 'react';
import { copyToClipboard } from '../config';

export default function useDashboardActions({
  channels, fetchDashboard, fetchData, fetchLogs, fmtPct, isZh, logs, percentiles, refreshing,
  setLastRefreshed, setRefreshing, showToast, slaAvailability, stats, tokenMetrics, totalRequests
}) {
  const [copiedReport, setCopiedReport] = useState(false);

  const handleExportExecutiveReport = async () => {
    const totalCalls = stats.total_requests || logs.length;
    // No sample means unknown, not a made-up 180 ms in a report an operator may
    // paste into a ticket or send to a customer.
    const avgTtftValue = typeof stats.avg_ttft_ms === 'number' && stats.avg_ttft_ms > 0
      ? stats.avg_ttft_ms
      : null;
    const avgTtftText = avgTtftValue === null
      ? (isZh ? '无样本' : 'n/a')
      : `${avgTtftValue}ms`;
    const report = isZh
      ? `# 📊 Airoute AI 网关集群运营与 SLA 遥测报告

**生成时间**：${new Date().toLocaleString()}
**高可用集群架构**：双活双节点负载均衡 (Active-Active HA)
**整体 SLA 可用性**：${slaAvailability}${slaAvailability === '—' ? '' : '%'} (目标: 99.9%, 基于全部 ${totalRequests} 次请求)

---

## 1. 核心关键指标 (KPIs)
- **累计调用量**：${totalCalls.toLocaleString()} 次
- **Token 总吞吐**：${(stats.total_tokens || 0).toLocaleString()} Tokens (输入占比 ${tokenMetrics.promptPctStr}, 输出占比 ${tokenMetrics.compPctStr})
- **首字时延 SLA (TTFT)**：平均 ${avgTtftText} (P50: ${fmtPct(percentiles.p50)}, P90: ${fmtPct(percentiles.p90)}, P99: ${fmtPct(percentiles.p99)}; 样本 ${percentiles.sampled}/${percentiles.total})
- **接入上游通道**：${channels.length} 个服务商 (活跃: ${channels.filter(c => c.status === 'active').length} 个, 熔断: ${channels.filter(c => ['OPEN', 'HALF-OPEN'].includes(String(c.breaker_status || '').toUpperCase())).length} 个)

## 2. FinOps 成本优化与缓存成效
- **Prompt Caching 命中**：累计命中识别 ${logs.filter(l => l.cached_tokens > 0).length} 次
- **闲时优惠覆盖**：夜间/周末闲时请求享受时段费率折扣，大幅削减峰值推理成本

## 3. 架构优化与行动建议
1. 建议持续保持高频核心模型（如 DeepSeek-R1 / Claude 3.5）的多通道优先级冗余配置。
2. 保持 P99 延迟预警阈值设为 2500ms，配合断路器实现亚秒级故障转移。`
      : `# 📊 Airoute AI Gateway SLA & Telemetry Executive Report

**Generated**: ${new Date().toLocaleString()}
**Cluster Architecture**: Active-Active Dual-Node HA
**SLA Availability**: ${slaAvailability}${slaAvailability === '—' ? '' : '%'} (Target: 99.9%, over all ${totalRequests} requests)

---

## 1. Core KPIs
- **Total Invocations**: ${totalCalls.toLocaleString()}
- **Token Throughput**: ${(stats.total_tokens || 0).toLocaleString()} Tokens (Prompt: ${tokenMetrics.promptPctStr}, Completion: ${tokenMetrics.compPctStr})
- **TTFT SLA Latency**: Avg ${avgTtftText} (P50: ${fmtPct(percentiles.p50)}, P90: ${fmtPct(percentiles.p90)}, P99: ${fmtPct(percentiles.p99)}; ${percentiles.sampled}/${percentiles.total} samples)
- **Upstream Channels**: ${channels.length} providers (${channels.filter(c => c.status === 'active').length} active, ${channels.filter(c => ['OPEN', 'HALF-OPEN'].includes(String(c.breaker_status || '').toUpperCase())).length} tripped)

## 2. FinOps Cost Optimization
- **Prompt Cache Hits**: ${logs.filter(l => l.cached_tokens > 0).length} requests accelerated
- **Off-Peak Economics**: Time-of-use rate optimization during off-peak windows

## 3. Recommendations
1. Maintain redundant fallback routes for top reasoning models.
2. Monitor P99 latency alerts under 2500ms with automated circuit-breaker protection.`;

    await copyToClipboard(report);
    setCopiedReport(true);
    setTimeout(() => setCopiedReport(false), 2000);
    if (showToast) showToast(isZh ? 'AI 运营与遥测大盘报告已复制！' : 'AI Executive Telemetry Report copied!', 'success');
  };

  // Manual refresh. fetchData()/fetchLogs() resolve to a boolean (false = the
  // request failed), so the toast reflects the real outcome instead of firing
  // optimistically before the requests settle.
  const handleRefresh = async () => {
    if (refreshing) return;
    setRefreshing(true);
    try {
      const [a, b, c] = await Promise.all([fetchData?.(), fetchLogs?.(), fetchDashboard()]);
      const ok = a !== false && b !== false && c !== false;
      if (ok) showToast?.(isZh ? '遥测数据已更新' : 'Telemetry refreshed', 'success');
      else showToast?.(isZh ? '部分数据刷新失败，请检查网络或后端服务' : 'Some data failed to refresh — check the network or backend', 'error');
    } catch (e) {
      showToast?.((isZh ? '刷新失败: ' : 'Refresh failed: ') + (e?.message || ''), 'error');
    } finally {
      setLastRefreshed(new Date());
      setRefreshing(false);
    }
  };
  return {
    copiedReport, handleExportExecutiveReport, handleRefresh
  };
}
