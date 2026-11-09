import { Check, Copy, X, AlertCircle, FileCode, Lock } from 'lucide-react';

export default function McpConfigModal({
  activeConfigClient, configToken, copiedKey, getClaudeConfig, getClineConfig, getCurlSnippet,
  getCursorConfig, getPythonSnippet, handleCopyText, isZh, selectedServerForConfig, setActiveConfigClient,
  setConfigModalOpen, setConfigToken
}) {
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm"
      onClick={(e) => {
        if (e.target === e.currentTarget) setConfigModalOpen(false);
      }}
    >
      <div className="bg-white dark:bg-[#191B1F] rounded-xl shadow-2xl border border-[#E4E6E9] dark:border-[#2C3038] max-w-2xl w-full p-6 space-y-4 max-h-[90vh] overflow-y-auto">
        <div className="flex items-center justify-between pb-3 border-b border-[#E4E6E9] dark:border-[#2C3038]">
          <div className="flex items-center gap-2.5">
            <div className="p-2 bg-blue-50 dark:bg-blue-950/50 text-[#355CFF] dark:text-[#91A7FF] rounded-lg">
              <FileCode className="w-5 h-5" />
            </div>
            <div>
              <h3 className="text-base font-bold text-gray-900 dark:text-white">
                {isZh ? '客户端连接配置指南' : 'Client Connection Guide'}
              </h3>
              <p className="text-xs text-gray-500 dark:text-gray-400">
                {selectedServerForConfig
                  ? (isZh ? `针对 [${selectedServerForConfig.name}] 的专属配置` : `Configuration for [${selectedServerForConfig.name}]`)
                  : (isZh ? '将 Airoute 统一 MCP 网关导入您的 AI 客户端' : 'Integrate Airoute unified MCP gateway into your AI clients')}
              </p>
            </div>
          </div>
          <button
            onClick={() => setConfigModalOpen(false)}
            className="p-1.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 rounded-lg"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* 注入鉴权 Token 凭证栏 */}
        <div className="p-3 bg-[#F0F1F2] dark:bg-[#22252B] rounded-xl border border-[#E4E6E9] dark:border-[#2C3038] flex flex-col sm:flex-row sm:flex-wrap sm:items-center justify-between gap-2.5">
          <div className="flex items-center gap-2">
            <Lock className="w-4 h-4 text-[#355CFF] dark:text-[#91A7FF]" />
            <div>
              <span className="text-xs font-semibold text-[#202124] dark:text-[#F1F2F4]">
                {isZh ? '鉴权凭证注入 (Authorization Token / API Key)' : 'Auth Credential Injection (Token / API Key)'}
              </span>
              <span className="hidden sm:inline text-xs text-[#656B75] dark:text-[#A1A7B0] ml-2">
                {isZh ? '(自动写入客户端配置文件与请求头)' : '(Injected into client configs & headers)'}
              </span>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <input
              type="text"
              value={configToken}
              onChange={(e) => setConfigToken(e.target.value)}
              placeholder="YOUR_AIROUTE_KEY"
              className="px-2.5 py-1 text-xs font-mono rounded-sm border border-[#E4E6E9] dark:border-[#2C3038] bg-white dark:bg-[#191B1F] text-[#202124] dark:text-[#F1F2F4] min-w-[200px]"
            />
          </div>
          <p className="basis-full text-xs text-[#656B75] dark:text-[#A1A7B0]">
            {isZh
              ? '列表里的 ak_… 是密钥 ID，只用于日志与工单识别，不能用于调用；明文仅在创建密钥时显示一次，遗失请新建一把。'
              : 'An ak_… value in the list is a key ID: it identifies the key in logs and tickets but cannot be used to call. The plaintext is shown once at creation — create a new key if it was lost.'}
          </p>
        </div>

        {/* 客户端选择 Tabs */}
        <div className="flex items-center gap-2 border-b border-[#E4E6E9] dark:border-[#2C3038] pb-2 overflow-x-auto">
          {[
            { id: 'cursor', label: 'Cursor (.cursor/mcp.json)' },
            { id: 'claude', label: 'Claude Desktop' },
            { id: 'cline', label: 'Cline / Roo-Code' },
            { id: 'python', label: 'Python SDK' },
            { id: 'curl', label: isZh ? 'cURL / 终端 CLI' : 'cURL / Terminal CLI' }
          ].map(c => (
            <button
              key={c.id}
              onClick={() => setActiveConfigClient(c.id)}
              className={`px-3 py-1.5 text-xs font-medium rounded-lg whitespace-nowrap transition-colors ${activeConfigClient === c.id ? 'bg-[#355CFF] text-white shadow-2xs' : 'text-[#656B75] dark:text-[#A1A7B0] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B]'}`}
            >
              {c.label}
            </button>
          ))}
        </div>

        {/* 配置代码预览与一键复制 */}
        <div>
          <div className="flex items-center justify-between mb-2">
            <span className="text-xs font-medium text-[#202124] dark:text-[#F1F2F4]">
              {activeConfigClient === 'cursor' && (isZh ? '配置文件位置: 项目根目录 .cursor/mcp.json' : 'Config path: project root .cursor/mcp.json')}
              {activeConfigClient === 'claude' && (isZh ? '配置文件位置: ~/Library/Application Support/Claude/claude_desktop_config.json' : 'Config path: ~/Library/Application Support/Claude/claude_desktop_config.json')}
              {activeConfigClient === 'cline' && (isZh ? '配置文件位置: Cline Settings -> MCP Servers' : 'Config path: Cline Settings -> MCP Servers')}
              {activeConfigClient === 'python' && (isZh ? '安装依赖: pip install mcp httpx' : 'Dependencies: pip install mcp httpx')}
              {activeConfigClient === 'curl' && (isZh ? '终端命令行测试 (自动携带 Authorization Bearer 鉴权头)' : 'Terminal CLI test (auto-includes Authorization Bearer header)')}
            </span>
            <button
              onClick={() => {
                const code = activeConfigClient === 'cursor' ? getCursorConfig() :
                             activeConfigClient === 'claude' ? getClaudeConfig() :
                             activeConfigClient === 'cline' ? getClineConfig() :
                             activeConfigClient === 'python' ? getPythonSnippet() : getCurlSnippet();
                handleCopyText(code, 'client-config');
              }}
              className="inline-flex items-center gap-1 px-2.5 py-1 text-xs font-medium bg-[#F0F1F2] dark:bg-[#22252B] text-[#202124] dark:text-[#F1F2F4] rounded-sm hover:bg-slate-200 dark:hover:bg-slate-700 transition-colors"
            >
              {copiedKey === 'client-config' ? <Check className="w-3.5 h-3.5 text-emerald-500" /> : <Copy className="w-3.5 h-3.5" />}
              <span>{copiedKey === 'client-config' ? (isZh ? '已复制' : 'Copied') : (isZh ? '复制配置' : 'Copy Config')}</span>
            </button>
          </div>

          <div className="bg-[#111214] text-gray-100 p-4 rounded-xl font-mono text-xs overflow-x-auto border border-[#2C3038]">
            <pre className="whitespace-pre-wrap leading-relaxed">
              {activeConfigClient === 'cursor' && getCursorConfig()}
              {activeConfigClient === 'claude' && getClaudeConfig()}
              {activeConfigClient === 'cline' && getClineConfig()}
              {activeConfigClient === 'python' && getPythonSnippet()}
              {activeConfigClient === 'curl' && getCurlSnippet()}
            </pre>
          </div>
        </div>

        <div className="p-3 bg-amber-50 dark:bg-amber-950/40 border border-amber-200 dark:border-amber-800/50 rounded-lg text-xs text-amber-800 dark:text-amber-300 flex items-start gap-2">
          <AlertCircle className="w-4 h-4 mt-0.5 shrink-0" />
          <div>
            <strong>{isZh ? '安全最佳实践：' : 'Security Best Practice: '}</strong>
            {isZh ? '所有客户端均通过 Airoute 统一协议代理连接，无需直接暴露内部数据库或私有服务凭据。网关会自动执行参数脱敏与 SQL 注入审计。' : 'All clients connect via the unified Airoute protocol proxy without exposing database or private server credentials. Gateway auto-executes redaction and SQL injection auditing.'}
          </div>
        </div>

        <div className="flex justify-end pt-2">
          <button
            onClick={() => setConfigModalOpen(false)}
            className="px-4 py-2 text-xs font-medium text-[#656B75] dark:text-[#A1A7B0] bg-[#F0F1F2] dark:bg-[#22252B] rounded-lg hover:bg-slate-200 dark:hover:bg-slate-700"
          >
            {isZh ? '关闭' : 'Close'}
          </button>
        </div>
      </div>
    </div>
  );
}
