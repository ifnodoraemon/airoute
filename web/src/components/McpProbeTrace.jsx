import { Terminal, Check, Copy, Send, CheckCircle2 } from 'lucide-react';

export default function McpProbeTrace({
  copiedKey, getCurlRequestText, getCurrentRpcPayload, getWireRequestText, handleCopyText, isZh,
  lastSentPayload, messagesUrl, probeFailed, probeHttpStatus, probeLatency, probeResult, probeToken,
  requestViewTab, setRequestViewTab
}) {
  return (
    <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 pt-2">
      {/* 1. 发送请求报文 */}
      <div className="flex flex-col space-y-1.5">
        <div className="flex items-center justify-between text-xs font-semibold text-[#202124] dark:text-[#F1F2F4]">
          <div className="flex items-center gap-1.5">
            <span className="flex items-center gap-1 text-[#355CFF] dark:text-[#91A7FF]">
              <Send className="w-3.5 h-3.5" />
              {isZh ? '发送请求报文' : 'Request Payload'}
            </span>
            <div className="flex items-center bg-[#F0F1F2] dark:bg-[#22252B] rounded-lg p-0.5 ml-1 text-xs">
              <button
                type="button"
                onClick={() => setRequestViewTab('wire')}
                className={`px-1.5 py-0.5 rounded-sm ${requestViewTab === 'wire' ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-xs font-medium' : 'text-[#656B75] hover:text-[#202124] dark:hover:text-[#F1F2F4]'}`}
              >
                {isZh ? 'HTTP报文' : 'Wire'}
              </button>
              <button
                type="button"
                onClick={() => setRequestViewTab('json')}
                className={`px-1.5 py-0.5 rounded-sm ${requestViewTab === 'json' ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-xs font-medium' : 'text-[#656B75] hover:text-[#202124] dark:hover:text-[#F1F2F4]'}`}
              >
                JSON-RPC
              </button>
              <button
                type="button"
                onClick={() => setRequestViewTab('curl')}
                className={`px-1.5 py-0.5 rounded-sm ${requestViewTab === 'curl' ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-xs font-medium' : 'text-[#656B75] hover:text-[#202124] dark:hover:text-[#F1F2F4]'}`}
              >
                cURL
              </button>
            </div>
          </div>
          <button
            type="button"
            onClick={() => {
              const txt = requestViewTab === 'wire' ? getWireRequestText() :
                          requestViewTab === 'json' ? JSON.stringify(lastSentPayload || getCurrentRpcPayload(), null, 2) :
                          getCurlRequestText();
              handleCopyText(txt, 'probe-req');
            }}
            className="inline-flex items-center gap-1 text-xs font-normal text-[#656B75] hover:text-[#355CFF] dark:hover:text-[#91A7FF] transition-colors"
          >
            {copiedKey === 'probe-req' ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
            {isZh ? '复制请求' : 'Copy Request'}
          </button>
        </div>
        <div className="bg-[#111214] text-gray-100 p-3.5 rounded-xl font-mono text-xs overflow-x-auto min-h-[170px] max-h-64 border border-[#2C3038] inset-shadow-sm">
          <pre className="whitespace-pre-wrap leading-relaxed">
            {requestViewTab === 'wire' && getWireRequestText()}
            {requestViewTab === 'json' && JSON.stringify(lastSentPayload || getCurrentRpcPayload(), null, 2)}
            {requestViewTab === 'curl' && getCurlRequestText()}
          </pre>
        </div>
        <div className="text-xs text-[#656B75] dark:text-[#A1A7B0] flex items-center justify-between">
          <span>{isZh ? `传输端点: POST ${messagesUrl}` : `Endpoint: POST ${messagesUrl}`}</span>
          <span>{isZh ? (probeToken ? '鉴权: Bearer Token (已配置)' : '鉴权: 未配置 Token') : (probeToken ? 'Auth: Bearer Token' : 'Auth: None')}</span>
        </div>
      </div>

      {/* 2. 网关接收响应 */}
      <div className="flex flex-col space-y-1.5">
        <div className="flex items-center justify-between text-xs font-semibold text-[#202124] dark:text-[#F1F2F4]">
          <span className="flex items-center gap-1.5 text-emerald-600 dark:text-emerald-400">
            <CheckCircle2 className="w-3.5 h-3.5" />
            {isZh ? `接收响应 (${probeHttpStatus ? `HTTP ${probeHttpStatus}` : '等待请求'})` : `Response Payload (${probeHttpStatus ? `HTTP ${probeHttpStatus}` : 'Pending'})`}
          </span>
          {probeResult && (
            <button
              type="button"
              onClick={() => handleCopyText(JSON.stringify(probeResult, null, 2), 'probe-res')}
              className="inline-flex items-center gap-1 text-xs font-normal text-[#656B75] hover:text-emerald-600 dark:hover:text-emerald-400 transition-colors"
            >
              {copiedKey === 'probe-res' ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
              {isZh ? '复制响应' : 'Copy Response'}
            </button>
          )}
        </div>
        <div className="bg-[#111214] text-gray-100 p-3.5 rounded-xl font-mono text-xs overflow-x-auto min-h-[170px] max-h-64 border border-[#2C3038] inset-shadow-sm flex flex-col justify-start">
          {probeResult ? (
            <pre className="whitespace-pre-wrap leading-relaxed">{JSON.stringify(probeResult, null, 2)}</pre>
          ) : (
            <div className="h-full flex flex-col items-center justify-center text-gray-500 text-center py-8">
              <Terminal className="w-8 h-8 mb-2 opacity-40 text-[#355CFF]" />
              <p className="text-xs font-medium text-gray-400">{isZh ? '等待发送探针' : 'Awaiting Probe Execution'}</p>
              <p className="text-xs text-gray-400 mt-1">{isZh ? '点击上方“发送 JSON-RPC 2.0 报文”发起实时调用测试' : 'Click "Send JSON-RPC 2.0 Request" above to test live execution'}</p>
            </div>
          )}
        </div>
        <div className="text-xs text-slate-500 dark:text-slate-400 flex items-center justify-between">
          <span>{isZh ? `网关延迟: ${probeLatency !== null ? `${probeLatency} ms` : '--'}` : `Latency: ${probeLatency !== null ? `${probeLatency} ms` : '--'}`}</span>
          <span>{isZh ? `状态: ${probeResult ? (probeFailed ? '调用失败' : '成功返回') : '待调用'}` : `Status: ${probeResult ? (probeFailed ? 'Failed' : 'Success') : 'Pending'}`}</span>
        </div>
      </div>
    </div>
  );
}
