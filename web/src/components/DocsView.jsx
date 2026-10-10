import React, { useState } from 'react';
import {
  Shield,
  Code,
  Terminal,
  Image as ImageIcon,
  Layers,
  Sliders,
  Server,
  Copy,
  Sparkles,
  Check
} from 'lucide-react';
import { getGatewayOrigin } from '../config';

export default function DocsView({ showToast, lang = 'zh', t }) {
  const isZh = lang === 'zh';
  const [docsSection, setDocsSection] = useState('architecture');
  const [copiedKey, setCopiedKey] = useState('');

  const docOrigin = getGatewayOrigin();
  const docBaseUrl = `${docOrigin}/v1`;

  const copyToClipboard = (text, key) => {
    navigator.clipboard.writeText(text);
    setCopiedKey(key || 'code');
    setTimeout(() => setCopiedKey(''), 2200);
    if (showToast) {
      showToast(isZh ? '代码已复制到剪贴板' : 'Code copied to clipboard', 'success');
    }
  };

  return (
    <div className="grid grid-cols-1 md:grid-cols-4 gap-6">
      {/* Category sidebar */}
      <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl p-4 space-y-1 shadow-xs h-fit">
        <span className="text-xs font-bold text-slate-400 dark:text-slate-500 uppercase tracking-wider px-3 mb-2 block">
          {isZh ? '接入与规范文档' : 'Documentation & Specs'}
        </span>
        
        <button
          onClick={() => setDocsSection('architecture')}
          className={`w-full text-left px-3 py-2 rounded-xl text-xs font-semibold flex items-center space-x-2 transition cursor-pointer ${
            docsSection === 'architecture'
              ? 'bg-indigo-50 dark:bg-indigo-950/60 text-indigo-700 dark:text-indigo-300 font-bold border border-indigo-200 dark:border-indigo-800/60'
              : 'text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800'
          }`}
        >
          <Shield className="w-3.5 h-3.5 text-indigo-500" />
          <span>{isZh ? '核心架构与高可用设计' : 'Architecture & HA'}</span>
        </button>

        <button
          onClick={() => setDocsSection('quickstart')}
          className={`w-full text-left px-3 py-2 rounded-xl text-xs font-semibold flex items-center space-x-2 transition cursor-pointer ${
            docsSection === 'quickstart'
              ? 'bg-indigo-50 dark:bg-indigo-950/60 text-indigo-700 dark:text-indigo-300 font-bold border border-indigo-200 dark:border-indigo-800/60'
              : 'text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800'
          }`}
        >
          <Code className="w-3.5 h-3.5" />
          <span>{isZh ? 'OpenAI SDK 极速接入' : 'OpenAI SDK Quickstart'}</span>
        </button>

        <button
          onClick={() => setDocsSection('claude')}
          className={`w-full text-left px-3 py-2 rounded-xl text-xs font-semibold flex items-center space-x-2 transition cursor-pointer ${
            docsSection === 'claude'
              ? 'bg-indigo-50 dark:bg-indigo-950/60 text-indigo-700 dark:text-indigo-300 font-bold border border-indigo-200 dark:border-indigo-800/60'
              : 'text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800'
          }`}
        >
          <Terminal className="w-3.5 h-3.5" />
          <span>{isZh ? 'Claude Messages API 接入' : 'Claude Messages API'}</span>
        </button>

        <button
          onClick={() => setDocsSection('multimodal')}
          className={`w-full text-left px-3 py-2 rounded-xl text-xs font-semibold flex items-center space-x-2 transition cursor-pointer ${
            docsSection === 'multimodal'
              ? 'bg-indigo-50 dark:bg-indigo-950/60 text-indigo-700 dark:text-indigo-300 font-bold border border-indigo-200 dark:border-indigo-800/60'
              : 'text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800'
          }`}
        >
          <ImageIcon className="w-3.5 h-3.5" />
          <span>{isZh ? '多模态 (图/音/视) 接口规范' : 'Multimodal Endpoints'}</span>
        </button>

        <button
          onClick={() => setDocsSection('cascading')}
          className={`w-full text-left px-3 py-2 rounded-xl text-xs font-semibold flex items-center space-x-2 transition cursor-pointer ${
            docsSection === 'cascading'
              ? 'bg-indigo-50 dark:bg-indigo-950/60 text-indigo-700 dark:text-indigo-300 font-bold border border-indigo-200 dark:border-indigo-800/60'
              : 'text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800'
          }`}
        >
          <Layers className="w-3.5 h-3.5" />
          <span>{isZh ? '级联模型映射语法' : 'Cascading Mapping Rules'}</span>
        </button>

        <button
          onClick={() => setDocsSection('rerank')}
          className={`w-full text-left px-3 py-2 rounded-xl text-xs font-semibold flex items-center space-x-2 transition cursor-pointer ${
            docsSection === 'rerank'
              ? 'bg-indigo-50 dark:bg-indigo-950/60 text-indigo-700 dark:text-indigo-300 font-bold border border-indigo-200 dark:border-indigo-800/60'
              : 'text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800'
          }`}
        >
          <Sliders className="w-3.5 h-3.5" />
          <span>{isZh ? 'Rerank 检索重排规范' : 'Rerank API Guide'}</span>
        </button>

        <button
          onClick={() => setDocsSection('deploy')}
          className={`w-full text-left px-3 py-2 rounded-xl text-xs font-semibold flex items-center space-x-2 transition cursor-pointer ${
            docsSection === 'deploy'
              ? 'bg-indigo-50 dark:bg-indigo-950/60 text-indigo-700 dark:text-indigo-300 font-bold border border-indigo-200 dark:border-indigo-800/60'
              : 'text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800'
          }`}
        >
          <Server className="w-3.5 h-3.5" />
          <span>{isZh ? 'Docker & K8s 高可用部署' : 'Cluster Deployment'}</span>
        </button>
      </div>

      {/* Doc Content */}
      <div className="md:col-span-3 bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl p-6 shadow-xs space-y-6">
        {docsSection === 'architecture' && (
          <div className="space-y-6">
            <div className="border-b border-slate-100 dark:border-slate-800 pb-3">
              <div className="flex items-center space-x-2">
                <Shield className="w-5 h-5 text-indigo-500" />
                <h3 className="text-base font-bold text-slate-900 dark:text-slate-100">
                  Airoute 核心架构与高可用设计规范
                </h3>
              </div>
              <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
                为企业私有化部署打造的超高性能、全双工协议转换与零感知容灾大模型统一接入网关。
              </p>
            </div>

            {/* Feature 1: Pre-Token Fallback */}
            <div className="p-5 rounded-2xl bg-indigo-50/50 dark:bg-indigo-950/30 border border-indigo-200/80 dark:border-indigo-800/60 space-y-3">
              <div className="flex items-center justify-between">
                <h4 className="font-bold text-sm text-indigo-900 dark:text-indigo-200 flex items-center space-x-2">
                  <Shield className="w-4 h-4 text-indigo-600 dark:text-indigo-400" />
                  <span>1. 首字前无感容灾兜底 (Pre-Token Fallback)</span>
                </h4>
                <span className="text-[11px] font-mono px-2 py-0.5 rounded-md bg-indigo-100 dark:bg-indigo-900/60 text-indigo-700 dark:text-indigo-300 font-semibold">
                  Zero-Perception Failover
                </span>
              </div>
              <p className="text-xs text-slate-700 dark:text-slate-300 leading-relaxed">
                遭遇上游 429 (并发限流)、500/502/503 (服务端异常) 或网络连接超时，首字分块发出前毫秒内切换到备份 Provider，客户端完全无感，连接不中断，零错误率。
              </p>
              <div className="bg-white dark:bg-[#0b0f19] p-3.5 rounded-xl border border-indigo-100 dark:border-indigo-900/50 text-xs space-y-2">
                <span className="font-semibold text-slate-800 dark:text-slate-200 block">容灾工作流时序:</span>
                <div className="font-mono text-[11px] text-slate-600 dark:text-slate-400 space-y-1">
                  <div>[客户端请求] ➔ Airoute ➔ 首选渠道 A (Primary Provider)</div>
                  <div className="text-amber-600 dark:text-amber-400">↳ 发生 429 Rate Limit / 500 错误 (未输出首字 chunk)</div>
                  <div className="text-emerald-600 dark:text-emerald-400">↳ 网关拦截异常并在 15ms 内重定向至备份渠道 B (Backup Provider)</div>
                  <div>↳ 客户端正常接收首字分块及完整 SSE 数据流，业务层调用成功率稳定保持 100%</div>
                </div>
              </div>
            </div>

            {/* Feature 2: Protocol Translation */}
            <div className="p-5 rounded-2xl bg-emerald-50/50 dark:bg-emerald-950/30 border border-emerald-200/80 dark:border-emerald-800/60 space-y-3">
              <div className="flex items-center justify-between">
                <h4 className="font-bold text-sm text-emerald-900 dark:text-emerald-200 flex items-center space-x-2">
                  <Cpu className="w-4 h-4 text-emerald-600 dark:text-emerald-400" />
                  <span>2. 智能探测 & 全双工协议转换 (Full-Duplex Protocol Matrix)</span>
                </h4>
                <span className="text-[11px] font-mono px-2 py-0.5 rounded-md bg-emerald-100 dark:bg-emerald-900/60 text-emerald-700 dark:text-emerald-300 font-semibold">
                  Bi-Directional Translation
                </span>
              </div>
              <p className="text-xs text-slate-700 dark:text-slate-300 leading-relaxed">
                原生支持 OpenAI、Claude Messages、Gemini 与主流推理协议，并实现全双工流式实时转译。客户端使用任何主流 SDK 均可自由互通！
              </p>
              <div className="overflow-x-auto">
                <table className="w-full text-left border-collapse text-xs">
                  <thead>
                    <tr className="border-b border-emerald-200/60 dark:border-emerald-800/60 text-slate-500 dark:text-slate-400">
                      <th className="py-2 px-3 font-semibold">下游客户端调用协议</th>
                      <th className="py-2 px-3 font-semibold">网关接入端点</th>
                      <th className="py-2 px-3 font-semibold">支持的上游后端提供商</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-emerald-100 dark:divide-emerald-900/40 text-[11px]">
                    <tr>
                      <td className="py-2 px-3 font-mono text-indigo-600 dark:text-indigo-400 font-semibold">OpenAI Chat</td>
                      <td className="py-2 px-3 font-mono">/v1/chat/completions</td>
                      <td className="py-2 px-3">OpenAI, DeepSeek, Claude, Gemini 及主流推理集群</td>
                    </tr>
                    <tr>
                      <td className="py-2 px-3 font-mono text-indigo-600 dark:text-indigo-400 font-semibold">OpenAI Responses</td>
                      <td className="py-2 px-3 font-mono">/v1/responses</td>
                      <td className="py-2 px-3">新代智能体协议，支持全格式转译与流式推导</td>
                    </tr>
                    <tr>
                      <td className="py-2 px-3 font-mono text-indigo-600 dark:text-indigo-400 font-semibold">Claude Messages</td>
                      <td className="py-2 px-3 font-mono">/v1/messages</td>
                      <td className="py-2 px-3">Anthropic 官方、OpenAI 格式上游、主流私有推理集群</td>
                    </tr>
                    <tr>
                      <td className="py-2 px-3 font-mono text-indigo-600 dark:text-indigo-400 font-semibold">Google Gemini</td>
                      <td className="py-2 px-3 font-mono">/v1beta/models/*</td>
                      <td className="py-2 px-3">Gemini 官方、OpenAI 格式上游全双工转译</td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>

            {/* Feature 3: Multimodal Pipeline */}
            <div className="p-5 rounded-2xl bg-purple-50/50 dark:bg-purple-950/30 border border-purple-200/80 dark:border-purple-800/60 space-y-3">
              <div className="flex items-center justify-between">
                <h4 className="font-bold text-sm text-purple-900 dark:text-purple-200 flex items-center space-x-2">
                  <Layers className="w-4 h-4 text-purple-600 dark:text-purple-400" />
                  <span>3. 全模态统一管道 (Unified Pipeline)</span>
                </h4>
                <span className="text-[11px] font-mono px-2 py-0.5 rounded-md bg-purple-100 dark:bg-purple-900/60 text-purple-700 dark:text-purple-300 font-semibold">
                  Unified Pipeline
                </span>
              </div>
              <p className="text-xs text-slate-700 dark:text-slate-300 leading-relaxed">
                Chat 对话、AI 生图 (DALL-E / Flux)、语音合成 (TTS)、Whisper 语音转录与翻译、视频生成均采用统一调度分发，高内聚低耦合，共享熔断、鉴权与计量基础设施。
              </p>
              <div className="grid grid-cols-2 sm:grid-cols-4 gap-2 pt-1 text-center">
                <div className="p-2.5 rounded-xl bg-white dark:bg-[#0b0f19] border border-purple-100 dark:border-purple-900/50">
                  <span className="text-xs font-bold text-purple-700 dark:text-purple-300">💬 文本/对话</span>
                  <p className="text-[10px] text-slate-400 mt-0.5">SSE 零拷贝流式</p>
                </div>
                <div className="p-2.5 rounded-xl bg-white dark:bg-[#0b0f19] border border-purple-100 dark:border-purple-900/50">
                  <span className="text-xs font-bold text-pink-700 dark:text-pink-300">🎨 图像生成</span>
                  <p className="text-[10px] text-slate-400 mt-0.5">/v1/images/generations</p>
                </div>
                <div className="p-2.5 rounded-xl bg-white dark:bg-[#0b0f19] border border-purple-100 dark:border-purple-900/50">
                  <span className="text-xs font-bold text-cyan-700 dark:text-cyan-300">🔊 语音 TTS/STT</span>
                  <p className="text-[10px] text-slate-400 mt-0.5">音频二进制直通流</p>
                </div>
                <div className="p-2.5 rounded-xl bg-white dark:bg-[#0b0f19] border border-purple-100 dark:border-purple-900/50">
                  <span className="text-xs font-bold text-teal-700 dark:text-teal-300">🎬 视频生成</span>
                  <p className="text-[10px] text-slate-400 mt-0.5">异步任务与轮询</p>
                </div>
              </div>
            </div>

            {/* Feature 4: High Availability */}
            <div className="p-5 rounded-2xl bg-sky-50/50 dark:bg-sky-950/30 border border-sky-200/80 dark:border-sky-800/60 space-y-2">
              <h4 className="font-bold text-sm text-sky-900 dark:text-sky-200 flex items-center space-x-2">
                <Server className="w-4 h-4 text-sky-600 dark:text-sky-400" />
                <span>4. Zero-DB 热路径与双节点双活高可用架构</span>
              </h4>
              <p className="text-xs text-slate-700 dark:text-slate-300 leading-relaxed">
                数据面读操作 100% 内存无锁运行，无任何数据库 IO 延迟；控制面配置变更通过 WAL 模式原子写入并毫秒级广播至内存。前置 Nginx 对 2 个网关副本进行加权轮询，实现节点故障秒级隔离与 99.99% 高可用。
              </p>
            </div>
          </div>
        )}

        {docsSection === 'quickstart' && (
          <div className="space-y-4">
            <div className="border-b border-slate-100 dark:border-slate-800 pb-3">
              <h3 className="text-base font-bold text-slate-900 dark:text-slate-100">Python OpenAI SDK 接入指南</h3>
              <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">将官方 OpenAI SDK 的 base_url 直接指向 Airoute 网关入口即可（自适应当前访问域名与端口）。</p>
            </div>

            <div className="relative group">
              <pre className="p-4 bg-slate-900 text-slate-100 rounded-xl text-xs font-mono overflow-x-auto leading-relaxed">
{`from openai import OpenAI

client = OpenAI(
    base_url="${docBaseUrl}",  # Airoute 网关入口 (自适应当前主机与端口)
    api_key="sk-airoute-xxxx",               # 在工作台签发的客户端访问密钥
)

response = client.chat.completions.create(
    model="deepseek-v3",                 # 支持任意映射模型
    messages=[{"role": "user", "content": "你好！"}],
    stream=True,                         # 原生毫秒级 SSE 流式传输
)

for chunk in response:
    content = chunk.choices[0].delta.content or ""
    print(content, end="", flush=True)`}
              </pre>
              <button
                onClick={() => copyToClipboard(`from openai import OpenAI\n\nclient = OpenAI(\n    base_url="${docBaseUrl}",\n    api_key="sk-airoute-xxxx",\n)\n\nresponse = client.chat.completions.create(\n    model="deepseek-v3",\n    messages=[{"role": "user", "content": "你好！"}],\n    stream=True,\n)\n\nfor chunk in response:\n    content = chunk.choices[0].delta.content or ""\n    print(content, end="", flush=True)`, 'py')}
                className="absolute top-3 right-3 px-2 py-1 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded text-[11px] font-mono flex items-center space-x-1"
              >
                {copiedKey === 'py' ? <Check className="w-3 h-3 text-emerald-400" /> : <Copy className="w-3 h-3" />}
                <span>{copiedKey === 'py' ? '已复制' : '复制'}</span>
              </button>
            </div>

            <div className="p-4 bg-amber-50 dark:bg-amber-950/40 border border-amber-200 dark:border-amber-800/60 rounded-xl text-xs text-amber-900 dark:text-amber-200 space-y-1">
              <span className="font-bold flex items-center space-x-1">
                <Sparkles className="w-3.5 h-3.5 text-amber-600 dark:text-amber-400" />
                <span>DeepSeek-R1 / OpenAI o1 思维链透明透传:</span>
              </span>
              <p className="leading-relaxed">
                网关在流式与非流式模式下均原生支持 <code>reasoning_content</code> 字段的零拷贝透传。前端应用或 NextChat / LobeChat 可直接原生渲染展开思维链推导卡片。
              </p>
            </div>

            <div className="border-t border-slate-100 dark:border-slate-800 pt-4">
              <h4 className="text-xs font-bold text-slate-800 dark:text-slate-200 mb-2">cURL 极速调试命令:</h4>
              <div className="relative group">
                <pre className="p-3 bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl text-xs font-mono text-slate-700 dark:text-slate-300 overflow-x-auto">
{`curl -X POST "${docBaseUrl}/chat/completions" \\
  -H "Authorization: Bearer sk-airoute-xxxx" \\
  -H "Content-Type: application/json" \\
  -d '{"model": "deepseek-v3", "messages": [{"role": "user", "content": "Ping"}], "stream": true}'`}
                </pre>
                <button
                  onClick={() => copyToClipboard(`curl -X POST "${docBaseUrl}/chat/completions" \\\n  -H "Authorization: Bearer sk-airoute-xxxx" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model": "deepseek-v3", "messages": [{"role": "user", "content": "Ping"}], "stream": true}'`, 'curl')}
                  className="absolute top-2 right-2 px-2 py-1 bg-slate-200 dark:bg-slate-800 hover:bg-slate-300 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-200 rounded text-[11px] font-mono flex items-center space-x-1"
                >
                  {copiedKey === 'curl' ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                  <span>{copiedKey === 'curl' ? '已复制' : '复制'}</span>
                </button>
              </div>
            </div>
          </div>
        )}

        {docsSection === 'claude' && (
          <div className="space-y-4">
            <div className="border-b border-slate-100 dark:border-slate-800 pb-3">
              <h3 className="text-base font-bold text-slate-900 dark:text-slate-100">Anthropic Claude Messages API 接入</h3>
              <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">原生支持 Claude Code、Cursor、Cline 等工具直接使用 Anthropic 原生协议调用任何异构下游！</p>
            </div>

            <div className="relative group">
              <pre className="p-4 bg-slate-900 text-slate-100 rounded-xl text-xs font-mono overflow-x-auto leading-relaxed">
{`import anthropic

client = anthropic.Anthropic(
    base_url="${docOrigin}",  # 网关根路径，将自动请求 /v1/messages
    api_key="sk-airoute-xxxx",            # 在工作台签发的客户端访问密钥
)

message = client.messages.create(
    model="claude-3-7-sonnet",
    max_tokens=1024,
    messages=[
        {"role": "user", "content": "请介绍量子计算的核心原理。"}
    ]
)
print(message.content[0].text)`}
              </pre>
              <button
                onClick={() => copyToClipboard(`import anthropic\n\nclient = anthropic.Anthropic(\n    base_url="${docOrigin}",\n    api_key="sk-airoute-xxxx",\n)\n\nmessage = client.messages.create(\n    model="claude-3-7-sonnet",\n    max_tokens=1024,\n    messages=[\n        {"role": "user", "content": "请介绍量子计算的核心原理。"}\n    ]\n)\nprint(message.content[0].text)`, 'claude')}
                className="absolute top-3 right-3 px-2 py-1 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded text-[11px] font-mono flex items-center space-x-1"
              >
                {copiedKey === 'claude' ? <Check className="w-3 h-3 text-emerald-400" /> : <Copy className="w-3 h-3" />}
                <span>{copiedKey === 'claude' ? '已复制' : '复制'}</span>
              </button>
            </div>

            <div className="p-4 bg-indigo-50 dark:bg-indigo-950/40 border border-indigo-200 dark:border-indigo-800/60 rounded-xl text-xs text-indigo-900 dark:text-indigo-200 space-y-1">
              <span className="font-bold">💡 全双工协议转换特性:</span>
              <p>
                即使您的下游供应商是仅支持 OpenAI 协议的私有集群，客户端通过 Anthropic SDK 请求时，网关也会在内存零拷贝将 Claude Messages 双向转换为 OpenAI Completions 并在返回时转回 Anthropic 格式。
              </p>
            </div>

            <div className="border-t border-slate-100 dark:border-slate-800 pt-4">
              <h4 className="text-xs font-bold text-slate-800 dark:text-slate-200 mb-1">Anthropic Token 预估计算 (/v1/messages/count_tokens):</h4>
              <pre className="p-3 bg-slate-900 text-slate-100 rounded-xl text-xs font-mono overflow-x-auto">
{`curl -X POST "${docOrigin}/v1/messages/count_tokens" \\
  -H "x-api-key: sk-airoute-xxxx" \\
  -H "anthropic-version: 2023-06-01" \\
  -H "Content-Type: application/json" \\
  -d '{"model": "claude-3-7-sonnet", "messages": [{"role": "user", "content": "Hello world"}]}'

# 响应示例:
# {"input_tokens": 12}`}
              </pre>
            </div>
          </div>
        )}

        {docsSection === 'multimodal' && (
          <div className="space-y-4">
            <div className="border-b border-slate-100 dark:border-slate-800 pb-3">
              <h3 className="text-base font-bold text-slate-900 dark:text-slate-100">多模态 API 接口规范</h3>
              <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">生图、语音合成 TTS、语音识别 STT、语音翻译、视频生成与轮询均通过统一熔断与分发管道提供。</p>
            </div>

            <div className="space-y-3 text-xs">
              <div className="p-3 bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl">
                <span className="font-bold text-pink-600 dark:text-pink-400">🎨 1. AI 图像生成 (/v1/images/generations)</span>
                <pre className="mt-1 font-mono text-slate-700 dark:text-slate-300">
{`POST /v1/images/generations
{"model": "flux-1-schnell", "prompt": "cyberpunk city, 8k", "size": "1024x1024"}`}
                </pre>
              </div>

              <div className="p-3 bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl">
                <span className="font-bold text-cyan-600 dark:text-cyan-400">🔊 2. 语音合成 TTS (/v1/audio/speech)</span>
                <pre className="mt-1 font-mono text-slate-700 dark:text-slate-300">
{`POST /v1/audio/speech
{"model": "tts-1", "input": "你好世界", "voice": "alloy", "response_format": "mp3"}
(返回二进制流式音频流，零内存占用直连客户端)`}
                </pre>
              </div>

              <div className="p-3 bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl">
                <span className="font-bold text-teal-600 dark:text-teal-400">🎙️ 3. Whisper 语音转录 (/v1/audio/transcriptions)</span>
                <pre className="mt-1 font-mono text-slate-700 dark:text-slate-300">
{`POST /v1/audio/transcriptions (multipart/form-data)
file=@recording.mp3; model=whisper-large-v3-turbo`}
                </pre>
              </div>

              <div className="p-3 bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl">
                <span className="font-bold text-purple-600 dark:text-purple-400">🎬 4. 视频生成与轮询 (/v1/videos/generations & /v1/videos/tasks/:id)</span>
                <pre className="mt-1 font-mono text-slate-700 dark:text-slate-300">
{`POST /v1/videos/generations -> 返回 {"task_id": "task_xxx", "status": "PENDING"}
GET /v1/videos/tasks/:id    -> 轮询状态直到 SUCCESS 并返回 video_url`}
                </pre>
              </div>

              <div className="p-3 bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl">
                <span className="font-bold text-emerald-600 dark:text-emerald-400">🧠 5. 文本向量化 Embeddings (/v1/embeddings)</span>
                <pre className="mt-1 font-mono text-slate-700 dark:text-slate-300">
{`POST /v1/embeddings
{"model": "text-embedding-3-small", "input": "企业级超高性能大模型网关"}`}
                </pre>
              </div>
            </div>
          </div>
        )}

        {docsSection === 'cascading' && (
          <div className="space-y-4">
            <div className="border-b border-slate-100 dark:border-slate-800 pb-3">
              <h3 className="text-base font-bold text-slate-900 dark:text-slate-100">级联模型别名与通配映射</h3>
              <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">不设任何斜杠深度限制，支持多组织层级命名与任意前缀重写。</p>
            </div>

            <div className="p-4 bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl text-xs space-y-3">
              <h4 className="font-bold text-slate-900 dark:text-slate-100">映射格式与示例:</h4>
              <ul className="list-disc pl-5 space-y-1.5 text-slate-700 dark:text-slate-300">
                <li>
                  <strong>精确别名重写:</strong> <code className="text-indigo-600 font-mono">yy/xxx/xx:xxx/xx</code> <br />
                  客户端请求 <code className="text-indigo-600 font-mono">yy/xxx/xx</code>，发往上游时自动零拷贝重写为 <code className="text-indigo-600 font-mono">xxx/xx</code>。
                </li>
                <li>
                  <strong>前缀通配映射:</strong> <code className="text-indigo-600 font-mono">org/dept/*:*</code> <br />
                  客户端请求 <code className="text-indigo-600 font-mono">org/dept/v1/deepseek-ai/DeepSeek-V4-Pro</code>，自动剥离前缀发往目标集群。
                </li>
                <li>
                  <strong>服务商自动前缀:</strong> <code className="text-indigo-600 font-mono">&lt;ProviderName&gt;/&lt;Model&gt;</code> <br />
                  当存在多个提供商均提供 <code className="text-indigo-600 font-mono">gpt-6</code> 时，客户端可直接指定 <code className="text-indigo-600 font-mono">openai-us/gpt-6</code> 精准定向路由！
                </li>
              </ul>
            </div>
          </div>
        )}

        {docsSection === 'rerank' && (
          <div className="space-y-4">
            <div className="border-b border-slate-100 dark:border-slate-800 pb-3">
              <h3 className="text-base font-bold text-slate-900 dark:text-slate-100">Rerank 检索重排 API 规范 (/v1/rerank)</h3>
              <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">全面兼容 Cohere、Hugging Face TEI、Xinference 与 Infinity 等重排标准协议。</p>
            </div>

            <div className="space-y-3 text-xs">
              <div className="p-4 bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl space-y-2">
                <span className="font-bold text-slate-900 dark:text-slate-100">1. 重排请求格式 (POST /v1/rerank):</span>
                <pre className="p-3 bg-slate-900 text-slate-100 rounded-xl font-mono overflow-x-auto text-[11px] leading-relaxed">
{`curl -X POST ${docOrigin}/v1/rerank \\
  -H "Authorization: Bearer sk-airoute-xxxx" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "bge-reranker-large",
    "query": "什么是企业级大模型网关的高可用与容灾设计？",
    "documents": [
      "Airoute 采用全双工流式转发，首字分块前支持透明故障转移与熔断兜底。",
      "今天天气非常晴朗，公园里的樱花盛开了，很适合去散步或野餐。",
      "基于 Raft 协议的分布式数据库能保证网络分区状态下的强一致性与多副本高可用。"
    ],
    "top_n": 2,
    "return_documents": true
  }'`}
                </pre>
              </div>

              <div className="p-4 bg-amber-50 dark:bg-amber-950/40 border border-amber-200 dark:border-amber-800/60 rounded-xl text-xs text-amber-900 dark:text-amber-200 space-y-1">
                <span className="font-bold">🎯 高可用熔断与透明兜底保障:</span>
                <p className="leading-relaxed">
                  当首选重排提供商（如私有部署的集群实例）发生 OOM、503 或网络异常时，Airoute 会在毫秒级内自动安全切换至备选重排提供商，为企业级 RAG 知识库检索流水线提供全天候 99.99% 的 SLA 稳定可用保障。
                </p>
              </div>
            </div>
          </div>
        )}

        {docsSection === 'deploy' && (
          <div className="space-y-4">
            <div className="border-b border-slate-100 dark:border-slate-800 pb-3">
              <h3 className="text-base font-bold text-slate-900 dark:text-slate-100">生产环境高可用集群部署 (HA)</h3>
              <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">提供开箱即用的多副本 Docker Compose 与生产级 Kubernetes Helm Chart。</p>
            </div>

            <div className="space-y-3">
              <h4 className="text-xs font-bold text-slate-800 dark:text-slate-200">1. Docker Compose (2 副本 Gateway + Nginx 负载均衡):</h4>
              <pre className="p-3 bg-slate-900 text-slate-100 rounded-xl text-xs font-mono overflow-x-auto">
docker compose up -d --build
              </pre>

              <h4 className="text-xs font-bold text-slate-800 dark:text-slate-200 pt-2">2. Kubernetes Helm 一键部署:</h4>
              <pre className="p-3 bg-slate-900 text-slate-100 rounded-xl text-xs font-mono overflow-x-auto">
helm install airoute ./helm/airoute -n gateway --create-namespace
              </pre>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
