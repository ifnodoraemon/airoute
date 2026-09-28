import React, { useState } from 'react';
import { X, Copy, Check, Terminal, Code, Cpu, Sparkles, BookOpen } from 'lucide-react';

export default function QuickStartModal({ virtualKey, onClose, onCopy }) {
  if (!virtualKey) return null;

  const [activeTab, setActiveTab] = useState('python');
  const [copiedTab, setCopiedTab] = useState('');

  const origin = window.location.origin || 'http://localhost:8080';
  const baseUrl = `${origin}/v1`;
  const keyStr = virtualKey.key || 'sk-airoute-xxxx';

  const pythonCode = `from openai import OpenAI

client = OpenAI(
    base_url="${baseUrl}",
    api_key="${keyStr}"
)

# 1. 聊天补全 (OpenAI / DeepSeek / Claude / Gemini 统一协议)
response = client.chat.completions.create(
    model="deepseek-chat",
    messages=[
        {"role": "system", "content": "You are a helpful coding assistant."},
        {"role": "user", "content": "用 Go 实现快速排序"}
    ],
    stream=True
)

for chunk in response:
    content = chunk.choices[0].delta.content or ""
    print(content, end="", flush=True)

# 2. 官方新代智能体 Responses API (/v1/responses)
# resp = client.responses.create(
#     model="gpt-4o",
#     input="分析微服务与单体架构的选型考量",
#     instructions="You are an enterprise system architect."
# )
# print(resp.output[0].content[0].text)
`;

  const curlCode = `# 1. 测试 Chat Completions (对话流式补全)
curl -X POST "${baseUrl}/chat/completions" \\
  -H "Authorization: Bearer ${keyStr}" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "deepseek-chat",
    "messages": [{"role": "user", "content": "你好，请做个自我介绍"}],
    "stream": true
  }'

# 2. 测试 OpenAI Responses API (新代智能体协议 /v1/responses)
curl -X POST "${baseUrl}/responses" \\
  -H "Authorization: Bearer ${keyStr}" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "deepseek-chat",
    "input": "请用一句话介绍微服务网关的核心价值",
    "instructions": "You are an infrastructure expert.",
    "stream": true
  }'

# 3. 探测已挂载模型列表
curl -X GET "${baseUrl}/models" \\
  -H "Authorization: Bearer ${keyStr}"
`;

  const nodeCode = `import OpenAI from "openai";

const openai = new OpenAI({
  baseURL: "${baseUrl}",
  apiKey: "${keyStr}",
});

async function main() {
  const stream = await openai.chat.completions.create({
    model: "deepseek-chat",
    messages: [{ role: "user", content: "解释量子计算的基本原理" }],
    stream: true,
  });

  for await (const chunk of stream) {
    process.stdout.write(chunk.choices[0]?.delta?.content || "");
  }
}

main();
`;

  const claudeEnvCode = `# 在终端中配置环境变量后，Claude Code / Cline / Cursor 将直接经由 Airoute 分发
export ANTHROPIC_BASE_URL="${origin}"
export ANTHROPIC_API_KEY="${keyStr}"

# 如果在 Cursor / Cline 中使用 OpenAI 兼容模式：
# Base URL: ${baseUrl}
# API Key: ${keyStr}
# Model: deepseek-chat 或 claude-3-5-sonnet-20241022
`;

  const desktopClientGuide = `# 常用第三方客户端配置指南 (NextChat / Chatbox / LobeChat / Cherry Studio)
# 
# 1. 接口地址 (API Host / Base URL):
#    ${baseUrl}
# 
# 2. API 访问密钥 (API Key):
#    ${keyStr}
# 
# 3. 自定义模型列表:
#    在客户端模型设置中填入当前网关已启用的模型名称 (例如 deepseek-chat 或部署的本地模型)
# 
# 4. 特色体验:
#    网关原生支持 DeepSeek-R1 思维链流式展开与全双工协议转换。
`;

  const getCode = () => {
    switch (activeTab) {
      case 'python': return pythonCode;
      case 'curl': return curlCode;
      case 'node': return nodeCode;
      case 'claude': return claudeEnvCode;
      case 'client': return desktopClientGuide;
      default: return pythonCode;
    }
  };

  const handleCopy = () => {
    onCopy(getCode());
    setCopiedTab(activeTab);
    setTimeout(() => setCopiedTab(''), 2000);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/60 backdrop-blur-sm animate-in fade-in">
      <div className="bg-white dark:bg-[#111726] border border-slate-200 dark:border-slate-800 rounded-3xl w-full max-w-3xl shadow-2xl overflow-hidden flex flex-col max-h-[90vh]">
        {/* Modal Header */}
        <div className="p-6 border-b border-slate-100 dark:border-slate-800/80 flex items-center justify-between bg-slate-50/50 dark:bg-slate-900/50">
          <div className="flex items-center space-x-3">
            <div className="w-10 h-10 rounded-2xl bg-indigo-50 dark:bg-indigo-900/30 border border-indigo-200 dark:border-indigo-700/50 flex items-center justify-center text-indigo-600 dark:text-indigo-400">
              <Sparkles className="w-5 h-5" />
            </div>
            <div>
              <h3 className="font-bold text-base text-slate-900 dark:text-slate-100 tracking-tight">
                接入代码示例
              </h3>
              <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
                密钥: <code className="font-mono text-indigo-600 dark:text-indigo-400 font-semibold">{keyStr}</code> (应用: {virtualKey.tenant_id})
              </p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-2 hover:bg-slate-100 dark:hover:bg-slate-800 rounded-xl text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 transition"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Tab switchers */}
        <div className="px-6 pt-4 flex items-center justify-between border-b border-slate-100 dark:border-slate-800/80 bg-white dark:bg-[#111726]">
          <div className="flex flex-wrap gap-2">
            <button
              onClick={() => setActiveTab('python')}
              className={`px-4 py-2 text-xs font-semibold rounded-xl transition flex items-center space-x-1.5 ${
                activeTab === 'python'
                  ? 'bg-indigo-50 dark:bg-indigo-900/40 text-indigo-700 dark:text-indigo-300 border border-indigo-200 dark:border-indigo-700/60'
                  : 'text-slate-500 hover:text-slate-800 dark:hover:text-slate-200'
              }`}
            >
              <Code className="w-3.5 h-3.5" />
              <span>Python SDK</span>
            </button>
            <button
              onClick={() => setActiveTab('curl')}
              className={`px-4 py-2 text-xs font-semibold rounded-xl transition flex items-center space-x-1.5 ${
                activeTab === 'curl'
                  ? 'bg-indigo-50 dark:bg-indigo-900/40 text-indigo-700 dark:text-indigo-300 border border-indigo-200 dark:border-indigo-700/60'
                  : 'text-slate-500 hover:text-slate-800 dark:hover:text-slate-200'
              }`}
            >
              <Terminal className="w-3.5 h-3.5" />
              <span>cURL</span>
            </button>
            <button
              onClick={() => setActiveTab('node')}
              className={`px-4 py-2 text-xs font-semibold rounded-xl transition flex items-center space-x-1.5 ${
                activeTab === 'node'
                  ? 'bg-indigo-50 dark:bg-indigo-900/40 text-indigo-700 dark:text-indigo-300 border border-indigo-200 dark:border-indigo-700/60'
                  : 'text-slate-500 hover:text-slate-800 dark:hover:text-slate-200'
              }`}
            >
              <Cpu className="w-3.5 h-3.5" />
              <span>Node / TS</span>
            </button>
            <button
              onClick={() => setActiveTab('claude')}
              className={`px-4 py-2 text-xs font-semibold rounded-xl transition flex items-center space-x-1.5 ${
                activeTab === 'claude'
                  ? 'bg-indigo-50 dark:bg-indigo-900/40 text-indigo-700 dark:text-indigo-300 border border-indigo-200 dark:border-indigo-700/60'
                  : 'text-slate-500 hover:text-slate-800 dark:hover:text-slate-200'
              }`}
            >
              <BookOpen className="w-3.5 h-3.5" />
              <span>Claude Code / Cursor</span>
            </button>
            <button
              onClick={() => setActiveTab('client')}
              className={`px-4 py-2 text-xs font-semibold rounded-xl transition flex items-center space-x-1.5 ${
                activeTab === 'client'
                  ? 'bg-indigo-50 dark:bg-indigo-900/40 text-indigo-700 dark:text-indigo-300 border border-indigo-200 dark:border-indigo-700/60'
                  : 'text-slate-500 hover:text-slate-800 dark:hover:text-slate-200'
              }`}
            >
              <span>桌面客户端</span>
            </button>
          </div>

          <button
            onClick={handleCopy}
            className="px-3.5 py-1.5 text-xs rounded-xl bg-indigo-600 hover:bg-indigo-700 text-white font-medium flex items-center space-x-1.5 transition shadow-sm"
          >
            {copiedTab === activeTab ? (
              <>
                <Check className="w-3.5 h-3.5 text-emerald-300" />
                <span>已复制！</span>
              </>
            ) : (
              <>
                <Copy className="w-3.5 h-3.5" />
                <span>复制代码</span>
              </>
            )}
          </button>
        </div>

        {/* Code Content */}
        <div className="p-6 overflow-y-auto flex-1 bg-slate-950 font-mono text-xs text-slate-200 leading-relaxed">
          <pre className="whitespace-pre overflow-x-auto">{getCode()}</pre>
        </div>

        {/* Modal Footer */}
        <div className="p-4 bg-slate-50 dark:bg-slate-900/50 border-t border-slate-100 dark:border-slate-800/80 flex items-center justify-end text-xs">
          <button
            onClick={onClose}
            className="px-4 py-2 bg-slate-200 dark:bg-slate-800 hover:bg-slate-300 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-200 rounded-xl font-medium transition"
          >
            关闭
          </button>
        </div>
      </div>
    </div>
  );
}
