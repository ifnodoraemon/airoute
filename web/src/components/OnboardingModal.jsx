import { useEffect, useMemo, useRef, useState } from 'react';
import {
  X,
  Sparkles,
  Key,
  Terminal,
  Copy,
  Check,
  ArrowRight,
  Play,
  RefreshCw
} from 'lucide-react';
import { getGatewayOrigin } from '../config';
import useModalA11y from '../hooks/useModalA11y';
import { copyToClipboard as crossOriginCopy } from '../utils/clipboard';
import useFormDirty from '../hooks/useFormDirty';
import useUnsavedGuard from '../hooks/useUnsavedGuard';
import DiscardChangesDialog from './DiscardChangesDialog';
import { availableKeyModels } from '../lib/keyModels';
import { inferClientModality } from '../lib/modelModality';
import { API_KEY_PLACEHOLDER, isUsableAPIKey } from '../lib/apiKey';

export default function OnboardingModal({
  isOpen,
  onClose,
  keys = [],
  models = [],
  onNavigateToPlayground,
  onCreateKey,
  showToast,
  lang = 'zh',
  t
}) {
  const isZh = lang === 'zh';
  const [step, setStep] = useState(1);
  const activeKeys = useMemo(() => keys.filter(key => !key.status || key.status === 'active'), [keys]);
  // Only rows with an id can scope the request; the selector keys off it.
  const contextKeys = useMemo(() => activeKeys.filter(key => key.id != null), [activeKeys]);
  // The wizard keeps two separate selections. `selectedKeyId` picks a listed row
  // for its tenant/model scope only — its `key` is an irreversible display mask.
  // `sessionApiKey` is the plaintext the operator pastes for the live probe and
  // is kept in memory for the lifetime of the modal.
  const [selectedKeyId, setSelectedKeyId] = useState(() => (contextKeys[0] ? String(contextKeys[0].id) : ''));
  const [sessionApiKey, setSessionApiKey] = useState('');
  const [selectedProtocol, setSelectedProtocol] = useState('openai');
  const [selectedModel, setSelectedModel] = useState('');
  const selectedKeyObj = useMemo(() => activeKeys.find(key => String(key.id) === selectedKeyId), [activeKeys, selectedKeyId]);
  const allowedModels = useMemo(() => availableKeyModels(selectedKeyObj, models).filter(name => inferClientModality(name) === 'chat'), [selectedKeyObj, models]);
  const [snippetTab, setSnippetTab] = useState('python');
  const [copiedKey, setCopiedKey] = useState('');
  const [testingPing, setTestingPing] = useState(false);
  const [testResult, setTestResult] = useState(null);

  useEffect(() => {
    if (!contextKeys.some(k => String(k.id) === selectedKeyId)) {
      setSelectedKeyId(contextKeys[0] ? String(contextKeys[0].id) : '');
    }
  }, [contextKeys, selectedKeyId]);

  useEffect(() => {
    if (!allowedModels.includes(selectedModel)) {
      setSelectedModel(allowedModels[0] || '');
    }
  }, [allowedModels, selectedModel]);

  // Only the wizard's configuration is a draft: the step indicator and the
  // snippet tab are navigation/view state, so they never trip the guard.
  const isDirty = useFormDirty(isOpen, { selectedKeyId, selectedProtocol, selectedModel });
  const { confirming, requestClose, keepEditing, discard } = useUnsavedGuard({ isOpen, isDirty, onClose });

  // Remember the configuration the wizard opened with, so finishing a run can
  // restore it and leave nothing dirty behind for the next open.
  const openedDraftRef = useRef(null);
  useEffect(() => {
    if (isOpen) {
      openedDraftRef.current = { selectedKeyId, selectedProtocol, selectedModel };
    }
    // Snapshot only on open; later edits must not overwrite the baseline.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen]);

  // Escape goes through the guard as well (useModalA11y reads the callback via a ref).
  useModalA11y(isOpen, requestClose);

  if (!isOpen) return null;

  const origin = getGatewayOrigin();
  const probeText = isZh ? 'Ping: 首次接入握手测试' : 'Ping: First handshake probe';
  // The credential is a parameter, never a closed-over list value: the snippets
  // get the placeholder, only the live probe gets the pasted plaintext.
  const buildProbe = (apiKey) => (selectedProtocol === 'claude' ? {
    path: '/v1/messages',
    headers: { 'x-api-key': apiKey, 'anthropic-version': '2023-06-01' },
    body: { model: selectedModel, messages: [{ role: 'user', content: probeText }], max_tokens: 30 },
  } : selectedProtocol === 'gemini' ? {
    path: `/v1beta/models/${encodeURIComponent(selectedModel)}:generateContent`,
    headers: { 'x-goog-api-key': apiKey },
    body: { contents: [{ role: 'user', parts: [{ text: probeText }] }], generationConfig: { maxOutputTokens: 30 } },
  } : {
    path: '/v1/chat/completions',
    headers: { Authorization: `Bearer ${apiKey}` },
    body: { model: selectedModel, messages: [{ role: 'user', content: probeText }], max_tokens: 30, stream: false },
  });
  const snippetProbe = buildProbe(API_KEY_PLACEHOLDER);

  // Completion closes for real: restore the opened configuration first so the
  // dirty draft can never re-open the discard prompt on the way out.
  const completeWizard = (after) => {
    const opened = openedDraftRef.current;
    if (opened) {
      setSelectedKeyId(opened.selectedKeyId);
      setSelectedProtocol(opened.selectedProtocol);
      setSelectedModel(opened.selectedModel);
    }
    onClose?.();
    if (after) after();
  };

  const copyToClipboard = async (text, key) => {
    await crossOriginCopy(text);
    setCopiedKey(key || 'snippet');
    setTimeout(() => setCopiedKey(''), 2000);
    if (showToast) showToast(isZh ? '已复制到剪贴板！' : 'Copied to clipboard!', 'success');
  };

  const handleTestProbe = async () => {
    if (!isUsableAPIKey(sessionApiKey)) {
      if (showToast) {
        showToast(
          isZh
            ? '请先粘贴本次会话的 API Key 明文：密钥列表只显示掩码，无法用于握手探测'
            : 'Paste the plaintext API key for this session first: the key list only shows display masks, which cannot authenticate the probe',
          'warning'
        );
      }
      return;
    }
    if (!allowedModels.includes(selectedModel)) return;
    const probe = buildProbe(sessionApiKey.trim());
    setTestingPing(true);
    setTestResult(null);
    const start = Date.now();
    try {
      const res = await fetch(`${origin}${probe.path}`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...probe.headers,
        },
        body: JSON.stringify(probe.body),
      });
      const data = await res.json().catch(() => ({}));
      const duration = Date.now() - start;
      const reply = selectedProtocol === 'claude' ? data.content?.map(part => part.text || '').join('')
        : selectedProtocol === 'gemini' ? data.candidates?.[0]?.content?.parts?.map(part => part.text || '').join('')
        : data.choices?.[0]?.message?.content;
      if (res.ok && reply !== undefined) {
        setTestResult({
          success: true,
          latency: duration,
          text: reply || (isZh ? '通信正常！' : 'Communication healthy!'),
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

  // A stored credential is never rendered: the console shows the plaintext exactly
  // once, in the creation dialog. Every copyable sample carries a placeholder; the
  // live "test ping" button uses the plaintext the operator pasted for this session.
  const KEY_PLACEHOLDER = API_KEY_PLACEHOLDER;

  const pythonCode = `from openai import OpenAI

client = OpenAI(
    base_url="${origin}/v1",
    api_key="${KEY_PLACEHOLDER}",
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
  apiKey: "${KEY_PLACEHOLDER}",
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

  const curlCode = `curl -X POST "${origin}${snippetProbe.path}" \\
  -H "Content-Type: application/json" \\
${Object.entries(snippetProbe.headers).map(([name, value]) => `  -H "${name}: ${value}" \\`).join('\n')}
  -d '${JSON.stringify(snippetProbe.body, null, 2)}'`;

  const claudeCode = `import anthropic

client = anthropic.Anthropic(
    base_url="${origin}",
    api_key="${KEY_PLACEHOLDER}",
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
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/60 backdrop-blur-sm animate-in fade-in duration-200"
      onClick={(e) => {
        if (e.target === e.currentTarget) requestClose();
      }}
    >
      <div className="bg-white dark:bg-[#191B1F] border border-[#E4E6E9] dark:border-[#2C3038] rounded-2xl max-w-2xl w-full p-6 sm:p-7 shadow-2xl relative space-y-6">
        {/* Header */}
        <div className="flex items-start justify-between">
          <div className="flex items-center space-x-3">
            <div className="w-9 h-9 rounded-lg bg-[#355CFF] text-white flex items-center justify-center shadow-sm">
              <Sparkles className="w-4 h-4" />
            </div>
            <div>
              <h2 className="text-base font-semibold text-[#202124] dark:text-[#F1F2F4]">
                {isZh ? '开发者极速上手接入向导' : 'Developer Onboarding Wizard'}
              </h2>
              <p className="text-xs text-[#656B75] dark:text-[#A1A7B0] mt-0.5">
                {isZh ? '3 步完成密钥、代码与连通探测' : '3 steps: key, SDK snippet, live probe'}
              </p>
            </div>
          </div>
          <button
            onClick={requestClose}
            className="p-1.5 rounded-lg text-[#656B75] hover:text-[#202124] dark:hover:text-[#F1F2F4] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] transition cursor-pointer"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Step Indicator */}
        <div className="flex items-center justify-between border-b border-[#E4E6E9] dark:border-[#2C3038] pb-3 text-xs font-semibold">
          <button
            onClick={() => setStep(1)}
            className={`flex items-center space-x-2 pb-1 transition cursor-pointer ${
              step === 1
                ? 'text-[#355CFF] dark:text-[#91A7FF] border-b-2 border-[#355CFF] dark:border-[#91A7FF] font-bold'
                : 'text-[#656B75] hover:text-[#202124] dark:hover:text-[#F1F2F4]'
            }`}
          >
            <span className={`w-5 h-5 rounded-full border flex items-center justify-center text-xs ${
              step === 1
                ? 'bg-[#EEF2FF] dark:bg-[#242D48] border-[#355CFF]/40 text-[#355CFF] dark:text-[#91A7FF]'
                : 'bg-[#F0F1F2] dark:bg-[#22252B] border-[#E4E6E9] dark:border-[#2C3038] text-[#656B75]'
            }`}>
              1
            </span>
            <span>{isZh ? '选择或获取密钥' : 'Select API Key'}</span>
          </button>

          <ArrowRight className="w-3.5 h-3.5 text-[#656B75]/40" />

          <button
            onClick={() => setStep(2)}
            disabled={!selectedKeyId}
            className={`flex items-center space-x-2 pb-1 transition cursor-pointer ${
              step === 2
                ? 'text-[#355CFF] dark:text-[#91A7FF] border-b-2 border-[#355CFF] dark:border-[#91A7FF] font-bold'
                : 'text-[#656B75] hover:text-[#202124] dark:hover:text-[#F1F2F4]'
            }`}
          >
            <span className={`w-5 h-5 rounded-full border flex items-center justify-center text-xs ${
              step === 2
                ? 'bg-[#EEF2FF] dark:bg-[#242D48] border-[#355CFF]/40 text-[#355CFF] dark:text-[#91A7FF]'
                : 'bg-[#F0F1F2] dark:bg-[#22252B] border-[#E4E6E9] dark:border-[#2C3038] text-[#656B75]'
            }`}>
              2
            </span>
            <span>{isZh ? '选择模型与协议' : 'Choose Protocol & Model'}</span>
          </button>

          <ArrowRight className="w-3.5 h-3.5 text-[#656B75]/40" />

          <button
            onClick={() => setStep(3)}
            disabled={!selectedKeyId || !selectedModel}
            className={`flex items-center space-x-2 pb-1 transition cursor-pointer ${
              step === 3
                ? 'text-[#355CFF] dark:text-[#91A7FF] border-b-2 border-[#355CFF] dark:border-[#91A7FF] font-bold'
                : 'text-[#656B75] hover:text-[#202124] dark:hover:text-[#F1F2F4]'
            }`}
          >
            <span className={`w-5 h-5 rounded-full border flex items-center justify-center text-xs ${
              step === 3
                ? 'bg-[#EEF2FF] dark:bg-[#242D48] border-[#355CFF]/40 text-[#355CFF] dark:text-[#91A7FF]'
                : 'bg-[#F0F1F2] dark:bg-[#22252B] border-[#E4E6E9] dark:border-[#2C3038] text-[#656B75]'
            }`}>
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
                <Key className="w-3.5 h-3.5 text-[#355CFF] dark:text-[#91A7FF]" />
                <span>{isZh ? '选择用于接入调用的 API 密钥 (API Key)' : 'Select API Key for client requests'}</span>
              </label>

              {contextKeys.length > 0 ? (
                <div className="space-y-2">
                  <select
                    value={selectedKeyId}
                    onChange={(e) => setSelectedKeyId(e.target.value)}
                    className="ui-field font-mono"
                    aria-label={isZh ? '密钥上下文（租户与模型范围）' : 'Key context (tenant and model scope)'}
                  >
                    {contextKeys.map((k) => (
                      <option key={String(k.id)} value={String(k.id)}>
                        {k.tenant_id ? `[${k.tenant_id}] ` : ''}
                        {k.key_fingerprint || (isZh ? '无指纹' : 'no fingerprint')} (RPM: {k.rpm || 60})
                      </option>
                    ))}
                  </select>
                  <p className="ui-meta">
                    {isZh ? '后续模型列表会按该密钥的授权范围筛选。' : 'Models are filtered by this key’s access permissions.'}
                  </p>
                </div>
              ) : (
                <div className="p-3 bg-amber-50 dark:bg-amber-950/40 border border-amber-200 dark:border-amber-800/60 rounded-lg text-xs text-amber-800 dark:text-amber-300">
                  {isZh 
                    ? '当前没有有效密钥，请先创建或启用 API 密钥。'
                    : 'No active API key. Create or enable a key before testing.'}
                  {onCreateKey && <button type="button" className="ui-btn ui-btn-sm mt-2" onClick={() => completeWizard(onCreateKey)}>{isZh ? '创建 API 密钥' : 'Create API Key'}</button>}
                </div>
              )}

              {/* The mask in the list cannot authenticate anything, so the wizard
                  asks for the plaintext it will actually send. */}
              <div className="mt-3 space-y-1.5">
                <label className="block text-xs font-bold text-slate-700 dark:text-slate-300">
                  {isZh ? '本次会话 API Key 明文 (仅保存在内存)' : 'Session API key (memory only)'}
                </label>
                <input
                  type="password"
                  autoComplete="off"
                  value={sessionApiKey}
                  onChange={(e) => setSessionApiKey(e.target.value)}
                  placeholder={isZh ? '粘贴用于握手探测的明文密钥' : 'Paste the plaintext key used by the live probe'}
                  aria-label={isZh ? '会话 API Key 明文' : 'Session API key'}
                  className="ui-field font-mono"
                />
                <p className="ui-meta">
                  {isZh
                    ? '密钥列表只显示不可还原的掩码；明文仅用于握手探测，不写入浏览器存储。'
                    : 'The key list only shows irreversible display masks. The plaintext is used by the live probe only and is never written to browser storage.'}
                </p>
              </div>
            </div>

            <div className="flex justify-end pt-2">
              <button
                type="button"
                onClick={() => setStep(2)}
                disabled={!selectedKeyId}
                className="ui-btn-primary"
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
                    onClick={() => { setSelectedProtocol(p.id); setSnippetTab(p.id === 'claude' ? 'claude' : p.id === 'gemini' ? 'curl' : 'python'); }}
                    className={`p-3 rounded-lg border text-left transition cursor-pointer ${
                      selectedProtocol === p.id
                        ? 'bg-[#EEF2FF] dark:bg-[#242D48] border-[#355CFF] text-[#202124] dark:text-[#F1F2F4] ring-1 ring-[#355CFF]'
                        : 'bg-white dark:bg-[#191B1F] border-[#E4E6E9] dark:border-[#2C3038] text-[#656B75] dark:text-[#A1A7B0] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B]'
                    }`}
                  >
                    <div className="font-semibold text-xs">{p.label}</div>
                    <div className="ui-meta font-mono mt-0.5">{p.sub}</div>
                  </button>
                ))}
              </div>
            </div>

            <div>
              <label className="block text-xs font-bold text-slate-700 dark:text-slate-300 mb-2">
                {isZh ? '测试请求模型 (挂载模型)' : 'Target Model'}
              </label>
              {allowedModels.length > 0 ? (
                <select
                  value={selectedModel}
                  onChange={(e) => setSelectedModel(e.target.value)}
                  className="w-full bg-[#F7F7F5] dark:bg-[#22252B] border border-[#E4E6E9] dark:border-[#2C3038] rounded-lg px-3 py-2 text-xs font-mono text-[#202124] dark:text-[#F1F2F4] focus:outline-none focus:border-[#355CFF]"
                >
                  {allowedModels.map((m) => (
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
                  disabled
                  placeholder={isZh ? '该密钥暂无可用聊天模型' : 'No available chat model for this key'}
                  className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-lg px-3 py-2 text-xs font-mono text-slate-800 dark:text-slate-100"
                />
              )}
              <p className="ui-meta mt-1">
                {isZh ? 'Airoute 会根据此模型名称，通过多通道优先级和三态熔断引擎无感分发。' : 'Airoute automatically routes requests across multiple upstream channels with zero-loss fallback.'}
              </p>
            </div>

            <div className="flex justify-between pt-2">
              <button
                type="button"
                onClick={() => setStep(1)}
                className="px-4 py-2 bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-300 rounded-lg text-xs font-medium transition cursor-pointer"
              >
                {isZh ? '上一步' : 'Back'}
              </button>
              <button
                type="button"
                onClick={() => setStep(3)}
                disabled={!selectedModel}
                className="ui-btn-primary"
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
                    className={`px-2.5 py-1 rounded-lg text-xs font-medium transition cursor-pointer ${
                      snippetTab === tab.id
                        ? 'bg-[#355CFF] text-white shadow-sm'
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
            <pre className="p-4 bg-slate-900 text-slate-100 rounded-xl text-xs font-mono overflow-x-auto max-h-56 leading-relaxed">
              {snippetTab === 'python' && pythonCode}
              {snippetTab === 'typescript' && tsCode}
              {snippetTab === 'curl' && curlCode}
              {snippetTab === 'claude' && claudeCode}
            </pre>

            {/* Live Handshake Test */}
            <div className="p-4 bg-[#F0F1F2]/60 dark:bg-[#22252B]/60 border border-[#E4E6E9] dark:border-[#2C3038] rounded-xl space-y-2">
              <div className="flex items-center justify-between">
                <div>
                  <span className="font-semibold text-xs text-[#202124] dark:text-[#F1F2F4] flex items-center space-x-1.5">
                    <Sparkles className="w-3.5 h-3.5 text-[#355CFF] dark:text-[#91A7FF]" />
                    <span>{isZh ? '在线握手探测 (Live Handshake Probe)' : 'Live Handshake Probe'}</span>
                  </span>
                  <p className="ui-meta mt-0.5">
                    {isZh ? '从浏览器直接发送首次轻量请求，实时校验网关连通度与首字时延' : 'Send lightweight probe directly from the browser to verify gateway connectivity and latency'}
                  </p>
                </div>

                <button
                  type="button"
                  onClick={handleTestProbe}
                  disabled={testingPing || !isUsableAPIKey(sessionApiKey) || !allowedModels.includes(selectedModel)}
                  className="px-3.5 py-1.5 bg-[#355CFF] hover:bg-[#2849E6] text-white rounded-lg text-xs font-medium transition flex items-center space-x-1.5 shrink-0 shadow-sm cursor-pointer"
                >
                  {testingPing ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Play className="w-3.5 h-3.5" />}
                  <span>{testingPing ? (isZh ? '探测中...' : 'Probing...') : (isZh ? '立即测试握手' : 'Test Handshake')}</span>
                </button>
              </div>

              {/* The probe is where the credential is needed, so it is editable
                  here too (same in-memory state as step 1). */}
              <div className="flex items-center gap-2">
                <Key className="w-3.5 h-3.5 text-[#355CFF] dark:text-[#91A7FF] shrink-0" />
                <input
                  type="password"
                  autoComplete="off"
                  value={sessionApiKey}
                  onChange={(e) => setSessionApiKey(e.target.value)}
                  placeholder={isZh ? '粘贴明文 API Key 后才能握手 (仅内存)' : 'Paste the plaintext API key to enable the probe (memory only)'}
                  aria-label={isZh ? '会话 API Key 明文' : 'Session API key'}
                  className="ui-field font-mono text-xs flex-1"
                />
              </div>

              {testResult && (
                <div
                  className={`mt-2 p-3 rounded-lg text-xs font-mono border ${
                    testResult.success
                      ? 'bg-emerald-50 dark:bg-emerald-950/60 text-emerald-800 dark:text-emerald-200 border-emerald-200 dark:border-emerald-800'
                      : 'bg-rose-50 dark:bg-rose-950/60 text-rose-800 dark:text-rose-200 border-rose-200 dark:border-rose-800'
                  }`}
                >
                  <div className="flex items-center justify-between font-semibold">
                    <span>
                      {testResult.success ? (isZh ? '握手成功' : 'Handshake Successful') : (isZh ? '探测返回异常' : 'Handshake Error')}
                    </span>
                    <span className="text-xs font-mono tabular-nums">
                      {isZh ? '耗时' : 'Latency'}: {testResult.latency} ms
                    </span>
                  </div>
                  <div className="mt-1 text-xs truncate">
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
                className="px-4 py-2 bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-300 rounded-lg text-xs font-medium transition cursor-pointer"
              >
                {isZh ? '上一步' : 'Back'}
              </button>

              <div className="flex items-center space-x-2">
                <button
                  type="button"
                  onClick={() => completeWizard(() => {
                    if (onNavigateToPlayground) {
                      onNavigateToPlayground(selectedModel);
                    }
                  })}
                  className="px-4 py-2 bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-300 border border-slate-200 dark:border-slate-700 rounded-lg text-xs font-medium transition flex items-center space-x-1 cursor-pointer"
                >
                  <Terminal className="w-3.5 h-3.5" />
                  <span>{isZh ? '前往调试演练场 →' : 'Go to Playground →'}</span>
                </button>

                <button
                  type="button"
                  onClick={() => completeWizard()}
                  className="ui-btn-primary"
                >
                  {isZh ? '完成向导' : 'Finish Wizard'}
                </button>
              </div>
            </div>
          </div>
        )}

        <DiscardChangesDialog
          open={confirming}
          onKeepEditing={keepEditing}
          onDiscard={discard}
          lang={lang}
          title={isZh ? '放弃未完成的接入向导配置？' : 'Discard this onboarding setup?'}
        />
      </div>
    </div>
  );
}
