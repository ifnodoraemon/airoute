import { X, Send, Radio, BookOpen, Lock } from 'lucide-react';
import McpProbeSelection from './McpProbeSelection';
import McpProbeArguments from './McpProbeArguments';
import McpProbeTrace from './McpProbeTrace';

export default function McpProbeModal({
  TOOL_TEMPLATES, copiedKey, getCurlRequestText, getCurrentRpcPayload, getGroupLabel, getToolInfo,
  getWireRequestText, handleCopyText, handleRunProbe, handleSelectTool, isZh, lastSentPayload, messagesUrl,
  probeFailed, probeHttpStatus, probeLatency, probeMethod, probeResult, probeRunning, probeServer, probeToken,
  probeToolArgs, probeToolName, requestViewTab, servers, setProbeMethod, setProbeModalOpen, setProbeServer,
  setProbeToken, setProbeToolArgs, setRequestViewTab
}) {
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm"
      onClick={(e) => {
        if (e.target === e.currentTarget) setProbeModalOpen(false);
      }}
    >
      <div className="bg-white dark:bg-[#191B1F] rounded-xl shadow-2xl border border-[#E4E6E9] dark:border-[#2C3038] max-w-5xl w-full p-6 space-y-4 max-h-[90vh] overflow-y-auto">
        <div className="flex items-center justify-between pb-3 border-b border-[#E4E6E9] dark:border-[#2C3038]">
          <div className="flex items-center gap-2.5">
            <div className="p-2 bg-[#355CFF] text-white rounded-lg shadow-2xs">
              <Radio className="w-5 h-5" />
            </div>
            <div>
              <h3 className="text-base font-bold text-[#202124] dark:text-[#F1F2F4] flex items-center gap-2">
                {isZh ? '在线协议探针控制台' : 'Online Protocol Probe Console'}
                <span className="text-xs font-mono font-normal text-[#355CFF] dark:text-[#91A7FF]">JSON-RPC 2.0</span>
              </h3>
              <p className="text-xs text-[#656B75] dark:text-[#A1A7B0]">
                {isZh ? `向网关端点 ${messagesUrl} 发送标准协议报文并实时验证安全拦截与工具输出` : `Send standard JSON-RPC 2.0 messages to ${messagesUrl} to verify security interception & output`}
              </p>
            </div>
          </div>
          <button
            onClick={() => setProbeModalOpen(false)}
            className="p-1.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 rounded-lg"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* 协议教程与通信原理解析 */}
        <div className="p-3 bg-blue-50/60 dark:bg-[#242D48]/50 border border-blue-200/50 dark:border-blue-900/40 rounded-xl text-xs text-blue-950 dark:text-[#91A7FF]">
          <div className="flex items-center gap-2 font-semibold mb-1 text-[#355CFF] dark:text-[#91A7FF]">
            <BookOpen className="w-4 h-4" />
            <span>{isZh ? 'MCP 双向通信原理解析与接入教程' : 'MCP Bi-directional Architecture & Tutorial'}</span>
          </div>
          <p className="text-xs leading-relaxed text-[#656B75] dark:text-[#A1A7B0]">
            {isZh ? (
              <><b>标准交互流:</b> 客户端首先发起 <code className="font-mono bg-blue-100 dark:bg-blue-950/80 px-1 py-0.5 rounded-sm">GET /mcp/sse</code> 建立长连接握手；随后将左侧的 <b>JSON-RPC 2.0 请求报文</b> 通过 <code className="font-mono bg-blue-100 dark:bg-blue-950/80 px-1 py-0.5 rounded-sm">POST /mcp/messages</code> 投递至网关。网关安全审计并调度工具后，将右侧的 <b>响应结果</b> 实时返回给客户端。</>
            ) : (
              <><b>Standard Flow:</b> Clients initiate <code className="font-mono bg-blue-100 dark:bg-blue-950/80 px-1 py-0.5 rounded-sm">GET /mcp/sse</code> handshake, then send <b>JSON-RPC 2.0 request</b> via <code className="font-mono bg-blue-100 dark:bg-blue-950/80 px-1 py-0.5 rounded-sm">POST /mcp/messages</code>. Gateway audits security and dispatches tools, returning <b>responses</b> in real-time.</>
            )}
          </p>
        </div>

        {/* 鉴权 Token 凭证与目标服务配置栏 */}
        <div className="p-3 bg-[#F0F1F2] dark:bg-[#22252B] rounded-xl border border-[#E4E6E9] dark:border-[#2C3038] space-y-2.5">
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
            <div className="flex items-center gap-2">
              <Lock className="w-4 h-4 text-[#355CFF] dark:text-[#91A7FF]" />
              <span className="text-xs font-semibold text-[#202124] dark:text-[#F1F2F4]">
                {isZh ? '鉴权凭证 (Authorization Token / 虚拟 API Key)' : 'Authentication Token / Virtual API Key'}
              </span>
              <span className="text-xs text-[#656B75] dark:text-[#A1A7B0]">
                {isZh ? '(支持 Bearer Token 与 X-API-Key 多租户隔离)' : '(Supports Bearer Token & multi-tenant isolation)'}
              </span>
            </div>
          </div>
          <input
            type="text"
            value={probeToken}
            onChange={(e) => setProbeToken(e.target.value)}
            placeholder={isZh ? '输入 API Key 明文 (如 sk-airoute-...)' : 'Enter plaintext API Key (e.g., sk-airoute-...)'}
            className="w-full px-3 py-1.5 font-mono text-xs rounded-lg border border-[#E4E6E9] dark:border-[#2C3038] bg-white dark:bg-[#191B1F] text-[#202124] dark:text-[#F1F2F4] placeholder-[#656B75] dark:placeholder-[#A1A7B0] focus:outline-none focus:ring-1 focus:ring-[#355CFF]"
          />
          <p className="text-xs text-[#656B75] dark:text-[#A1A7B0]">
            {isZh
              ? '列表里的 ak_… 是密钥 ID，只用于日志与工单识别，不能用于调用；明文仅在创建密钥时显示一次，遗失请新建一把。'
              : 'An ak_… value in the list is a key ID: it identifies the key in logs and tickets but cannot be used to call. The plaintext is shown once at creation — create a new key if it was lost.'}
          </p>
        </div>

        {/* 方法、服务与参数选择 */}
        <div className="space-y-3">
          <McpProbeSelection {...{
            TOOL_TEMPLATES, getGroupLabel, getToolInfo, handleSelectTool, isZh, probeMethod, probeServer, probeToolName,
            servers, setProbeMethod, setProbeServer
          }} />

          {probeMethod === 'tools/call' && (
            <McpProbeArguments {...{
              handleSelectTool, isZh, probeToolArgs, setProbeToolArgs
            }} />
          )}

          <div className="flex items-center justify-between pt-1">
            <span className="text-xs text-[#656B75] dark:text-[#A1A7B0]">
              {probeLatency !== null && (
                <span className={probeFailed ? 'text-rose-500 font-mono' : 'text-emerald-500 font-mono'}>
                  {probeFailed ? '✗' : '✓'} {isZh ? `往返耗时: ${probeLatency} ms` : `Roundtrip: ${probeLatency} ms`}
                </span>
              )}
            </span>
            <button
              onClick={handleRunProbe}
              disabled={probeRunning}
              className="inline-flex items-center gap-1.5 px-4 py-2 text-xs font-semibold text-white bg-[#355CFF] rounded-lg hover:bg-blue-600 disabled:opacity-50 transition-colors shadow-sm"
            >
              <Send className={`w-3.5 h-3.5 ${probeRunning ? 'animate-pulse' : ''}`} />
              <span>{probeRunning ? (isZh ? '探针执行中...' : 'Probing...') : (isZh ? '发送 JSON-RPC 2.0 报文' : 'Send JSON-RPC 2.0 Request')}</span>
            </button>
          </div>

          {/* 双向报文链路对比: 发送报文 (左) 与 响应结果 (右) */}
          <McpProbeTrace {...{
            copiedKey, getCurlRequestText, getCurrentRpcPayload, getWireRequestText, handleCopyText, isZh,
            lastSentPayload, messagesUrl, probeFailed, probeHttpStatus, probeLatency, probeResult, probeToken,
            requestViewTab, setRequestViewTab
          }} />
        </div>

        <div className="flex justify-end pt-2 border-t border-gray-100 dark:border-gray-800">
          <button
            onClick={() => setProbeModalOpen(false)}
            className="px-4 py-2 text-xs font-medium text-gray-700 dark:text-gray-300 bg-gray-100 dark:bg-gray-800 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-700"
          >
            {isZh ? '关闭' : 'Close'}
          </button>
        </div>
      </div>
    </div>
  );
}
