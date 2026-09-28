import React, { useState, useEffect } from 'react';
import {
  Database,
  Layers,
  Activity,
  Shield,
  DollarSign,
  Mail,
  RefreshCw,
  CheckCircle2,
  AlertTriangle,
  Server,
  Zap,
  Info
} from 'lucide-react';

export default function MiddlewareStatusMatrix({ adminFetch, showToast }) {
  const [components, setComponents] = useState([]);
  const [loading, setLoading] = useState(false);
  const [lastCheckTime, setLastCheckTime] = useState(null);

  const fetchStatus = async () => {
    if (!adminFetch) return;
    setLoading(true);
    try {
      const res = await adminFetch('/api/v1/admin/system/middlewares');
      const data = await res.json();
      if (res.ok && data.code === 0 && Array.isArray(data.data)) {
        setComponents(data.data);
        setLastCheckTime(new Date());
      }
    } catch (err) {
      console.error('Failed to fetch middleware status:', err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchStatus();
    const timer = setInterval(fetchStatus, 15000);
    return () => clearInterval(timer);
  }, []);

  const getCategoryIcon = (category) => {
    switch (category) {
      case '数据存储':
        return <Database className="w-4 h-4 text-sky-600" />;
      case '缓存与集群协调':
      case '缓存与状态机':
        return <Layers className="w-4 h-4 text-indigo-600" />;
      case '消息流管道':
      case '消息管道与缓冲':
        return <Activity className="w-4 h-4 text-emerald-600" />;
      case '可靠性与路由':
        return <Shield className="w-4 h-4 text-purple-600" />;
      case '计量与计费':
        return <DollarSign className="w-4 h-4 text-amber-600" />;
      case '通知与认证':
        return <Mail className="w-4 h-4 text-teal-600" />;
      default:
        return <Server className="w-4 h-4 text-slate-600" />;
    }
  };

  return (
    <div className="bg-white border border-slate-200/80 rounded-3xl p-6 shadow-xs space-y-4">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-4 border-b border-slate-100">
        <div className="flex items-center space-x-3">
          <div className="w-10 h-10 rounded-2xl bg-indigo-50 border border-indigo-100 flex items-center justify-center text-indigo-600 shrink-0">
            <Server className="w-5 h-5" />
          </div>
          <div>
            <div className="flex items-center space-x-2">
              <h3 className="font-bold text-sm text-slate-900">核心中间件与基础设施矩阵</h3>
              <span className="inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-semibold bg-emerald-50 text-emerald-700 border border-emerald-200">
                <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 mr-1.5 animate-pulse"></span>
                全组件协同在线
              </span>
            </div>
          </div>
        </div>

        <div className="flex items-center space-x-2">
          {lastCheckTime && (
            <span className="text-[11px] text-slate-400 font-mono hidden md:inline">
              最近体检: {lastCheckTime.toLocaleTimeString()}
            </span>
          )}
          <button
            onClick={() => {
              fetchStatus();
              if (showToast) showToast('中间件运行状态已刷新', 'info');
            }}
            disabled={loading}
            className="px-3 py-1.5 rounded-xl bg-slate-100 hover:bg-slate-200 text-xs font-semibold text-slate-700 transition flex items-center space-x-1.5 cursor-pointer disabled:opacity-50"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin text-indigo-600' : ''}`} />
            <span>实时诊断</span>
          </button>
        </div>
      </div>

      {/* Grid of Middlewares */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3.5">
        {components.map((item) => {
          const isOperational = item.status === 'operational';
          return (
            <div
              key={item.id}
              className="p-4 rounded-2xl bg-slate-50/70 border border-slate-200/70 hover:bg-white hover:border-indigo-300 hover:shadow-xs transition duration-150 flex flex-col justify-between space-y-3"
            >
              <div className="flex items-start justify-between gap-2">
                <div className="flex items-center space-x-2.5">
                  <div className="p-2 rounded-xl bg-white border border-slate-200/80 shadow-2xs shrink-0">
                    {getCategoryIcon(item.category)}
                  </div>
                  <div>
                    <h4 className="text-xs font-bold text-slate-900 leading-tight">{item.name}</h4>
                    <span className="text-[10px] text-slate-400 font-medium">{item.category}</span>
                  </div>
                </div>

                <span
                  className={`inline-flex items-center px-2 py-0.5 rounded-lg text-[10px] font-semibold shrink-0 ${
                    isOperational
                      ? 'bg-emerald-50 text-emerald-700 border border-emerald-200'
                      : 'bg-amber-50 text-amber-700 border border-amber-200'
                  }`}
                >
                  <span
                    className={`w-1.5 h-1.5 rounded-full mr-1 ${
                      isOperational ? 'bg-emerald-500' : 'bg-amber-500'
                    }`}
                  ></span>
                  {isOperational ? '健康运转' : '告警降级'}
                </span>
              </div>

              <div className="space-y-1.5 pt-1 border-t border-slate-200/50">
                <div className="flex items-center justify-between text-[11px]">
                  <span className="text-slate-500">运行模式:</span>
                  <span className="font-semibold text-slate-800 font-mono truncate max-w-[190px] text-right" title={item.mode}>
                    {item.mode}
                  </span>
                </div>
                {item.latency_ms !== undefined && item.latency_ms > 0 && (
                  <div className="flex items-center justify-between text-[11px]">
                    <span className="text-slate-500">探针延迟:</span>
                    <span className="font-mono text-emerald-600 font-bold">
                      {item.latency_ms.toFixed(2)} ms
                    </span>
                  </div>
                )}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
