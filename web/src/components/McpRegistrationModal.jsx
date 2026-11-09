import { Plus, X } from 'lucide-react';

export default function McpRegistrationModal({
  editingServerId, handleCreateServer, isZh, newServer, savingServer, setAddServerModalOpen, setNewServer
}) {
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm"
      onClick={(e) => {
        if (e.target === e.currentTarget) setAddServerModalOpen(false);
      }}
    >
      <div className="bg-white dark:bg-[#191B1F] rounded-xl shadow-2xl border border-[#E4E6E9] dark:border-[#2C3038] max-w-xl w-full p-6 space-y-4 max-h-[90vh] overflow-y-auto">
        <div className="flex items-center justify-between pb-3 border-b border-[#E4E6E9] dark:border-[#2C3038]">
          <div className="flex items-center gap-2.5">
            <div className="p-2 bg-[#355CFF] text-white rounded-lg shadow-2xs">
              <Plus className="w-5 h-5" />
            </div>
            <div>
              <h3 className="text-base font-bold text-gray-900 dark:text-white">
                {isZh ? '接入新 MCP 服务' : 'Connect New MCP Server'}
              </h3>
              <p className="text-xs text-gray-500 dark:text-gray-400">
                {isZh ? '遵循 ModelScope / Anthropic 规范标准接入自定义协议服务' : 'Connect custom protocol servers adhering to ModelScope / Anthropic standards'}
              </p>
            </div>
          </div>
          <button
            onClick={() => setAddServerModalOpen(false)}
            className="p-1.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 rounded-lg"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        <form onSubmit={handleCreateServer} className="space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            <div>
              <label className="ui-label">
                {isZh ? '服务唯一标识 (ID) *' : 'Server Unique ID *'}
              </label>
              <input
                type="text"
                required
                placeholder={isZh ? "如: docker-mcp" : "e.g. docker-mcp"}
                readOnly={!!editingServerId}
                value={newServer.id}
                onChange={(e) => setNewServer({ ...newServer, id: e.target.value })}
                className="ui-field bg-gray-50 dark:bg-gray-900"
              />
            </div>
            <div>
              <label className="ui-label">
                {isZh ? '服务显示名称 *' : 'Server Display Name *'}
              </label>
              <input
                type="text"
                required
                placeholder={isZh ? '如: Docker 容器管理服务' : 'e.g. Docker Container Service'}
                value={newServer.name}
                onChange={(e) => setNewServer({ ...newServer, name: e.target.value })}
                className="ui-field bg-gray-50 dark:bg-gray-900"
              />
            </div>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            <div>
              <label className="ui-label">
                {isZh ? '分类领域' : 'Category'}
              </label>
              <select
                value={newServer.category}
                onChange={(e) => setNewServer({ ...newServer, category: e.target.value })}
                className="ui-field bg-gray-50 dark:bg-gray-900"
              >
                <option value="ops">{isZh ? '运维网关 (ops)' : 'Ops Gateway (ops)'}</option>
                <option value="dev">{isZh ? '研发协作 (dev)' : 'Dev Collaboration (dev)'}</option>
                <option value="search">{isZh ? '搜索抓取 (search)' : 'Search & Scrape (search)'}</option>
                <option value="db">{isZh ? '数据存储 (db)' : 'Data & Storage (db)'}</option>
                <option value="ai">{isZh ? '深度推理 (ai)' : 'Deep Reasoning (ai)'}</option>
                <option value="productivity">{isZh ? '企业协同 (productivity)' : 'Enterprise Collab (productivity)'}</option>
              </select>
            </div>
            <div>
              <label className="ui-label">
                {isZh ? '传输通道 (Transport)' : 'Transport Channel'}
              </label>
              <select
                value={newServer.transport}
                onChange={(e) => setNewServer({ ...newServer, transport: e.target.value })}
                className="ui-field bg-gray-50 dark:bg-gray-900"
              >
                <option value="stdio">{isZh ? 'stdio (本地命令/子进程)' : 'stdio (Local Process)'}</option>
                <option value="sse">{isZh ? 'sse (远程 HTTP Server-Sent Events)' : 'sse (Remote HTTP SSE)'}</option>
                <option value="http">http (Streamable HTTP)</option>
              </select>
            </div>
          </div>

          <div>
            <label className="ui-label">
              {isZh ? '服务 Endpoint / 执行命令 *' : 'Service Endpoint / Command *'}
            </label>
            <input
              type="text"
              required
              placeholder={isZh ? '如: npx -y @modelcontextprotocol/server-docker 或 https://mcp.example.com/sse' : 'e.g. npx -y @modelcontextprotocol/server-docker or https://mcp.example.com/sse'}
              value={newServer.endpoint}
              onChange={(e) => setNewServer({ ...newServer, endpoint: e.target.value })}
              className="ui-field bg-gray-50 dark:bg-gray-900"
            />
          </div>

          <div>
            <label className="ui-label">
              {isZh ? '服务描述' : 'Description'}
            </label>
            <textarea
              rows="2"
              placeholder={isZh ? '简述该 MCP 服务的功能、适用场景及接入说明...' : 'Describe MCP server capabilities, scenario, and instructions...'}
              value={newServer.description}
              onChange={(e) => setNewServer({ ...newServer, description: e.target.value })}
              className="ui-field bg-gray-50 dark:bg-gray-900"
            />
          </div>

          <div>
            <label className="ui-label">
              {isZh ? '暴露的 Tools 工具集 (逗号分隔)' : 'Exposed Tools (comma separated)'}
            </label>
            <input
              type="text"
              placeholder={isZh ? "如: docker_ps, docker_logs, docker_restart" : "e.g. docker_ps, docker_logs, docker_restart"}
              value={newServer.tools}
              onChange={(e) => setNewServer({ ...newServer, tools: e.target.value })}
              className="ui-field bg-gray-50 dark:bg-gray-900 font-mono"
            />
          </div>

          <label className="block space-y-1 text-xs">
            <span>{isZh ? '进程环境变量 (JSON)' : 'Process environment (JSON)'}</span>
            <textarea rows={3} value={newServer.env_vars} onChange={e => setNewServer({ ...newServer, env_vars: e.target.value })} className="ui-field font-mono" placeholder='{"SERVICE_TOKEN":"..."}' />
          </label>
          <div className="flex items-center justify-end gap-2.5 pt-3 border-t border-gray-200 dark:border-gray-800">
            <button
              type="button"
              onClick={() => setAddServerModalOpen(false)}
              className="px-4 py-2 text-xs font-medium text-gray-700 dark:text-gray-300 bg-gray-100 dark:bg-gray-800 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-700"
            >
              {isZh ? '取消' : 'Cancel'}
            </button>
            <button
              type="submit"
              disabled={savingServer}
              className="px-4 py-2 text-xs font-medium text-white bg-[#355CFF] rounded-lg hover:bg-blue-600 shadow-sm"
            >
              {savingServer ? (isZh ? '保存中...' : 'Saving...') : (isZh ? '保存服务' : 'Save server')}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
