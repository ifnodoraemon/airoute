import { copyToClipboard } from '../config';
import { Server, Globe, Layers, Database, Code2, Sparkles, Boxes } from 'lucide-react';
import { buildMCPClientConfig } from '../lib/mcpClientConfig';

export default function useMcpPresentation({
  configToken, isZh, messagesUrl, onCopy, searchQuery, selectedCategory, selectedServerForConfig, servers,
  setCopiedKey, showToast, sseUrl
}) {
  const handleCopyText = async (text, key) => {
    if (onCopy) {
      onCopy(text);
    } else {
      await copyToClipboard(text);
    }
    setCopiedKey(key);
    setTimeout(() => setCopiedKey(''), 2000);
    if (showToast) showToast(isZh ? '内容已复制到剪贴板' : 'Copied to clipboard', 'success');
  };

  const categories = [
    { id: 'all', name: isZh ? '全部服务' : 'All Services', icon: Boxes },
    { id: 'ops', name: isZh ? '网关原生' : 'Gateway Native', icon: Server },
    { id: 'dev', name: isZh ? '研发协作' : 'Dev Collab', icon: Code2 },
    { id: 'search', name: isZh ? '搜索抓取' : 'Search & Scrape', icon: Globe },
    { id: 'db', name: isZh ? '数据存储' : 'Data & Storage', icon: Database },
    { id: 'ai', name: isZh ? '深度推理' : 'Deep Reasoning', icon: Sparkles },
    { id: 'productivity', name: isZh ? '企业协同' : 'Productivity', icon: Layers }
  ];

  const filteredServers = servers.filter(srv => {
    const matchCategory = selectedCategory === 'all' || srv.category === selectedCategory;
    const matchSearch = !searchQuery ||
      srv.name?.toLowerCase().includes(searchQuery.toLowerCase()) ||
      srv.id?.toLowerCase().includes(searchQuery.toLowerCase()) ||
      srv.description?.toLowerCase().includes(searchQuery.toLowerCase()) ||
      (srv.tools && srv.tools.some(t => t.toLowerCase().includes(searchQuery.toLowerCase())));
    return matchCategory && matchSearch;
  });

  const totalToolsCount = servers.reduce((acc, s) => acc + (s.tools ? s.tools.length : 0), 0);
  const onlineServersCount = servers.filter(s => s.enabled).length;

  // Code Generation for Clients with Token Support
  const clientConfig = (client) => {
    try { return buildMCPClientConfig({ client, gatewayUrl: sseUrl, token: configToken, server: selectedServerForConfig }); }
    catch (err) { return JSON.stringify({ error: err.message }, null, 2); }
  };
  const getCursorConfig = () => clientConfig('cursor');
  const getClaudeConfig = () => clientConfig('claude');
  const getClineConfig = () => clientConfig('cline');

  const getPythonSnippet = () => {
    const token = configToken?.trim() || 'YOUR_AIROUTE_KEY';
    return `from mcp import ClientSession, StdioServerParameters
from mcp.client.sse import sse_client
import asyncio

async def run_mcp_client():
    # 配置统一鉴权 Token 请求头 (支持虚拟 API Key 与多租户配额隔离)
    headers = {
        "Authorization": "Bearer ${token}"
    }

    # 连接 Airoute 统一 MCP 代理网关 SSE 端点
    async with sse_client("${sseUrl}", headers=headers) as (read_stream, write_stream):
        async with ClientSession(read_stream, write_stream) as session:
            await session.initialize()

            # 列出网关聚合的所有 MCP 安全工具
            tools = await session.list_tools()
            print(f"网关已就绪，挂载 {len(tools.tools)} 个安全合规工具")

            # 调用数据安全脱敏工具示例
            result = await session.call_tool(
                "airoute_data_redact",
                arguments={"text": "客户张三 手机13812345678 密钥sk-abcdef123456"}
            )
            print("脱敏审计响应:", result.content[0].text)

if __name__ == "__main__":
    asyncio.run(run_mcp_client())`;
  };

  const getCurlSnippet = () => {
    const token = configToken?.trim() || 'YOUR_AIROUTE_KEY';
    return `# 1. 建立 MCP SSE 事件流长连接 (监听工具通知与端点握手)
curl -N -H "Authorization: Bearer ${token}" \\
  "${sseUrl}"

# 2. 发送 JSON-RPC 2.0 请求报文 (执行工具或发现服务)
curl -X POST "${messagesUrl}" \\
  -H "Authorization: Bearer ${token}" \\
  -H "Content-Type: application/json" \\
  -d '{
    "jsonrpc": "2.0",
    "id": "cli-test-01",
    "method": "tools/call",
    "params": {
      "name": "airoute_data_redact",
      "arguments": {
        "text": "客户张三 手机13812345678 密钥sk-abcdef123456"
      }
    }
  }'`;
  };
  return {
    categories, filteredServers, getClaudeConfig, getClineConfig, getCurlSnippet, getCursorConfig,
    getPythonSnippet, handleCopyText, onlineServersCount, totalToolsCount
  };
}
