import React, { useState } from 'react';
import {
  X,
  Sparkles,
  Key,
  Cpu,
  Code2,
  Terminal,
  CheckCircle2,
  Copy,
  Check,
  ArrowRight,
  Play,
  RefreshCw,
  Zap,
  ShieldCheck,
  ExternalLink
} from 'lucide-react';
import { getGatewayOrigin } from '../config';

export default function OnboardingModal({
  isOpen,
  onClose,
  keys = [],
  models = [],
  onNavigateToPlayground,
  showToast,
  lang = 'zh',
  t
}) {
  const isZh = lang === 'zh';
  const [step, setStep] = useState(1);
  const [selectedKey, setSelectedKey] = useState(() => (keys.length > 0 ? keys[0].key : 'sk-airoute-demo-key'));
  const [selectedProtocol, setSelectedProtocol] = useState('openai');
  const [selectedModel, setSelectedModel] = useState(() => (models.length > 0 ? models[0] : 'deepseek-v3'));
  const [snippetTab, setSnippetTab] = useState('python');
  const [copiedKey, setCopiedKey] = useState('');
  const [testingPing, setTestingPing] = useState(false);
  const [testResult, setTestResult] = useState(null);

  if (!isOpen) return null;

  const origin = getGatewayOrigin();

  const copyToClipboard = (text, key) => {
    navigator.clipboard.writeText(text);
    setCopiedKey(key || 'snippet');
    setTimeout(() => setCopiedKey(''), 2000);
    if (showToast) showToast(isZh ? '已复制到剪贴板！' : 'Copied to clipboard!', 'success');
  };

  const handleTestProbe = async () => {
    setTestingPing(true);
    setTestResult(null);
    const start = Date.now();
    try {
      const res = await fetch('/v1/chat/completions', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${selectedKey}`,
        },
        body: JSON.stringify({
          model: selectedModel,
          messages: [{ role: 'user', content: isZh ? 'Ping: 首次接入握手测试' : 'Ping: First handshake probe' }],
          max_tokens: 30,
          stream: false,
        }),
      });
      const data = await res.json().catch(() => ({}));
      const duration = Date.now() - start;
      if (res.ok && data.choices && data.choices[0]) {
        setTestResult({
          success: true,
          latency: duration,
          text: data.choices[0].message?.content || (isZh ? '通信正常！' : 'Communication healthy!'),
          model: data.model || selectedModel,
        });
        if (showToast) showToast(isZh ? '握手成功！网关通信正常' : 'Handshake successful! Gateway is online', 'success');
      } else {
        setTestResult({
          success: false,
          latency: duration,
          error: data.error?.message || `HTTP ${res.status}: ${res.statusText}`,
        });
      }
    } catch (e) {
      setTestResult({
        success: false,
        latency: Date.now() - start,
        error: e.message || (isZh ? '网络连接超时' : 'Connection timeout'),
      });
    } finally {
      setTestingPing(false);
    }
  };

  const pythonCode = `from openai import OpenAI

client = OpenAI(
    base_url="${origin}/v1",
    api_key="${selectedKey}",
)

response = client.chat.completions.create(
    model="${selectedModel}",
    messages=[
        {"role": "user", "content": "Hello Airoute!"}
    ],
    stream=True
)

for chunk in response:
    print(chunk.choices[0].delta.content or "", end="")
`;

  const tsCode = `import OpenAI from "openai";

const client = new OpenAI({
  baseURL: "${origin}/v1",
  apiKey: "${selectedKey}",
});

async function main() {
  const stream = await client.chat.completions.create({
    model: "${selectedModel}",
    messages: [{ role: "user", content: "Hello Airoute!" }],
    stream: true,
  });

  for await (const chunk of stream) {
    process.stdout.write(chunk.choices[0]?.delta?.content || "");
  }
}

main();
`;

  const curlCode = `curl -X POST "${origin}/v1/chat/completions" \\
  -H "Authorization: Bearer ${selectedKey}" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "${selectedModel}",
    "messages": [
      {"role": "user", "content": "Hello Airoute!"}
    ],
    "stream": true
  }'
`;

  const claudeCode = `import anthropic

client = anthropic.Anthropic(
    base_url="${origin}",
    api_key="${selectedKey}",
)

message = client.messages.create(
    model="${selectedModel}",
    max_tokens=1024,
    messages=[
        {"role": "user", "content": "Hello Airoute from Claude SDK!"}
    ]
)
print(message.content[0].text)
`;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/60 backdrop-blur-sm animate-in fade-in duration-200">
      <div className="bg-white dark:bg-[#111726] border border-slate-200 dark:border-slate-800 rounded-3xl max-w-2xl w-full p-6 sm:p-7 shadow-2xl relative space-y-6">
        {/* Header */}
        <div className="flex items-start justify-between">
          <div className="flex items-center space-x-3">
            <div className="w-10 h-10 rounded-2xl bg-gradient-to-tr from-indigo-500 to-sky-500 text-white flex items-center justify-center shadow-md">
              <Sparkles className="w-5 h-5" />
            </div>
            <div>
              <h2 className="text-base font-bold text-slate-900 dark:text-slate-100">
                {isZh ? '开发者极速上手接入向导' : 'Developer Onboarding Wizard'}
              </h2>
              <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
                {isZh ? '3 步完成密钥配置、多协议代码生成与网关实时连通性探测' : '3 steps: API Key selection, multi-protocol SDK snippet generation & live handshake probe'}
              </p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 rounded-xl text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-800 transition cursor-pointer"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Step Indicator */}
        <div className="flex items-center justify-between border-b border-slate-100 dark:border-slate-800 pb-3 text-xs font-semibold">
          <button
            onClick={() => setStep(1)}
            className={`flex items-center space-x-2 pb-1 transition ${
              step === 1
                ? 'text-indigo-600 dark:text-indigo-400 border-b-2 border-indigo-600 font-bold'
                : 'text-slate-400 hover:text-slate-700 dark:hover:text-slate-300'
            }`}
          >
            <span className="w-5 h-5 rounded-full bg-indigo-50 dark:bg-indigo-950 border border-indigo-200 dark:border-indigo-800 flex items-center justify-center text-[11px]">
              1
            </span>
            <span>{isZh ? '选择或获取密钥' : 'Select API Key'}</span>
          </button>

          <ArrowRight className="w-3.5 h-3.5 text-slate-300" />

          <button
            onClick={() => setStep(2)}
            className={`flex items-center space-x-2 pb-1 transition ${
              step === 2
                ? 'text-indigo-600 dark:text-indigo-400 border-b-2 border-indigo-600 font-bold'
                : 'text-slate-400 hover:text-slate-700 dark:hover:text-slate-300'
            }`}
          >
            <span className="w-5 h-5 rounded-full bg-indigo-50 dark:bg-indigo-950 border border-indigo-200 dark:border-indigo-800 flex items-center justify-center text-[11px]">
              2
            </span>
            <span>{isZh ? '选择模型与协议' : 'Choose Protocol & Model'}</span>
          </button>

          <ArrowRight className="w-3.5 h-3.5 text-slate-300" />

          <button
            onClick={() => setStep(3)}
            className={`flex items-center space-x-2 pb-1 transition ${
              step === 3
                ? 'text-indigo-600 dark:text-indigo-400 border-b-2 border-indigo-600 font-bold'
                : 'text-slate-400 hover:text-slate-700 dark:hover:text-slate-300'
            }`}
          >
            <span className="w-5 h-5 rounded-full bg-indigo-50 dark:bg-indigo-950 border border-indigo-200 dark:border-indigo-800 flex items-center justify-center text-[11px]">
              3
            </span>
            <span>{isZh ? '代码接入与握手' : 'SDK & Handshake Probe'}</span>
          </button>
        </div>

        {/* Step 1: Keys */}
        {step === 1 && (
          <div className="space-y-4 animate-in fade-in duration-150">
            <div className="bg-slate-50 dark:bg-slate-900/60 p-4 rounded-2xl border border-slate-200/80 dark:border-slate-800/80">
              <label className="block text-xs font-bold text-slate-700 dark:text-slate-300 mb-2 flex items-center space-x-1.5">
                <Key className="w-3.5 h-3.5 text-indigo-500" />
                <span>{isZh ? '选择用于接入调用的 API 密钥 (API Key)' : 'Select API Key for client requests'}</span>
              </label>

              {keys.length > 0 ? (
                <div className="space-y-2">
                  <select
                    value={selectedKey}
                    onChange={(e) => setSelectedKey(e.target.value)}
                    className="w-full bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-xl px-3 py-2 text-xs font-mono text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500"
                  >
                    {keys.map((k) => (
                      <option key={k.id || k.key} value={k.key}>
                        {k.tenant_id ? `[${k.tenant_id}] ` : ''}
                        {k.key.slice(0, 10)}...{k.key.slice(-4)} (RPM: {k.rpm || 60})
                      </option>
                    ))}
                  </select>
                  <p className="text-[11px] text-slate-400">
                    💡 {isZh ? '该密钥已自动关联默认扣费组与全模态访问权限。' : 'This key is automatically mapped to the default billing group and full multimodal scope.'}
                  </p>
                </div>
              ) : (
                <div className="p-3 bg-amber-50 dark:bg-amber-950/40 border border-amber-200 dark:border-amber-800/60 rounded-xl text-xs text-amber-800 dark:text-amber-300">
                  {isZh 
                    ? '当前暂无已签发的 API 密钥。可使用演示临时密钥进行测试，或前往【API 密钥】菜单创建企业级密钥。' 
                    : 'No API keys issued yet. You may use the demo key for testing, or navigate to [API Keys] to create one.'}
                </div>
              )}

              <div className="mt-3 flex items-center justify-between bg-white dark:bg-slate-800 p-2.5 rounded-xl border border-slate-200 dark:border-slate-700 text-xs font-mono">
                <span className="text-slate-600 dark:text-slate-300 truncate max-w-sm">
                  {selectedKey}
                </span>
                <button
                  type="button"
                  onClick={() => copyToClipboard(selectedKey, 'key')}
                  className="px-2.5 py-1 bg-slate-100 dark:bg-slate-700 hover:bg-indigo-50 dark:hover:bg-indigo-950 text-indigo-600 dark:text-indigo-400 rounded-lg font-medium transition flex items-center space-x-1 shrink-0"
                >
                  {copiedKey === 'key' ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                  <span>{copiedKey === 'key' ? (isZh ? '已复制' : 'Copied') : (isZh ? '复制密钥' : 'Copy Key')}</span>
                </button>
              </div>
            </div>

            <div className="flex justify-end pt-2">
              <button
                type="button"
                onClick={() => setStep(2)}
                className="px-5 py-2.5 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-bold shadow-xs transition flex items-center space-x-1.5 cursor-pointer"
              >
                <span>{isZh ? '下一步：选择模型与协议' : 'Next: Protocol & Model'}</span>
                <ArrowRight className="w-3.5 h-3.5" />
              </button>
            </div>
          </div>
        )}

        {/* Step 2: Protocol & Model */}
        {step === 2 && (
          <div className="space-y-4 animate-in fade-in duration-150">
            <div>
              <label className="block text-xs font-bold text-slate-700 dark:text-slate-300 mb-2">
                {isZh ? '客户端 SDK 协议格式' : 'Client SDK Protocol'}
              </label>
              <div className="grid grid-cols-3 gap-3">
                {[
                  { id: 'openai', label: isZh ? 'OpenAI 官方协议' : 'OpenAI Standard', sub: '/v1/chat/completions' },
                  { id: 'claude', label: 'Claude Messages', sub: '/v1/messages' },
                  { id: 'gemini', label: 'Google Gemini', sub: '/v1beta/models/*' },
                ].map((p) => (
                  <button
                    key={p.id}
                    type="button"
                    onClick={() => setSelectedProtocol(p.id)}
                    className={`p-3 rounded-2xl border text-left transition cursor-pointer ${
                      selectedProtocol === p.id
                        ? 'bg-indigo-50/80 dark:bg-indigo-950/60 border-indigo-400 text-indigo-900 dark:text-indigo-200 ring-2 ring-indigo-500/20'
                        : 'bg-white dark:bg-slate-900 border-slate-200 dark:border-slate-800 text-slate-700 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-800'
                    }`}
                  >
                    <div className="font-bold text-xs">{p.label}</div>
                    <div className="text-[10px] text-slate-400 font-mono mt-0.5">{p.sub}</div>
                  </button>
                ))}
              </div>
            </div>

            <div>
              <label className="block text-xs font-bold text-slate-700 dark:text-slate-300 mb-2">
                {isZh ? '测试请求模型 (挂载模型)' : 'Target Model'}
              </label>
              {models.length > 0 ? (
                <select
                  value={selectedModel}
                  onChange={(e) => setSelectedModel(e.target.value)}
                  className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-3 py-2 text-xs font-mono text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500"
                >
                  {models.map((m) => (
                    <option key={m} value={m}>
                      {m}
                    </option>
                  ))}
                </select>
              ) : (
                <input
                  type="text"
                  value={selectedModel}
                  onChange={(e) => setSelectedModel(e.target.value)}
                  placeholder="deepseek-v3 / gpt-4o / claude-3-5-sonnet"
                  className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-3 py-2 text-xs font-mono text-slate-800 dark:text-slate-100"
                />
              )}
              <p className="text-[11px] text-slate-400 mt-1">
                {isZh ? 'Airoute 会根据此模型名称，通过多通道优先级和三态熔断引擎无感分发。' : 'Airoute automatically routes requests across multiple upstream channels with zero-loss fallback.'}
              </p>
            </div>

            <div className="flex justify-between pt-2">
              <button
                type="button"
                onClick={() => setStep(1)}
                className="px-4 py-2 bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-300 rounded-xl text-xs font-semibold transition cursor-pointer"
              >
                {isZh ? '上一步' : 'Back'}
              </button>
              <button
                type="button"
                onClick={() => setStep(3)}
                className="px-5 py-2.5 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-bold shadow-xs transition flex items-center space-x-1.5 cursor-pointer"
              >
                <span>{isZh ? '下一步：代码验证与握手' : 'Next: Code & Handshake'}</span>
                <ArrowRight className="w-3.5 h-3.5" />
              </button>
            </div>
          </div>
        )}

        {/* Step 3: Code & Live Ping */}
        {step === 3 && (
          <div className="space-y-4 animate-in fade-in duration-150">
            {/* Snippet Tabs */}
            <div className="flex items-center justify-between pb-2 border-b border-slate-100 dark:border-slate-800">
              <div className="flex items-center space-x-1.5">
                {[
                  { id: 'python', label: 'Python SDK' },
                  { id: 'typescript', label: 'TypeScript / Node' },
                  { id: 'curl', label: 'cURL' },
                  { id: 'claude', label: 'Claude SDK' },
                ].map((tab) => (
                  <button
                    key={tab.id}
                    type="button"
                    onClick={() => setSnippetTab(tab.id)}
                    className={`px-2.5 py-1 rounded-lg text-xs font-semibold transition cursor-pointer ${
                      snippetTab === tab.id
                        ? 'bg-indigo-600 text-white shadow-xs'
                        : 'text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800'
                    }`}
                  >
                    {tab.label}
                  </button>
                ))}
              </div>

              <button
                type="button"
                onClick={() => {
                  const code =
                    snippetTab === 'python'
                      ? pythonCode
                      : snippetTab === 'typescript'
                      ? tsCode
                      : snippetTab === 'curl'
                      ? curlCode
                      : claudeCode;
                  copyToClipboard(code, snippetTab);
                }}
                className="px-2.5 py-1 bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-200 rounded-lg text-xs font-medium flex items-center space-x-1 transition cursor-pointer"
              >
                {copiedKey === snippetTab ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                <span>{copiedKey === snippetTab ? (isZh ? '已复制' : 'Copied') : (isZh ? '复制代码' : 'Copy Code')}</span>
              </button>
            </div>

            {/* Code Box */}
            <pre className="p-4 bg-slate-900 text-slate-100 rounded-2xl text-xs font-mono overflow-x-auto max-h-56 leading-relaxed">
              {snippetTab === 'python' && pythonCode}
              {snippetTab === 'typescript' && tsCode}
              {snippetTab === 'curl' && curlCode}
              {snippetTab === 'claude' && claudeCode}
            </pre>

            {/* Live Handshake Test */}
            <div className="p-4 bg-indigo-50/70 dark:bg-indigo-950/40 border border-indigo-200/80 dark:border-indigo-800/60 rounded-2xl space-y-2">
              <div className="flex items-center justify-between">
                <div>
                  <span className="font-bold text-xs text-indigo-950 dark:text-indigo-200 flex items-center space-x-1.5">
                    <Sparkles className="w-3.5 h-3.5 text-indigo-600 animate-pulse" />
                    <span>{isZh ? '在线握手探测 (Live Handshake Probe)' : 'Live Handshake Probe'}</span>
                  </span>
                  <p className="text-[11px] text-slate-600 dark:text-slate-400 mt-0.5">
                    {isZh ? '从浏览器直接发送首次轻量请求，实时校验网关连通度与首字时延' : 'Send lightweight probe directly from the browser to verify gateway connectivity and latency'}
                  </p>
                </div>

                <button
                  type="button"
                  onClick={handleTestProbe}
                  disabled={testingPing}
                  className="px-3.5 py-1.5 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-bold transition flex items-center space-x-1.5 shrink-0 shadow-xs cursor-pointer"
                >
                  {testingPing ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Play className="w-3.5 h-3.5" />}
                  <span>{testingPing ? (isZh ? '探测中...' : 'Probing...') : (isZh ? '立即测试握手' : 'Test Handshake')}</span>
                </button>
              </div>

              {testResult && (
                <div
                  className={`mt-2 p-3 rounded-xl text-xs font-mono border ${
                    testResult.success
                      ? 'bg-emerald-50 dark:bg-emerald-950/60 text-emerald-800 dark:text-emerald-200 border-emerald-200 dark:border-emerald-800'
                      : 'bg-rose-50 dark:bg-rose-950/60 text-rose-800 dark:text-rose-200 border-rose-200 dark:border-rose-800'
                  }`}
                >
                  <div className="flex items-center justify-between font-bold">
                    <span>
                      {testResult.success ? (isZh ? '✅ 握手成功！' : '✅ Handshake Successful!') : (isZh ? '❌ 探测返回异常' : '❌ Handshake Error')}
                    </span>
                    <span className="text-[11px] font-mono">
                      {isZh ? '耗时' : 'Latency'}: {testResult.latency} ms
                    </span>
                  </div>
                  <div className="mt-1 text-[11px] truncate">
                    {testResult.success ? testResult.text : testResult.error}
                  </div>
                </div>
              )}
            </div>

            {/* Bottom Actions */}
            <div className="flex items-center justify-between pt-2">
              <button
                type="button"
                onClick={() => setStep(2)}
                className="px-4 py-2 bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-300 rounded-xl text-xs font-semibold transition cursor-pointer"
              >
                {isZh ? '上一步' : 'Back'}
              </button>

              <div className="flex items-center space-x-2">
                <button
                  type="button"
                  onClick={() => {
                    onClose();
                    if (onNavigateToPlayground) {
                      onNavigateToPlayground(selectedModel);
                    }
                  }}
                  className="px-4 py-2.5 bg-indigo-50 dark:bg-indigo-950 hover:bg-indigo-100 dark:hover:bg-indigo-900/60 text-indigo-700 dark:text-indigo-300 border border-indigo-200 dark:border-indigo-800 rounded-xl text-xs font-bold transition flex items-center space-x-1 cursor-pointer"
                >
                  <Terminal className="w-3.5 h-3.5" />
                  <span>{isZh ? '前往调试演练场 →' : 'Go to Playground →'}</span>
                </button>

                <button
                  type="button"
                  onClick={onClose}
                  className="px-5 py-2.5 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-bold shadow-xs transition cursor-pointer"
                >
                  {isZh ? '完成向导' : 'Finish Wizard'}
                </button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
