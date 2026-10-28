import { useState, useEffect, useMemo, useRef } from 'react';
import {
  MessageSquare,
  Image as ImageIcon,
  Volume2,
  Mic,
  Video,
  Cpu,
  Sliders,
  Sparkles,
  Key,
  Code,
  Terminal,
  RefreshCw,
  Download,
  FileAudio,
  Send,
  Split,
  Square,
  ChevronDown,
  ChevronUp,
  Clock,
  Zap
} from 'lucide-react';
import { getGatewayOrigin, copyToClipboard as crossOriginCopy } from '../config';
import MarkdownRenderer from './MarkdownRenderer';
import { inferClientModality } from '../lib/modelModality';
import { availableKeyModels } from '../lib/keyModels';
import {
  API_KEY_PLACEHOLDER,
  UNAUTHENTICATED_KEY,
  credentialProblem,
  credentialProblemHint,
  isUsableAPIKey,
  keyDisplayRef
} from '../lib/apiKey';

export default function PlaygroundView({
  keys = [],
  models = [],
  modelRoutes = [],
  prices = [],
  showToast,
  initialModel = '',
  initialModality = 'chat',
  lang = 'zh',
  t
}) {
  const isZh = lang === 'zh';
  const [playModality, setPlayModality] = useState(initialModality || 'chat');

  const [playMode, setPlayMode] = useState('single'); // 'single' | 'arena'
  // Two different things are selected here, and conflating them was the bug:
  //   * `selectedKeyId` picks a row from the key list for its *context*
  //     (tenant + allowed models). It exposes a key ID, never a credential.
  //   * `sessionApiKey` is the plaintext credential the operator pastes for this
  //     session; it signs the actual request and lives only in component memory.
  const activeKeys = useMemo(() => keys.filter(key => !key.status || key.status === 'active'), [keys]);
  // Only rows with an id can scope the request; the selector keys off it.
  const contextKeys = useMemo(() => activeKeys.filter(key => key.id != null), [activeKeys]);
  const [selectedKeyId, setSelectedKeyId] = useState(() => (
    contextKeys[0] ? String(contextKeys[0].id) : UNAUTHENTICATED_KEY
  ));
  const [sessionApiKey, setSessionApiKey] = useState('');

  // Auto-select the first key for entitlement context, keyed off its id. A console
  // with no keys configured keeps its historical public-gateway behaviour, but as
  // an explicit unauthenticated choice rather than an empty credential.
  useEffect(() => {
    if (selectedKeyId === UNAUTHENTICATED_KEY) return;
    if (!contextKeys.some(key => String(key.id) === selectedKeyId)) {
      setSelectedKeyId(contextKeys[0] ? String(contextKeys[0].id) : UNAUTHENTICATED_KEY);
    }
  }, [contextKeys, selectedKeyId]);

  const selectedKeyObj = useMemo(
    () => keys.find(k => String(k.id) === selectedKeyId),
    [keys, selectedKeyId]
  );

  // Resolves the credential for an outbound call. `null` means the call must be
  // refused: a listed key was chosen but its plaintext was never pasted, and
  // sending the listed key ID would fail authentication while looking like a gateway
  // fault. The explicit "unauthenticated" choice returns an empty key (no header).
  const resolveCallCredentials = () => {
    if (selectedKeyId === UNAUTHENTICATED_KEY) return { key: '' };
    const typed = sessionApiKey.trim();
    if (isUsableAPIKey(typed)) return { key: typed };
    // Name the actual defect: a key ID copied from the list and a missing value
    // need different remedies, and both used to surface as a gateway 401.
    showToast?.(credentialProblemHint(credentialProblem(typed), isZh), 'warning');
    return null;
  };

  const getPlaygroundModels = (modality) => {
    let list = [];
    if (modelRoutes && modelRoutes.length > 0) {
      const matched = modelRoutes.filter(r => r.modality === modality).map(r => r.model);
      if (matched.length > 0) list = matched;
    }
    if (list.length === 0 && models && models.length > 0) {
      const matched = models.filter(m => inferClientModality(m) === modality);
      if (matched.length > 0) list = matched;
      else if (modality === 'chat') list = models.filter(m => inferClientModality(m) === 'chat');
    }

    if (selectedKeyObj && selectedKeyObj.allowed_models && selectedKeyObj.allowed_models.length > 0) {
      if (!selectedKeyObj.allowed_models.includes('*')) {
        return availableKeyModels(selectedKeyObj, list);
      }
    }
    return list;
  };

  const chatModels = getPlaygroundModels('chat');
  const imgModels = getPlaygroundModels('images');
  const ttsModels = getPlaygroundModels('audio_speech');
  const sttModels = getPlaygroundModels('audio_transcription');
  const videoModels = getPlaygroundModels('videos');
  const embedModels = getPlaygroundModels('embeddings');
  const rerankModels = getPlaygroundModels('rerank');

  // Single Model state
  const [playModel, setPlayModel] = useState(initialModel || (chatModels[0] || ''));
  const [arenaModelB, setArenaModelB] = useState(chatModels[1] || chatModels[0] || '');

  // Real off-peak state for the currently selected chat model.
  //
  // The footer used to print a hardcoded "闲时费率 5 折激活中" banner regardless
  // of the model's actual configuration, which is a claim the UI cannot support
  // without the pricing row. We now read it from the pricing data and show
  // nothing when the model has no off-peak discount configured.
  //
  // This must stay below the playModel declaration: reading a `const` state
  // binding before its initialization throws a TDZ ReferenceError that kills
  // the whole playground on mount.
  const activeChatModel = playModel || (models.length > 0 ? models[0] : '');
  const activePrice = prices.find(p => p.model === activeChatModel && (p.group_name || 'default') === 'default')
    || prices.find(p => p.model === activeChatModel);
  const offPeak = (() => {
    if (!activePrice || activePrice.off_peak_enabled === false) return null;
    const discount = typeof activePrice.off_peak_discount === 'number' ? activePrice.off_peak_discount : null;
    if (discount === null || discount >= 1) return null;
    return { discount, label: `${Math.round(discount * 10)}` };
  })();
  const [playStream, setPlayStream] = useState(true);
  const [temperature, setTemperature] = useState(0.7);
  const [maxTokens, setMaxTokens] = useState(2048);
  const [systemPrompt, setSystemPrompt] = useState(
    isZh ? '你是一个极具专业素养的 AI 架构专家与全栈工程师，回答请严谨、精炼、富有洞见。' : 'You are a highly professional AI architecture expert and full-stack engineer. Answer concisely and with precision.'
  );
  const [showSystemPrompt, setShowSystemPrompt] = useState(false);

  // Chat conversation history & streaming output
  const [messages, setMessages] = useState([]);
  const [chatInput, setChatInput] = useState('');
  const [playLoading, setPlayLoading] = useState(false);
  // `playStopped` records a user-initiated cancellation so the UI can show a
  // neutral state instead of an error; `abortRef` holds the in-flight request.
  const [playStopped, setPlayStopped] = useState(false);
  const abortRef = useRef(null);
  useEffect(() => () => abortRef.current?.abort(), []);
  const chatInputRef = useRef(null);

  // Single Model Output & Live Telemetry
  const [playOutput, setPlayOutput] = useState('');
  const [playReasoningOutput, setPlayReasoningOutput] = useState('');
  const [playTTFTMs, setPlayTTFTMs] = useState(0);
  const [playDurationMs, setPlayDurationMs] = useState(0);
  const [playTokensPerSec, setPlayTokensPerSec] = useState(0);

  // Arena Contender B Output & Telemetry
  const [arenaOutputB, setArenaOutputB] = useState('');
  const [arenaReasoningB, setArenaReasoningB] = useState('');
  const [arenaTTFTB, setArenaTTFTB] = useState(0);
  const [arenaDurationB, setArenaDurationB] = useState(0);
  const [arenaTokensPerSecB, setArenaTokensPerSecB] = useState(0);

  // Multimodal sub-states
  const [imgModel, setImgModel] = useState(imgModels[0] || '');
  const [imgPrompt, setImgPrompt] = useState('');
  const [imgSize, setImgSize] = useState('1024x1024');
  const [imgQuality] = useState('standard');
  const [imgResult, setImgResult] = useState(null);

  const [ttsModel, setTtsModel] = useState(ttsModels[0] || '');
  const [ttsVoice, setTtsVoice] = useState('alloy');
  const [ttsSpeed] = useState(1.0);
  const [ttsInput, setTtsInput] = useState('');
  const [ttsAudioUrl, setTtsAudioUrl] = useState(null);
  // Mirror of the synthesised audio object URL so unmount can release it. The
  // setter below only revokes the *previous* URL; without this cleanup, leaving
  // the view leaked one Blob URL per TTS generation.
  const ttsAudioUrlRef = useRef(null);
  useEffect(() => {
    ttsAudioUrlRef.current = ttsAudioUrl;
  }, [ttsAudioUrl]);
  useEffect(() => () => {
    if (ttsAudioUrlRef.current) URL.revokeObjectURL(ttsAudioUrlRef.current);
  }, []);

  // STT (audio transcription)
  const [sttModel, setSttModel] = useState(sttModels[0] || '');
  const [sttFile, setSttFile] = useState(null);
  const [sttLanguage, setSttLanguage] = useState('');
  const [sttResult, setSttResult] = useState(null);

  // Video generation (async task + polling)
  const [videoModel, setVideoModel] = useState(videoModels[0] || '');
  const [videoPrompt, setVideoPrompt] = useState('');
  const [videoResult, setVideoResult] = useState(null);
  const [videoTaskId, setVideoTaskId] = useState('');

  // Embeddings
  const [embedModel, setEmbedModel] = useState(embedModels[0] || '');
  const [embedInput, setEmbedInput] = useState('');
  const [embedResult, setEmbedResult] = useState(null);

  // Rerank
  const [rerankModel, setRerankModel] = useState(rerankModels[0] || '');
  const [rerankQuery, setRerankQuery] = useState('');
  const [rerankDocs, setRerankDocs] = useState('');
  const [rerankResult, setRerankResult] = useState(null);

  useEffect(() => {
    const setters = { chat: setPlayModel, images: setImgModel, audio_speech: setTtsModel, audio_transcription: setSttModel, videos: setVideoModel, embeddings: setEmbedModel, rerank: setRerankModel };
    if (initialModel) (setters[initialModality] || setPlayModel)(initialModel);
    setPlayModality(initialModality || 'chat');
  }, [initialModel, initialModality]);

  // Sync initial model across chat and multimodal categories
  useEffect(() => {
    if (chatModels.length > 0 && !chatModels.includes(playModel)) {
      setPlayModel(chatModels[0]);
    }
    if (chatModels.length > 1 && !chatModels.includes(arenaModelB)) {
      setArenaModelB(chatModels[1]);
    }
    if (imgModels.length > 0 && !imgModels.includes(imgModel)) {
      setImgModel(imgModels[0]);
    }
    if (ttsModels.length > 0 && !ttsModels.includes(ttsModel)) {
      setTtsModel(ttsModels[0]);
    }
    if (sttModels.length > 0 && !sttModels.includes(sttModel)) {
      setSttModel(sttModels[0]);
    }
    if (videoModels.length > 0 && !videoModels.includes(videoModel)) {
      setVideoModel(videoModels[0]);
    }
    if (embedModels.length > 0 && !embedModels.includes(embedModel)) {
      setEmbedModel(embedModels[0]);
    }
    if (rerankModels.length > 0 && !rerankModels.includes(rerankModel)) {
      setRerankModel(rerankModels[0]);
    }
  }, [
    chatModels, playModel, arenaModelB,
    imgModels, imgModel,
    ttsModels, ttsModel,
    sttModels, sttModel,
    videoModels, videoModel,
    embedModels, embedModel,
    rerankModels, rerankModel
  ]);

  const copyToClipboard = async (txt, label = '') => {
    await crossOriginCopy(txt);
    if (showToast) showToast(isZh ? `${label || '内容'} 已复制到剪贴板！` : `${label || 'Content'} copied to clipboard!`, 'success');
  };

  const handleOptimizeSystemPrompt = () => {
    const base = systemPrompt.trim() || (isZh ? '全栈技术专家与架构师' : 'Expert Full-Stack Architect');
    const optimized = isZh
      ? `# 角色定位 (Role)
你是一名顶尖的企业级 AI 架构师与专业任务执行专家，具备深厚工程化落地与严谨的逻辑推理能力。

# 核心任务 (Objective)
深度理解并高精度执行用户指令，兼顾代码健壮性、架构拓展性与清晰可维护性：
【${base}】

# 上下文约束与最佳实践 (Constraints)
1. 严禁捏造事实或虚构不存在的技术参数（零幻觉原则）。
2. 如涉及关键数据或技术选型，必须给出可量化的决策权衡依据。
3. 遵循安全性与合规性原则，避免返回不合规的高危操作指令。

# 输出规范 (Output Format)
- 采用清晰的 GitHub Markdown 格式组织。
- 若包含代码，需附带详尽的关键行注释。
- 提供结构化结论及后续可直接行动项 (Action Items)。`
      : `# Role
You are a distinguished enterprise AI architect and software engineer with deep technical rigor.

# Objective
Execute the following mission with precision, maintainability, and zero speculation:
[${base}]

# Constraints
1. Zero-hallucination principle: Do not invent non-existent APIs or parameters.
2. Provide measurable trade-offs for all architectural decisions.
3. Adhere to strict security and safety standards.

# Output Format
- GitHub Markdown formatting with syntax-highlighted code blocks.
- Actionable next steps.`;

    setSystemPrompt(optimized);
    setShowSystemPrompt(true);
    if (showToast) showToast(isZh ? '系统人设已完成 AI 结构化工程增强！' : 'System Prompt enhanced with AI prompt engineering!', 'success');
  };

  const handleOptimizeChatInput = () => {
    if (!chatInput.trim()) return;
    const base = chatInput.trim();
    const optimized = isZh
      ? `请针对以下需求展开专业、深入且结构化的解答：\n\n【核心目标】：${base}\n\n【具体要求】：\n1. 阐明核心底层原理与技术选型权衡\n2. 给出经过生产验证的代码实现或最佳实践架构方案\n3. 提示潜在边界陷阱（如高并发、网络延迟、内存开销）及对应防御措施`
      : `Please provide a rigorous, in-depth, and structured response for:\n\n[Core Goal]: ${base}\n\n[Requirements]:\n1. Explain underlying principles and architectural trade-offs\n2. Provide production-grade implementation or configuration examples\n3. Highlight edge-case pitfalls (concurrency, latency, failure recovery) and remediations`;

    setChatInput(optimized);
    if (showToast) showToast(isZh ? '输入 Prompt 已完成 AI 结构化规范增强！' : 'Prompt enriched with structural requirements!', 'success');
  };

  // Auto-grow composer: starts at one row, expands with the content up to
  // ~200px, then scrolls internally.
  useEffect(() => {
    const el = chatInputRef.current;
    if (!el) return;
    el.style.height = 'auto';
    el.style.height = `${Math.min(el.scrollHeight, 200)}px`;
  }, [chatInput, playModality]);

  /* -------------------------------------------------------------------------
     Request builders — the single source of truth for every outbound call.
     Both the fetch handlers below and "Copy cURL" consume these, so the copied
     command can never drift away from what the playground actually sends.
     ------------------------------------------------------------------------- */
  const buildHeaders = (key) => {
    const headers = { 'Content-Type': 'application/json' };
    if (key) headers['Authorization'] = `Bearer ${key}`;
    return headers;
  };

  const buildVideoRequest = () => ({
    method: 'POST',
    path: '/v1/videos/generations',
    body: {
      model: videoModel,
      prompt: videoPrompt
    }
  });

  const buildEmbeddingRequest = () => ({
    method: 'POST',
    path: '/v1/embeddings',
    body: {
      model: embedModel,
      // One line per input keeps batch embedding reproducible and obvious.
      input: embedInput.split('\n').map(t => t.trim()).filter(Boolean)
    }
  });

  const buildRerankRequest = () => ({
    method: 'POST',
    path: '/v1/rerank',
    body: {
      model: rerankModel,
      query: rerankQuery,
      documents: rerankDocs.split('\n').map(t => t.trim()).filter(Boolean),
      top_n: 3,
      return_documents: true
    }
  });

  const withSystemPrompt = (msgs) => [
    ...(systemPrompt.trim() ? [{ role: 'system', content: systemPrompt.trim() }] : []),
    ...msgs
  ];

  const buildChatRequest = (targetModel, payloadMessages) => {
    const body = {
      model: targetModel,
      messages: payloadMessages,
      temperature,
      stream: playStream
    };
    const parsedTokens = parseInt(maxTokens, 10);
    if (!isNaN(parsedTokens) && parsedTokens > 0) {
      body.max_tokens = parsedTokens;
    }
    return { method: 'POST', path: '/v1/chat/completions', body };
  };

  const buildImageRequest = () => ({
    method: 'POST',
    path: '/v1/images/generations',
    body: {
      model: imgModel,
      prompt: imgPrompt,
      size: imgSize,
      quality: imgQuality
    }
  });

  const buildTtsRequest = () => ({
    method: 'POST',
    path: '/v1/audio/speech',
    body: {
      model: ttsModel,
      input: ttsInput,
      voice: ttsVoice,
      speed: ttsSpeed,
      response_format: 'mp3'
    }
  });

  const isAbortError = (e) => !!e && (e.name === 'AbortError' || e.code === 20);

  const handleStopGeneration = () => {
    const controller = abortRef.current;
    if (!controller) return;
    setPlayStopped(true);
    try {
      controller.abort();
    } catch (e) {
      // Already settled — nothing left to cancel.
    }
  };

  /* -------------------------------------------------------------------------
     Copy cURL — mirrors the payload the current modality would actually send.
     ------------------------------------------------------------------------- */
  const shellQuote = (value) => `'${String(value).replace(/'/g, "'\\''")}'`;

  const formatCurl = (request, extraFlags) => {
    const typed = sessionApiKey.trim();
    const lines = [
      `curl -X ${request.method} "${getGatewayOrigin()}${request.path}"`,
      '  -H "Content-Type: application/json"',
    ];
    // Never echo a listed key ID into a snippet. Use the pasted plaintext when it is
    // usable, an obvious placeholder when it is not, and no auth header at all
    // for the explicit unauthenticated mode (which is what the request sends).
    if (selectedKeyId !== UNAUTHENTICATED_KEY) {
      const snippetKey = isUsableAPIKey(typed) ? typed : API_KEY_PLACEHOLDER;
      lines.push(`  -H "Authorization: Bearer ${snippetKey}"`);
    }
    if (extraFlags) lines.push(`  ${extraFlags}`);
    lines.push(`  -d ${shellQuote(JSON.stringify(request.body))}`);
    return lines.join(' \\\n');
  };

  // A multipart upload (STT) or a modality without request parameters cannot be
  // reproduced faithfully, so the button is disabled instead of emitting a
  // misleading chat request.
  const getCurlUnsupportedReason = () => {
    if (playModality === 'chat' || playModality === 'images' || playModality === 'audio_speech') return '';
    if (playModality === 'audio_transcription') {
      return isZh
        ? '语音转写需要上传音频文件 (multipart/form-data)，无法用一条 cURL 表达'
        : 'Speech-to-text uploads an audio file (multipart/form-data) and cannot be expressed as a single cURL';
    }
    return isZh
      ? '该模态在本面板尚未提供请求参数，无法生成可复现的 cURL'
      : 'This modality exposes no request parameters here, so a reproducible cURL cannot be generated';
  };

  const handleCopyCurl = () => {
    if (getCurlUnsupportedReason()) return;

    let calls = [];
    if (playModality === 'chat') {
      const pending = chatInput.trim();
      const payloadMessages = withSystemPrompt(
        pending ? [...messages, { role: 'user', content: pending }] : messages
      );
      const targets = playMode === 'arena' && arenaModelB && arenaModelB !== playModel
        ? [playModel, arenaModelB]
        : [playModel];
      calls = targets.map(m => ({ request: buildChatRequest(m, payloadMessages), flags: playStream ? '-N' : '' }));
    } else if (playModality === 'images') {
      calls = [{ request: buildImageRequest(), flags: '' }];
    } else {
      calls = [{ request: buildTtsRequest(), flags: '' }];
    }

    // The tenant is bound to the key; keep it as a comment rather than inventing
    // a header the request never sends.
    const tenant = selectedKeyObj?.tenant_id;
    const tenantNote = tenant ? `# tenant: ${tenant}\n` : '';
    copyToClipboard(`${tenantNote}${calls.map(c => formatCurl(c.request, c.flags)).join('\n\n')}`, 'cURL');
  };

  // Run chat invocation
  const handleRunChat = async () => {
    const credentials = resolveCallCredentials();
    if (!credentials) return;
    const typed = chatInput.trim();
    let newMessages;
    if (typed) {
      newMessages = [...messages, { role: 'user', content: typed }];
    } else {
      // Nothing typed: re-run the existing conversation. The previous code took
      // the last message's content and appended it again, sending the same user
      // turn twice. Only a conversation ending in a user turn can be re-run.
      const last = messages[messages.length - 1];
      if (!last || last.role !== 'user') return;
      newMessages = messages;
    }
    if (newMessages.length === 0) return;

    setMessages(newMessages);
    setChatInput('');
    setPlayLoading(true);
    setPlayStopped(false);

    const controller = new AbortController();
    abortRef.current = controller;

    // Reset Model A
    setPlayOutput('');
    setPlayReasoningOutput('');
    setPlayTTFTMs(0);
    setPlayDurationMs(0);
    setPlayTokensPerSec(0);

    // Reset Model B (Arena)
    if (playMode === 'arena') {
      setArenaOutputB('');
      setArenaReasoningB('');
      setArenaTTFTB(0);
      setArenaDurationB(0);
      setArenaTokensPerSecB(0);
    }

    const payloadMessages = withSystemPrompt(newMessages);

    const runModelStream = async (targetModel, setOutput, setReasoning, setTTFT, setDuration, setTPS) => {
      const startTime = Date.now();
      let firstByteTime = null;
      let fullContent = '';
      let fullReasoning = '';
      let serverReportedTokens = 0;

      try {
        const request = buildChatRequest(targetModel, payloadMessages);

        const res = await fetch(request.path, {
          method: request.method,
          headers: buildHeaders(credentials.key),
          body: JSON.stringify(request.body),
          signal: controller.signal
        });

        if (!res.ok) {
          const errData = await res.json().catch(() => ({}));
          setOutput(`[Error ${res.status}] ${errData.error?.message || res.statusText}`);
          showToast?.(errData.error?.message || res.statusText, 'error');
          return { failed: true };
        }

        if (playStream && res.body) {
          const reader = res.body.getReader();
          const decoder = new TextDecoder('utf-8');
          let done = false;
          let buffer = '';

          const applyChunk = (parsed) => {
            const delta = parsed.choices?.[0]?.delta;
            if (delta?.reasoning_content) {
              fullReasoning += delta.reasoning_content;
              setReasoning(fullReasoning);
            }
            if (delta?.content) {
              fullContent += delta.content;
              setOutput(fullContent);
            }
          };

          const handleLine = (line) => {
            const trimmed = line.trim();
            // Accept both "data: {...}" and "data:{...}".
            if (!trimmed.startsWith('data:')) return;
            const dataStr = trimmed.slice(5).trim();
            if (!dataStr || dataStr === '[DONE]') return;
            let parsed;
            try { parsed = JSON.parse(dataStr); } catch { return; }
            if (parsed.error) throw new Error(parsed.error.message || String(parsed.error));
            if (parsed.usage?.completion_tokens) serverReportedTokens = parsed.usage.completion_tokens;
            applyChunk(parsed);
          };

          try {
          while (!done) {
            const { value, done: readerDone } = await reader.read();
            done = readerDone;
            if (value) {
              if (!firstByteTime) {
                firstByteTime = Date.now();
                setTTFT(firstByteTime - startTime);
              }
              buffer += decoder.decode(value, { stream: true });
              const lines = buffer.split('\n');
              buffer = lines.pop() || '';
              for (const line of lines) handleLine(line);
            }
          }
          // Flush a trailing event that arrived without a final newline.
          if (buffer.trim()) handleLine(buffer);
          } finally {
            try { await reader.cancel?.(); } catch { /* The stream may already be closed. */ }
            reader.releaseLock?.();
          }
        } else {
          const data = await res.json();
          firstByteTime = Date.now();
          setTTFT(firstByteTime - startTime);
          if (data.usage?.completion_tokens) {
            serverReportedTokens = data.usage.completion_tokens;
          }
          const choice = data.choices?.[0];
          if (choice?.message?.reasoning_content) {
            fullReasoning = choice.message.reasoning_content;
            setReasoning(fullReasoning);
          }
          fullContent = choice?.message?.content || '';
          setOutput(fullContent);
        }

        const totalMs = Date.now() - startTime;
        setDuration(totalMs);
        const tokensCount = serverReportedTokens > 0 ? serverReportedTokens : (fullContent.length * 0.75);
        const activeSec = (totalMs - (firstByteTime ? firstByteTime - startTime : 0)) / 1000;
        if (activeSec > 0.1) {
          setTPS((tokensCount / activeSec).toFixed(1));
        }

        return { content: fullContent, reasoning: fullReasoning };
      } catch (e) {
        // A user-initiated stop is not an error: keep whatever arrived and stay quiet.
        if (isAbortError(e)) {
          return { content: fullContent, reasoning: fullReasoning, aborted: true };
        }
        showToast?.(e.message, 'error');
        if (!fullContent) setOutput(`[Error] ${e.message}`);
        return { content: fullContent, reasoning: fullReasoning, failed: true };
      }
    };

    // Run Model A
    const promises = [runModelStream(playModel, setPlayOutput, setPlayReasoningOutput, setPlayTTFTMs, setPlayDurationMs, setPlayTokensPerSec)];
    // If Arena mode, run Model B in parallel!
    if (playMode === 'arena' && arenaModelB) {
      promises.push(runModelStream(arenaModelB, setArenaOutputB, setArenaReasoningB, setArenaTTFTB, setArenaDurationB, setArenaTokensPerSecB));
    }

    try {
      const results = await Promise.all(promises);
      const aborted = results.some(r => r && r.aborted);

      // If single model chat mode, commit completed response into conversational
      // messages. A stopped run keeps its partial output in the live bubble
      // instead of being committed as a complete assistant turn.
      if (!aborted && !results.some(r => r?.failed) && playMode === 'single' && results[0]?.content) {
        setMessages(prev => [
          ...prev,
          {
            role: 'assistant',
            content: results[0].content,
            reasoning: results[0].reasoning
          }
        ]);
        setPlayOutput('');
        setPlayReasoningOutput('');
      }
    } finally {
      setPlayLoading(false);
      abortRef.current = null;
    }
  };

  // Image generation
  const handleGenerateImage = async () => {
    const credentials = resolveCallCredentials();
    if (!credentials) return;
    setPlayLoading(true);
    setPlayStopped(false);
    setImgResult(null);
    const controller = new AbortController();
    abortRef.current = controller;
    const start = Date.now();
    try {
      const request = buildImageRequest();
      const res = await fetch(request.path, {
        method: request.method,
        headers: buildHeaders(credentials.key),
        body: JSON.stringify(request.body),
        signal: controller.signal
      });
      const data = await res.json();
      setPlayDurationMs(Date.now() - start);
      if (res.ok && data.data && data.data[0]) {
        setImgResult(data.data[0]);
      } else {
        if (showToast) showToast(isZh ? `绘图失败: ${data.error?.message || '未知错误'}` : `Image generation failed: ${data.error?.message || 'Unknown error'}`, 'error');
      }
    } catch (e) {
      if (!isAbortError(e) && showToast) showToast(e.message, 'error');
    } finally {
      setPlayLoading(false);
      abortRef.current = null;
    }
  };

  // Audio Speech (TTS)
  const handleGenerateTTS = async () => {
    const credentials = resolveCallCredentials();
    if (!credentials) return;
    setPlayLoading(true);
    setPlayStopped(false);
    setTtsAudioUrl(null);
    const controller = new AbortController();
    abortRef.current = controller;
    const start = Date.now();
    try {
      const request = buildTtsRequest();
      const res = await fetch(request.path, {
        method: request.method,
        headers: buildHeaders(credentials.key),
        body: JSON.stringify(request.body),
        signal: controller.signal
      });
      setPlayDurationMs(Date.now() - start);
      if (res.ok) {
        const blob = await res.blob();
        const url = URL.createObjectURL(blob);
        // Release the previous audio object URL to avoid leaking memory on
        // every synthesis, and remember the current one for unmount cleanup.
        setTtsAudioUrl((prev) => {
          if (prev) URL.revokeObjectURL(prev);
          return url;
        });
      } else {
        const err = await res.json().catch(() => ({}));
        if (showToast) showToast(isZh ? `TTS 失败: ${err.error?.message || '未知错误'}` : `TTS failed: ${err.error?.message || 'Unknown error'}`, 'error');
      }
    } catch (e) {
      if (!isAbortError(e) && showToast) showToast(e.message, 'error');
    } finally {
      setPlayLoading(false);
      abortRef.current = null;
    }
  };

  // Audio Transcription (STT) — multipart upload
  const handleTranscribe = async () => {
    if (!sttFile) {
      if (showToast) showToast(isZh ? '请先选择音频文件' : 'Please choose an audio file first', 'warning');
      return;
    }
    const credentials = resolveCallCredentials();
    if (!credentials) return;
    setPlayLoading(true);
    setPlayStopped(false);
    setSttResult(null);
    const controller = new AbortController();
    abortRef.current = controller;
    const start = Date.now();
    try {
      const form = new FormData();
      form.append('file', sttFile);
      form.append('model', sttModel);
      if (sttLanguage.trim()) form.append('language', sttLanguage.trim());
      const res = await fetch('/v1/audio/transcriptions', {
        method: 'POST',
        // No Content-Type header: the browser must set the multipart boundary.
        headers: credentials.key ? { Authorization: `Bearer ${credentials.key}` } : {},
        body: form,
        signal: controller.signal
      });
      const data = await res.json().catch(() => ({}));
      setPlayDurationMs(Date.now() - start);
      if (res.ok) {
        setSttResult(data.text || '');
      } else if (showToast) {
        showToast(isZh ? `转写失败: ${data.error?.message || res.statusText}` : `Transcription failed: ${data.error?.message || res.statusText}`, 'error');
      }
    } catch (e) {
      if (!isAbortError(e) && showToast) showToast(e.message, 'error');
    } finally {
      setPlayLoading(false);
      abortRef.current = null;
    }
  };

  // Video generation. The gateway returns either a finished asset or an async
  // task id that must be polled through /v1/videos/tasks/{id}.
  const handleGenerateVideo = async () => {
    const credentials = resolveCallCredentials();
    if (!credentials) return;
    setPlayLoading(true);
    setPlayStopped(false);
    setVideoResult(null);
    setVideoTaskId('');
    const controller = new AbortController();
    abortRef.current = controller;
    const start = Date.now();
    try {
      const request = buildVideoRequest();
      const res = await fetch(request.path, {
        method: request.method,
        headers: buildHeaders(credentials.key),
        body: JSON.stringify(request.body),
        signal: controller.signal
      });
      const data = await res.json().catch(() => ({}));
      setPlayDurationMs(Date.now() - start);
      if (!res.ok) {
        if (showToast) showToast(isZh ? `视频生成失败: ${data.error?.message || res.statusText}` : `Video generation failed: ${data.error?.message || res.statusText}`, 'error');
        return;
      }
      const taskId = data.id || data.task_id || data.data?.id || data.data?.task_id;
      if (taskId) {
        setVideoTaskId(taskId);
        await pollVideoTask(taskId, request.body.model, controller.signal, credentials.key);
      } else {
        setVideoResult(data);
      }
    } catch (e) {
      if (!isAbortError(e) && showToast) showToast(e.message, 'error');
    } finally {
      setPlayLoading(false);
      abortRef.current = null;
    }
  };

  const pollVideoTask = async (taskId, taskModel, signal, callKey) => {
    for (let attempt = 0; attempt < 40; attempt += 1) {
      await new Promise((resolve, reject) => {
        const abort = () => { clearTimeout(timer); reject(new DOMException('Stopped', 'AbortError')); };
        const timer = setTimeout(() => { signal.removeEventListener('abort', abort); resolve(); }, 3000);
        if (signal.aborted) { abort(); return; }
        signal.addEventListener('abort', abort, { once: true });
      });
      signal.throwIfAborted();
      const res = await fetch(`/v1/videos/tasks/${encodeURIComponent(taskId)}?model=${encodeURIComponent(taskModel)}`, {
        headers: buildHeaders(callKey), signal,
      });
      const data = await res.json();
      signal.throwIfAborted();
      if (!res.ok) throw new Error(data.error?.message || `Video task query failed (HTTP ${res.status})`);
      const status = String(data.status || data.data?.status || '').toLowerCase();
      setVideoResult(data.data || data);
      if (['succeeded', 'completed', 'success'].includes(status)) return;
      if (['failed', 'error', 'cancelled', 'canceled'].includes(status)) throw new Error(data.error?.message || (isZh ? '视频任务失败或已取消' : 'Video task failed or was cancelled'));
    }
    showToast?.(isZh ? '视频任务仍在处理中，请稍后查询' : 'Video task still running; check again later', 'warning');
  };

  // Embeddings
  const handleEmbed = async () => {
    const credentials = resolveCallCredentials();
    if (!credentials) return;
    setPlayLoading(true);
    setPlayStopped(false);
    setEmbedResult(null);
    const controller = new AbortController();
    abortRef.current = controller;
    const start = Date.now();
    try {
      const request = buildEmbeddingRequest();
      const res = await fetch(request.path, {
        method: request.method,
        headers: buildHeaders(credentials.key),
        body: JSON.stringify(request.body),
        signal: controller.signal
      });
      const data = await res.json().catch(() => ({}));
      setPlayDurationMs(Date.now() - start);
      if (res.ok) setEmbedResult(data);
      else if (showToast) showToast(isZh ? `向量化失败: ${data.error?.message || res.statusText}` : `Embedding failed: ${data.error?.message || res.statusText}`, 'error');
    } catch (e) {
      if (!isAbortError(e) && showToast) showToast(e.message, 'error');
    } finally {
      setPlayLoading(false);
      abortRef.current = null;
    }
  };

  // Rerank
  const handleRerank = async () => {
    const credentials = resolveCallCredentials();
    if (!credentials) return;
    setPlayLoading(true);
    setPlayStopped(false);
    setRerankResult(null);
    const controller = new AbortController();
    abortRef.current = controller;
    const start = Date.now();
    try {
      const request = buildRerankRequest();
      const res = await fetch(request.path, {
        method: request.method,
        headers: buildHeaders(credentials.key),
        body: JSON.stringify(request.body),
        signal: controller.signal
      });
      const data = await res.json().catch(() => ({}));
      setPlayDurationMs(Date.now() - start);
      if (res.ok) setRerankResult(data);
      else if (showToast) showToast(isZh ? `重排失败: ${data.error?.message || res.statusText}` : `Rerank failed: ${data.error?.message || res.statusText}`, 'error');
    } catch (e) {
      if (!isAbortError(e) && showToast) showToast(e.message, 'error');
    } finally {
      setPlayLoading(false);
      abortRef.current = null;
    }
  };

  // Non-empty when the current modality cannot be reproduced as a cURL; used to
  // disable the copy button and explain why in its tooltip.
  const curlUnsupportedReason = getCurlUnsupportedReason();

  return (
    <div className="space-y-4 animate-in fade-in duration-200">
      {/* 1. Modality Selector Bar */}
      <div className="bg-white dark:bg-[#191B1F] border border-[#E4E6E9] dark:border-[#2C3038] rounded-2xl p-1.5 shadow-sm flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap items-center gap-1.5">
          {[
            { id: 'chat', label: isZh ? 'Chat 对话' : 'Chat', icon: MessageSquare, activeColor: 'bg-[#355CFF] text-white' },
            { id: 'images', label: isZh ? 'Images 绘图' : 'Images', icon: ImageIcon, activeColor: 'bg-[#355CFF] text-white' },
            { id: 'audio_speech', label: isZh ? 'TTS 语音合成' : 'TTS Speech', icon: Volume2, activeColor: 'bg-[#355CFF] text-white' },
            { id: 'audio_transcription', label: isZh ? 'STT 语音转写' : 'STT Audio', icon: Mic, activeColor: 'bg-[#355CFF] text-white' },
            { id: 'videos', label: isZh ? 'Video 视频生成' : 'Videos', icon: Video, activeColor: 'bg-[#355CFF] text-white' },
            { id: 'embeddings', label: isZh ? 'Embedding 向量' : 'Embeddings', icon: Cpu, activeColor: 'bg-[#355CFF] text-white' },
            { id: 'rerank', label: isZh ? 'Rerank 重排' : 'Rerank', icon: Sliders, activeColor: 'bg-[#355CFF] text-white' },
          ].map(tab => {
            const Icon = tab.icon;
            return (
              <button
                key={tab.id}
                onClick={() => {
                  setPlayModality(tab.id);
                  setPlayStopped(false);
                }}
                className={`flex items-center space-x-1.5 px-3 py-1.5 rounded-lg text-xs font-medium transition cursor-pointer ${
                  playModality === tab.id
                    ? `${tab.activeColor} shadow-sm font-semibold`
                    : 'text-[#656B75] dark:text-[#A1A7B0] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] hover:text-[#202124] dark:hover:text-[#F1F2F4]'
                }`}
              >
                <Icon className="w-3.5 h-3.5" />
                <span>{tab.label}</span>
              </button>
            );
          })}
        </div>

        {/* Arena Mode Switch (Only for Chat) */}
        {playModality === 'chat' && (
          <div className="flex items-center space-x-1 bg-[#F0F1F2] dark:bg-[#22252B] p-1 rounded-lg">
            <button
              onClick={() => setPlayMode('single')}
              className={`px-2.5 py-1 rounded-md text-xs font-medium transition cursor-pointer ${
                playMode === 'single'
                  ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-sm font-semibold'
                  : 'text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'
              }`}
            >
              {isZh ? '单模型' : 'Single Model'}
            </button>
            <button
              onClick={() => setPlayMode('arena')}
              className={`px-2.5 py-1 rounded-md text-xs font-medium flex items-center space-x-1 transition cursor-pointer ${
                playMode === 'arena'
                  ? 'bg-[#355CFF] text-white shadow-sm font-semibold'
                  : 'text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'
              }`}
            >
              <Split className="w-3 h-3" />
              <span>{isZh ? '双模型对决 (Arena)' : 'Model Arena'}</span>
            </button>
          </div>
        )}
      </div>

      {/* 2. Main Playground Grid Layout */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Left Column: Parameter Controls */}
        <div className="bg-white dark:bg-[#191B1F] border border-[#E4E6E9] dark:border-[#2C3038] rounded-2xl p-6 space-y-4 shadow-sm h-fit">
          <div className="flex items-center justify-between border-b border-[#E4E6E9] dark:border-[#2C3038] pb-3">
            <h3 className="font-bold text-[#202124] dark:text-[#F1F2F4] text-sm flex items-center space-x-2">
              <Sparkles className="w-4 h-4 text-[#355CFF] dark:text-[#91A7FF]" />
              <span>{isZh ? '参数配置' : 'Hyperparameters'}</span>
            </h3>
          </div>

          {/* Key context + session credential */}
          <div className="space-y-2">
            <label className="text-xs font-semibold text-slate-700 dark:text-slate-300 flex items-center justify-between mb-1">
              <span className="flex items-center space-x-1">
                <Key className="w-3.5 h-3.5 text-indigo-500" />
                <span>{isZh ? 'API 访问密钥' : 'API Key'}</span>
              </span>
              {selectedKeyObj?.key_id && (
                <span className="ui-meta font-mono normal-case">{selectedKeyObj.key_id}</span>
              )}
            </label>

            {/* The list is only entitlement context; it carries no credential. */}
            <select
              value={selectedKeyId}
              onChange={(e) => setSelectedKeyId(e.target.value)}
              className="ui-field font-mono"
              aria-label={isZh ? '密钥上下文（租户与模型范围）' : 'Key context (tenant and model scope)'}
            >
              {contextKeys.map((k) => (
                <option key={String(k.id)} value={String(k.id)}>
                  {k.tenant_id ? `${k.tenant_id} - ` : ''}{keyDisplayRef(k)}
                </option>
              ))}
              <option value={UNAUTHENTICATED_KEY}>{isZh ? '免密直通 (不带 API Key)' : 'Public / Unauthenticated'}</option>
            </select>

            {selectedKeyId === UNAUTHENTICATED_KEY ? (
              <p className="ui-meta">
                {isZh ? '将以无凭证方式调用（仅适用于开启了公开访问的网关）。' : 'Calls will be sent without a credential (only valid on a gateway with public access enabled).'}
              </p>
            ) : (
              <>
                <input
                  type="password"
                  autoComplete="off"
                  value={sessionApiKey}
                  onChange={(e) => setSessionApiKey(e.target.value)}
                  placeholder={isZh ? '粘贴本次会话的 API Key 明文' : 'Paste the plaintext API key for this session'}
                  aria-label={isZh ? '会话 API Key 明文' : 'Session API key'}
                  className="ui-field font-mono"
                />
                <p className="ui-meta">
                  {isZh
                    ? '粘贴完整明文（sk-airoute-…）。列表里的 ak_… 是密钥 ID，只用于识别，不能用于调用；明文仅在创建密钥时显示一次，遗失请新建一把。本页输入仅保存在内存中，不写入浏览器存储。'
                    : 'Paste the full plaintext (sk-airoute-…). An ak_… value in the list is a key ID: it identifies the key but cannot be used to call it. The plaintext is shown once at creation — create a new key if it was lost. This page keeps it in memory only and never writes it to browser storage.'}
                </p>
              </>
            )}
          </div>

          {/* Chat Modality Controls */}
          {playModality === 'chat' && (
            <>
              {/* Primary Model */}
              <div>
                <label className="block text-xs font-semibold text-slate-600 dark:text-slate-400 mb-1">
                  {playMode === 'arena' ? (isZh ? '对决模型 A (Primary Model)' : 'Contender Model A') : (isZh ? '目标模型' : 'Target Model')}
                </label>
                <select
                  value={playModel}
                  onChange={(e) => setPlayModel(e.target.value)}
                  className="ui-field font-mono"
                >
                  {chatModels.map(m => (
                    <option key={m} value={m}>{m}</option>
                  ))}
                </select>
              </div>

              {/* Arena Model B */}
              {playMode === 'arena' && (
                <div>
                  <label className="block text-xs font-semibold text-purple-600 dark:text-purple-400 mb-1">
                    {isZh ? '对决模型 B (Arena Contender)' : 'Contender Model B'}
                  </label>
                  <select
                    value={arenaModelB}
                    onChange={(e) => setArenaModelB(e.target.value)}
                    className="ui-field font-mono bg-purple-50/50 dark:bg-purple-950/40 border-purple-200 dark:border-purple-800 focus:border-purple-500"
                  >
                    {chatModels.map(m => (
                      <option key={m} value={m}>{m}</option>
                    ))}
                  </select>
                </div>
              )}

              {/* System Prompt Collapsible */}
              <div className="border border-slate-100 dark:border-slate-800 rounded-xl p-2.5">
                <div className="flex items-center justify-between">
                  <button
                    type="button"
                    onClick={() => setShowSystemPrompt(!showSystemPrompt)}
                    className="flex-1 flex items-center justify-between text-xs font-semibold text-slate-700 dark:text-slate-300 cursor-pointer mr-2"
                  >
                    <span>{isZh ? '系统人设 (System Prompt)' : 'System Prompt'}</span>
                    {showSystemPrompt ? <ChevronUp className="w-3.5 h-3.5" /> : <ChevronDown className="w-3.5 h-3.5" />}
                  </button>
                  <button
                    type="button"
                    onClick={handleOptimizeSystemPrompt}
                    className="text-[10px] font-bold text-purple-600 dark:text-purple-400 hover:text-purple-700 flex items-center space-x-1 px-2 py-0.5 rounded-lg bg-purple-50 dark:bg-purple-950/50 border border-purple-200 dark:border-purple-800 transition cursor-pointer"
                    title={isZh ? '使用 AI 将系统提示词升格为结构化工程模板' : 'AI Prompt Engineering Enhancement'}
                  >
                    <Sparkles className="w-3 h-3 text-amber-500" />
                    <span>{isZh ? 'AI 优化' : 'AI Enhance'}</span>
                  </button>
                </div>
                {showSystemPrompt && (
                  <textarea
                    rows={4}
                    value={systemPrompt}
                    onChange={(e) => setSystemPrompt(e.target.value)}
                    className="ui-field font-mono mt-2 leading-relaxed resize-y"
                  />
                )}
              </div>

              {/* Hyperparameters: Temperature & MaxTokens */}
              <div className="space-y-3 pt-2 border-t border-[#E4E6E9] dark:border-[#2C3038]">
                <div>
                  <div className="flex justify-between text-xs font-semibold text-[#656B75] dark:text-[#A1A7B0] mb-1">
                    <span>{isZh ? '温度 (Temperature)' : 'Temperature'}</span>
                    <span className="font-mono text-[#355CFF] dark:text-[#91A7FF]">{temperature}</span>
                  </div>
                  <input
                    type="range"
                    min="0"
                    max="2"
                    step="0.1"
                    value={temperature}
                    onChange={(e) => setTemperature(parseFloat(e.target.value))}
                    className="w-full accent-[#355CFF] cursor-pointer"
                  />
                </div>

                <div>
                  <div className="flex items-center justify-between text-xs font-semibold text-[#656B75] dark:text-[#A1A7B0] mb-1.5">
                    <span>{isZh ? '最大生成 Token (Max Tokens)' : 'Max Tokens'}</span>
                    <div className="flex items-center space-x-1.5">
                      <input
                        type="number"
                        min="1"
                        max="1048576"
                        step="256"
                        placeholder={isZh ? '自动' : 'Auto'}
                        value={maxTokens}
                        onChange={(e) => {
                          const val = e.target.value;
                          setMaxTokens(val === '' ? '' : Math.max(1, parseInt(val) || ''));
                        }}
                        className="ui-field ui-field-sm w-20 text-right font-mono font-bold text-[#355CFF] dark:text-[#91A7FF]"
                        title={isZh ? '输入任意正整数，留空表示不限制（自动按模型上限）' : 'Enter arbitrary number or leave empty for auto'}
                      />
                      <span className="ui-meta font-mono">
                        {maxTokens ? '' : (isZh ? '(自动)' : '(Auto)')}
                      </span>
                    </div>
                  </div>

                  {/* Preset Quick Selection Buttons */}
                  <div className="flex flex-wrap gap-1 mb-2">
                    {[
                      { label: isZh ? '自动' : 'Auto', val: '' },
                      { label: '2K', val: 2048 },
                      { label: '4K', val: 4096 },
                      { label: '8K', val: 8192 },
                      { label: '16K', val: 16384 },
                      { label: '32K', val: 32768 },
                      { label: '64K', val: 65536 },
                      { label: '128K', val: 131072 }
                    ].map((p, i) => {
                      const isActive = (p.val === '' && (maxTokens === '' || maxTokens === 0)) || (p.val !== '' && Number(maxTokens) === p.val);
                      return (
                        <button
                          key={i}
                          type="button"
                          onClick={() => setMaxTokens(p.val)}
                          className={`px-2 py-0.5 text-[10px] font-mono rounded-md border transition cursor-pointer ${
                            isActive
                              ? 'bg-[#355CFF] text-white border-[#355CFF] font-bold shadow-xs'
                              : 'bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0] border-[#E4E6E9] dark:border-[#2C3038] hover:bg-[#E4E6E9] dark:hover:bg-[#2C3038]'
                          }`}
                        >
                          {p.label}
                        </button>
                      );
                    })}
                  </div>

                  {/* Dynamic slider up to 131072 */}
                  <input
                    type="range"
                    min="256"
                    max="131072"
                    step="256"
                    value={maxTokens ? Math.min(131072, Math.max(256, Number(maxTokens))) : 2048}
                    onChange={(e) => setMaxTokens(parseInt(e.target.value))}
                    className="w-full accent-[#355CFF] cursor-pointer"
                  />
                  <div className="flex justify-between ui-meta font-mono mt-0.5">
                    <span>256</span>
                    <span>16K</span>
                    <span>64K</span>
                    <span>128K</span>
                  </div>
                </div>

                <div className="flex items-center space-x-2 pt-1">
                  <input
                    type="checkbox"
                    id="streamCheck"
                    checked={playStream}
                    onChange={(e) => setPlayStream(e.target.checked)}
                    className="rounded-sm border-[#E4E6E9] dark:border-[#2C3038] text-[#355CFF] focus:ring-0 cursor-pointer"
                  />
                  <label htmlFor="streamCheck" className="text-xs font-semibold text-[#202124] dark:text-[#F1F2F4] cursor-pointer">
                    {isZh ? '开启 SSE 流式输出 (Streaming)' : 'Enable SSE Streaming'}
                  </label>
                </div>
              </div>
            </>
          )}

          {/* Image Modality Controls */}
          {playModality === 'images' && (
            <div className="space-y-3">
              <div>
                <label className="block text-xs font-semibold text-slate-600 dark:text-slate-400 mb-1">{isZh ? '生图模型' : 'Image Model'}</label>
                <select
                  value={imgModel}
                  onChange={(e) => setImgModel(e.target.value)}
                  className="ui-field font-mono"
                >
                  {imgModels.map(m => (
                    <option key={m} value={m}>{m}</option>
                  ))}
                </select>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-600 dark:text-slate-400 mb-1">{isZh ? '画面尺寸 (Size)' : 'Image Size'}</label>
                <select
                  value={imgSize}
                  onChange={(e) => setImgSize(e.target.value)}
                  className="ui-field"
                >
                  <option value="1024x1024">{isZh ? '1024x1024 (方形 1:1)' : '1024x1024 (Square 1:1)'}</option>
                  <option value="1792x1024">{isZh ? '1792x1024 (横屏 16:9)' : '1792x1024 (Landscape 16:9)'}</option>
                  <option value="1024x1792">{isZh ? '1024x1792 (竖屏 9:16)' : '1024x1792 (Portrait 9:16)'}</option>
                </select>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-600 dark:text-slate-400 mb-1">{isZh ? '画面提示词 (Prompt)' : 'Prompt'}</label>
                <textarea
                  rows={3}
                  value={imgPrompt}
                  onChange={(e) => setImgPrompt(e.target.value)}
                  className="ui-field resize-y"
                />
              </div>

              <button
                onClick={handleGenerateImage}
                disabled={playLoading}
                className="ui-btn w-full bg-pink-600 text-white hover:bg-pink-700 shadow-sm"
              >
                {playLoading ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Sparkles className="w-3.5 h-3.5" />}
                <span>{playLoading ? (isZh ? '渲染生成中...' : 'Rendering...') : (isZh ? '生成图像' : 'Generate Image')}</span>
              </button>
            </div>
          )}

          {/* TTS Modality Controls */}
          {playModality === 'audio_speech' && (
            <div className="space-y-3">
              <div>
                <label className="block text-xs font-semibold text-slate-600 dark:text-slate-400 mb-1">{isZh ? '语音合成模型' : 'TTS Model'}</label>
                <select
                  value={ttsModel}
                  onChange={(e) => setTtsModel(e.target.value)}
                  className="ui-field font-mono"
                >
                  {ttsModels.map(m => (
                    <option key={m} value={m}>{m}</option>
                  ))}
                </select>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-600 dark:text-slate-400 mb-1">{isZh ? '音色 (Voice)' : 'Voice'}</label>
                <select
                  value={ttsVoice}
                  onChange={(e) => setTtsVoice(e.target.value)}
                  className="ui-field"
                >
                  <option value="alloy">alloy ({isZh ? '自然中性' : 'Neutral'})</option>
                  <option value="echo">echo ({isZh ? '沉稳男声' : 'Deep Male'})</option>
                  <option value="fable">fable ({isZh ? '磁性叙事' : 'Narrative'})</option>
                  <option value="nova">nova ({isZh ? '明朗女声' : 'Clear Female'})</option>
                  <option value="shimmer">shimmer ({isZh ? '温柔女声' : 'Soft Female'})</option>
                </select>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-600 dark:text-slate-400 mb-1">{isZh ? '文本内容' : 'Text Input'}</label>
                <textarea
                  rows={3}
                  value={ttsInput}
                  onChange={(e) => setTtsInput(e.target.value)}
                  className="ui-field resize-y"
                />
              </div>

              <button
                onClick={handleGenerateTTS}
                disabled={playLoading}
                className="ui-btn w-full bg-cyan-600 text-white hover:bg-cyan-700 shadow-sm"
              >
                {playLoading ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Volume2 className="w-3.5 h-3.5" />}
                <span>{playLoading ? (isZh ? '合成中...' : 'Synthesizing...') : (isZh ? '生成语音' : 'Synthesize Speech')}</span>
              </button>
            </div>
          )}

          {/* STT (audio transcription) Controls */}
          {playModality === 'audio_transcription' && (
            <div className="space-y-3">
              <div>
                <label className="block text-xs font-semibold text-slate-600 dark:text-slate-400 mb-1">{isZh ? '语音识别模型' : 'STT Model'}</label>
                <select value={sttModel} onChange={(e) => setSttModel(e.target.value)} className="ui-field font-mono">
                  {sttModels.length === 0 && <option value="">{isZh ? '未配置 STT 模型' : 'No STT model configured'}</option>}
                  {sttModels.map(m => <option key={m} value={m}>{m}</option>)}
                </select>
              </div>
              <div>
                <label className="block text-xs font-semibold text-slate-600 dark:text-slate-400 mb-1">{isZh ? '音频文件 (multipart)' : 'Audio File (multipart)'}</label>
                <input
                  type="file"
                  accept="audio/*"
                  onChange={(e) => setSttFile(e.target.files?.[0] || null)}
                  className="ui-field text-xs"
                />
                {sttFile && <span className="ui-meta mt-1 block font-mono">{sttFile.name} · {Math.round(sttFile.size / 1024)} KB</span>}
              </div>
              <div>
                <label className="block text-xs font-semibold text-slate-600 dark:text-slate-400 mb-1">{isZh ? '语言 (可选, 如 zh/en)' : 'Language (optional, e.g. zh/en)'}</label>
                <input value={sttLanguage} onChange={(e) => setSttLanguage(e.target.value)} className="ui-field font-mono" placeholder="zh" />
              </div>
              <button onClick={handleTranscribe} disabled={playLoading || !sttFile} className="ui-btn w-full bg-teal-600 text-white hover:bg-teal-700 shadow-sm disabled:opacity-50">
                {playLoading ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Mic className="w-3.5 h-3.5" />}
                <span>{playLoading ? (isZh ? '转写中...' : 'Transcribing...') : (isZh ? '开始转写' : 'Transcribe')}</span>
              </button>
            </div>
          )}

          {/* Video generation Controls */}
          {playModality === 'videos' && (
            <div className="space-y-3">
              <div>
                <label className="block text-xs font-semibold text-slate-600 dark:text-slate-400 mb-1">{isZh ? '视频生成模型' : 'Video Model'}</label>
                <select value={videoModel} onChange={(e) => setVideoModel(e.target.value)} className="ui-field font-mono">
                  {videoModels.length === 0 && <option value="">{isZh ? '未配置视频模型' : 'No video model configured'}</option>}
                  {videoModels.map(m => <option key={m} value={m}>{m}</option>)}
                </select>
              </div>
              <div>
                <label className="block text-xs font-semibold text-slate-600 dark:text-slate-400 mb-1">{isZh ? '视频提示词 (Prompt)' : 'Prompt'}</label>
                <textarea rows={4} value={videoPrompt} onChange={(e) => setVideoPrompt(e.target.value)} className="ui-field resize-y" />
              </div>
              <button onClick={handleGenerateVideo} disabled={playLoading} className="ui-btn w-full bg-purple-600 text-white hover:bg-purple-700 shadow-sm">
                {playLoading ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Video className="w-3.5 h-3.5" />}
                <span>{playLoading ? (isZh ? '生成中(异步轮询)...' : 'Generating (async polling)...') : (isZh ? '生成视频' : 'Generate Video')}</span>
              </button>
              {videoTaskId && (
                <div className="ui-meta font-mono break-all">
                  {isZh ? '任务 ID: ' : 'Task ID: '}{videoTaskId}
                </div>
              )}
            </div>
          )}

          {/* Embeddings Controls */}
          {playModality === 'embeddings' && (
            <div className="space-y-3">
              <div>
                <label className="block text-xs font-semibold text-slate-600 dark:text-slate-400 mb-1">{isZh ? '向量模型' : 'Embedding Model'}</label>
                <select value={embedModel} onChange={(e) => setEmbedModel(e.target.value)} className="ui-field font-mono">
                  {embedModels.length === 0 && <option value="">{isZh ? '未配置向量模型' : 'No embedding model configured'}</option>}
                  {embedModels.map(m => <option key={m} value={m}>{m}</option>)}
                </select>
              </div>
              <div>
                <label className="block text-xs font-semibold text-slate-600 dark:text-slate-400 mb-1">
                  {isZh ? '输入文本（每行一条，批量向量化）' : 'Input (one line per item, batch embedding)'}
                </label>
                <textarea rows={5} value={embedInput} onChange={(e) => setEmbedInput(e.target.value)} className="ui-field resize-y font-mono text-xs" />
              </div>
              <button onClick={handleEmbed} disabled={playLoading} className="ui-btn w-full bg-emerald-600 text-white hover:bg-emerald-700 shadow-sm">
                {playLoading ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Cpu className="w-3.5 h-3.5" />}
                <span>{playLoading ? (isZh ? '向量化中...' : 'Embedding...') : (isZh ? '生成向量' : 'Generate Embeddings')}</span>
              </button>
            </div>
          )}

          {/* Rerank Controls */}
          {playModality === 'rerank' && (
            <div className="space-y-3">
              <div>
                <label className="block text-xs font-semibold text-slate-600 dark:text-slate-400 mb-1">{isZh ? '重排模型' : 'Rerank Model'}</label>
                <select value={rerankModel} onChange={(e) => setRerankModel(e.target.value)} className="ui-field font-mono">
                  {rerankModels.length === 0 && <option value="">{isZh ? '未配置重排模型' : 'No rerank model configured'}</option>}
                  {rerankModels.map(m => <option key={m} value={m}>{m}</option>)}
                </select>
              </div>
              <div>
                <label className="block text-xs font-semibold text-slate-600 dark:text-slate-400 mb-1">{isZh ? '查询语句 (Query)' : 'Query'}</label>
                <input value={rerankQuery} onChange={(e) => setRerankQuery(e.target.value)} className="ui-field" />
              </div>
              <div>
                <label className="block text-xs font-semibold text-slate-600 dark:text-slate-400 mb-1">
                  {isZh ? '候选文档（每行一条）' : 'Documents (one per line)'}
                </label>
                <textarea rows={5} value={rerankDocs} onChange={(e) => setRerankDocs(e.target.value)} className="ui-field resize-y text-xs" />
              </div>
              <button onClick={handleRerank} disabled={playLoading} className="ui-btn w-full bg-amber-600 text-white hover:bg-amber-700 shadow-sm">
                {playLoading ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Sliders className="w-3.5 h-3.5" />}
                <span>{playLoading ? (isZh ? '重排中...' : 'Reranking...') : (isZh ? '执行重排' : 'Run Rerank')}</span>
              </button>
            </div>
          )}

          {/* Telemetry Summary Footer */}
          <div className="ui-meta pt-3 border-t border-slate-100 dark:border-slate-800 space-y-1.5">
            <div className="flex justify-between">
              <span>{isZh ? '首字时延 (TTFT):' : 'TTFT Latency:'}</span>
              <span className="font-mono font-bold text-amber-600 dark:text-amber-400">
                {playTTFTMs ? `${playTTFTMs} ms` : '-'}
              </span>
            </div>
            <div className="flex justify-between">
              <span>{isZh ? '总耗时:' : 'Duration:'}</span>
              <span className="font-mono font-bold text-indigo-600 dark:text-indigo-400">
                {playDurationMs ? `${playDurationMs} ms` : '-'}
              </span>
            </div>
            {playTokensPerSec && (
              <div className="flex justify-between">
                <span>{isZh ? '吞吐速度:' : 'Speed:'}</span>
                <span className="font-mono font-bold text-emerald-600 dark:text-emerald-400">
                  ⚡ {playTokensPerSec} tokens/s
                </span>
              </div>
            )}
          </div>
        </div>

        {/* Right Columns: Interactive Chat View / Arena View */}
        <div className="lg:col-span-2 bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-2xl p-6 flex flex-col h-[min(700px,calc(100vh-16rem))] min-h-[360px] shadow-sm">
          {/* Header Action Bar */}
          <div className="flex flex-wrap items-center justify-between gap-2 pb-3 border-b border-slate-100 dark:border-slate-800 mb-3 text-xs">
            <div className="flex items-center space-x-2">
              <Terminal className="w-4 h-4 text-indigo-600 dark:text-indigo-400" />
              <span className="font-bold text-slate-800 dark:text-slate-200">
                {playModality === 'chat'
                  ? (playMode === 'arena' ? (isZh ? '双模型横向比对评测竞技场 (Arena)' : 'Model Arena Side-by-Side Comparison') : (isZh ? '连续多轮对话调试' : 'Conversational Testing Stream'))
                  : (isZh ? '多模态结果呈现' : 'Multimodal Output Stream')}
              </span>
            </div>

            <div className="flex items-center space-x-2">
              {/* Stop: only while a request is in flight. */}
              {playLoading && (
                <button
                  type="button"
                  onClick={handleStopGeneration}
                  className="ui-btn-danger ui-btn-sm"
                  title={isZh ? '中断当前生成，保留已接收的内容' : 'Abort the current generation and keep what was received'}
                >
                  <Square className="w-3 h-3 fill-current" />
                  <span>{isZh ? '停止生成' : 'Stop'}</span>
                </button>
              )}

              {/* Neutral state after a user-initiated stop (never an error). */}
              {playStopped && !playLoading && (
                <span className="ui-chip" title={isZh ? '生成已被手动停止，已保留部分输出' : 'Generation was stopped manually; partial output is kept'}>
                  <span className="inline-block w-1.5 h-1.5 rounded-full bg-slate-400" />
                  <span>{isZh ? '已停止生成' : 'Generation stopped'}</span>
                </span>
              )}

              {/* Title lives on the wrapper: disabled buttons swallow pointer events,
                  so a title on the button itself would never show on hover. */}
              <span
                className="inline-flex"
                title={curlUnsupportedReason || (isZh ? '复制与当前请求完全一致的 cURL 命令' : 'Copy a cURL command that matches the current request exactly')}
              >
                <button
                  type="button"
                  onClick={handleCopyCurl}
                  disabled={!!curlUnsupportedReason}
                  className="ui-btn-secondary ui-btn-sm"
                >
                  <Code className="w-3 h-3 text-indigo-500" />
                  <span>{isZh ? '复制 cURL' : 'Copy cURL'}</span>
                </button>
              </span>

              <button
                onClick={() => {
                  setMessages([]);
                  setPlayOutput('');
                  setPlayReasoningOutput('');
                  setArenaOutputB('');
                  setImgResult(null);
                  setTtsAudioUrl(null);
                  setPlayStopped(false);
                }}
                className="px-2 py-1 text-xs text-slate-500 dark:text-slate-400 hover:text-rose-600 dark:hover:text-rose-400 transition cursor-pointer"
              >
                {isZh ? '清空会话' : 'Clear Chat'}
              </button>
            </div>
          </div>

          {/* Chat Body */}
          {playModality === 'chat' && (
            <div className="flex-1 flex flex-col min-h-0">
              {playMode === 'single' ? (
                /* Single Model Chat View */
                <div className="flex-1 overflow-y-auto space-y-4 p-4 rounded-xl border border-slate-200/80 dark:border-slate-800/80 bg-slate-50/60 dark:bg-slate-900/40">
                  {messages.length === 0 && !playLoading && !playOutput && !playReasoningOutput && (
                    <div className="h-full min-h-[240px] flex flex-col items-center justify-center text-center py-16 text-slate-400 dark:text-slate-500">
                      <MessageSquare className="w-10 h-10 mb-3 opacity-30 stroke-1" />
                      <p className="text-xs font-medium">{isZh ? '输入提示词开始调试演练' : 'Enter a prompt to start testing'}</p>
                      <p className="text-[11px] opacity-75 mt-1">{isZh ? '支持流式输出、深度思维链解析与多模型竞技场对比' : 'Supports streaming responses, reasoning traces, and model arena'}</p>
                    </div>
                  )}

                  {/* Messages Bubble Stream */}
                  {messages.map((msg, idx) => (
                    <div
                      key={idx}
                      className={`flex flex-col ${msg.role === 'user' ? 'items-end' : 'items-start'}`}
                    >
                      <div className="ui-meta font-mono mb-1">
                        {msg.role === 'user' ? (isZh ? '调用方 (User)' : 'User') : `${playModel} (Assistant)`}
                      </div>
                      <div
                        className={`max-w-[85%] rounded-2xl p-3.5 text-xs leading-relaxed ${
                          msg.role === 'user'
                            ? 'bg-indigo-600 text-white'
                            : 'bg-white dark:bg-slate-800 text-slate-800 dark:text-slate-100 border border-slate-200/80 dark:border-slate-700 shadow-sm'
                        }`}
                      >
                        {msg.reasoning && (
                          <div className="mb-2.5 p-2.5 bg-amber-50/70 dark:bg-amber-950/30 border border-amber-200/80 dark:border-amber-800/60 rounded-xl text-xs text-amber-950 dark:text-amber-200">
                            <div className="font-bold flex items-center space-x-1.5 text-amber-800 dark:text-amber-400 mb-1">
                              <Sparkles className="w-3 h-3" />
                              <span>{isZh ? '深度思维链' : 'Reasoning Process'}</span>
                            </div>
                            <div className="whitespace-pre-wrap text-[11px] font-mono leading-relaxed opacity-85">
                              {msg.reasoning}
                            </div>
                          </div>
                        )}
                        {msg.role === 'user' ? (
                          <div className="whitespace-pre-wrap">{msg.content}</div>
                        ) : (
                          <MarkdownRenderer content={msg.content} />
                        )}
                      </div>
                    </div>
                  ))}

                  {/* Live Streaming Assistant Message (Unified Bubble) */}
                  {(playLoading || playOutput || playReasoningOutput) && (
                    <div className="flex flex-col items-start w-full">
                      <div className="ui-meta font-mono mb-1 flex items-center space-x-1.5">
                        <span>{playModel} (Assistant)</span>
                        {playLoading && <span className="inline-block w-1.5 h-1.5 rounded-full bg-indigo-500 animate-pulse" />}
                      </div>
                      <div className="max-w-[85%] w-full rounded-2xl p-3.5 text-xs bg-white dark:bg-slate-800 text-slate-800 dark:text-slate-100 border border-slate-200/80 dark:border-slate-700 shadow-sm leading-relaxed">
                        {playReasoningOutput && (
                          <div className="mb-2.5 p-2.5 bg-amber-50/70 dark:bg-amber-950/30 border border-amber-200/80 dark:border-amber-800/60 rounded-xl text-xs text-amber-950 dark:text-amber-200">
                            <div className="font-bold flex items-center space-x-1.5 text-amber-800 dark:text-amber-400 mb-1">
                              <Sparkles className="w-3 h-3 text-amber-500 animate-pulse" />
                              <span>{isZh ? '深度思维链' : 'Reasoning Process'}</span>
                            </div>
                            <div className="whitespace-pre-wrap text-[11px] font-mono leading-relaxed opacity-85">
                              {playReasoningOutput}
                            </div>
                          </div>
                        )}
                        {playOutput ? (
                          <MarkdownRenderer content={playOutput} />
                        ) : (
                          playLoading && !playReasoningOutput && (
                            <div className="flex items-center space-x-2 ui-meta py-1 font-mono">
                              <span className="w-2 h-2 rounded-full bg-indigo-500 animate-ping" />
                              <span>{isZh ? '正在思考与组织响应...' : 'Thinking and generating response...'}</span>
                            </div>
                          )
                        )}
                      </div>
                    </div>
                  )}
                </div>
              ) : (
                /* Side-by-Side Arena Split View */
                <div className="flex-1 grid grid-cols-2 gap-3 min-h-0">
                  {/* Left Contender: Model A */}
                  <div className="flex flex-col rounded-xl border border-[#355CFF]/30 dark:border-[#355CFF]/40 bg-[#EEF2FF]/30 dark:bg-[#242D48]/20 p-3 overflow-y-auto space-y-3">
                    <div className="flex items-center justify-between pb-2 border-b border-[#355CFF]/20 dark:border-[#355CFF]/30 text-xs">
                      <span className="font-bold text-[#355CFF] dark:text-[#91A7FF] font-mono">
                        Model A: {playModel}
                      </span>
                      <div className="ui-meta font-mono space-x-2">
                        <span>TTFT: {playTTFTMs ? `${playTTFTMs}ms` : '-'}</span>
                        <span>{isZh ? '速度' : 'Speed'}: {playTokensPerSec ? `${playTokensPerSec}tps` : '-'}</span>
                      </div>
                    </div>
                    {playReasoningOutput && (
                      <div className="p-2.5 rounded-xl bg-amber-500/10 border border-amber-500/20 text-[11px] text-amber-900 dark:text-amber-200 font-mono whitespace-pre-wrap">
                        {playReasoningOutput}
                      </div>
                    )}
                    <div className="text-xs text-[#202124] dark:text-[#F1F2F4] leading-relaxed">
                      {playOutput ? (
                        <MarkdownRenderer content={playOutput} />
                      ) : (
                        <div className="text-center py-24 ui-meta">
                          {isZh ? '等待发送评测指令...' : 'Awaiting prompt evaluation...'}
                        </div>
                      )}
                    </div>
                  </div>

                  {/* Right Contender: Model B */}
                  <div className="flex flex-col rounded-xl border border-[#E4E6E9] dark:border-[#2C3038] bg-[#F0F1F2]/60 dark:bg-[#22252B]/40 p-3 overflow-y-auto space-y-3">
                    <div className="flex items-center justify-between pb-2 border-b border-[#E4E6E9] dark:border-[#2C3038] text-xs">
                      <span className="font-bold text-[#202124] dark:text-[#F1F2F4] font-mono">
                        Model B: {arenaModelB}
                      </span>
                      <div className="ui-meta font-mono space-x-2">
                        <span>TTFT: {arenaTTFTB ? `${arenaTTFTB}ms` : '-'}</span>
                        <span>{isZh ? '速度' : 'Speed'}: {arenaTokensPerSecB ? `${arenaTokensPerSecB}tps` : '-'}</span>
                        {/* The single-model view reports the total duration; the
                            arena collected it for B and never showed it. */}
                        <span>{isZh ? '总耗时' : 'Duration'}: {arenaDurationB ? `${arenaDurationB}ms` : '-'}</span>
                      </div>
                    </div>
                    {arenaReasoningB && (
                      <div className="p-2.5 rounded-xl bg-amber-50 dark:bg-amber-950/40 border border-amber-200 text-[11px] text-amber-900 dark:text-amber-200 font-mono whitespace-pre-wrap">
                        {arenaReasoningB}
                      </div>
                    )}
                    <div className="text-xs text-slate-800 dark:text-slate-100 leading-relaxed">
                      {arenaOutputB ? (
                        <MarkdownRenderer content={arenaOutputB} />
                      ) : (
                        <div className="text-center py-24 ui-meta">
                          {isZh ? '等待发送评测指令...' : 'Awaiting prompt evaluation...'}
                        </div>
                      )}
                    </div>
                  </div>
                </div>
              )}

              {/* Live FinOps Telemetry Bar */}
              <div className="ui-meta flex flex-wrap items-center justify-between gap-x-2 gap-y-1 px-1 pt-2">
                <div className="flex items-center space-x-2">
                  <span className="font-mono">
                    {isZh ? '预估输入 Tokens: ~' : 'Est. Prompt Tokens: ~'}
                    <span className="font-bold text-slate-700 dark:text-slate-300">
                      {Math.max(0, Math.round((chatInput.length + (showSystemPrompt ? systemPrompt.length : 0)) / 3.2))}
                    </span>
                  </span>
                  <span>·</span>
                  <span className="text-emerald-600 dark:text-emerald-400 font-medium flex items-center space-x-1">
                    <Zap className="w-3 h-3" />
                    <span>{isZh ? 'Prompt 缓存识别就绪' : 'Prompt Caching Ready'}</span>
                  </span>
                </div>
                {offPeak ? (
                  <div
                    className="flex items-center space-x-1 text-indigo-600 dark:text-indigo-400 font-medium"
                    title={isZh
                      ? `模型 [${activeChatModel}] 配置了闲时折扣；是否生效取决于当前时刻与优惠时段设置。`
                      : `Model [${activeChatModel}] has an off-peak discount; whether it applies now depends on the configured time windows.`}
                  >
                    <Clock className="w-3 h-3" />
                    <span>
                      {isZh
                        ? `闲时折扣已配置 (${offPeak.label} 折, 按时段生效)`
                        : `Off-peak discount configured (${offPeak.label}/10, time-windowed)`}
                    </span>
                  </div>
                ) : (
                  <div className="flex items-center space-x-1 text-slate-400 dark:text-slate-500 font-medium">
                    <Clock className="w-3 h-3" />
                    <span>{isZh ? '该模型未配置闲时折扣' : 'No off-peak discount configured'}</span>
                  </div>
                )}
              </div>

              {/* Chat Input Bar: auto-growing composer (Enter sends, Shift+Enter newline) */}
              <div className="mt-2 flex items-end space-x-2">
                <textarea
                  ref={chatInputRef}
                  rows={1}
                  value={chatInput}
                  onChange={(e) => setChatInput(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key !== 'Enter' || e.shiftKey) return;
                    // An IME candidate confirmation also reports Enter: never send
                    // while composing (Chinese / Japanese input).
                    if (e.nativeEvent.isComposing || e.keyCode === 229) return;
                    e.preventDefault();
                    if (!playLoading) handleRunChat();
                  }}
                  placeholder={playMode === 'arena'
                    ? (isZh ? '输入统一 Prompt，同时对两款模型展开并发比对 (Enter 发送 / Shift+Enter 换行)...' : 'Prompt to benchmark both models at once (Enter to send / Shift+Enter for newline)...')
                    : (isZh ? '输入对话 Prompt，Enter 发送 / Shift+Enter 换行...' : 'Enter prompt — Enter to send, Shift+Enter for a newline...')}
                  className="ui-field flex-1 min-w-0 resize-none overflow-y-auto"
                />
                <button
                  type="button"
                  onClick={handleOptimizeChatInput}
                  disabled={!chatInput.trim()}
                  className="ui-btn px-2.5 py-2.5 shrink-0 border border-purple-200 dark:border-purple-800/80 bg-purple-50 dark:bg-purple-950/40 text-purple-700 dark:text-purple-300 hover:bg-purple-100 dark:hover:bg-purple-900/60"
                  title={isZh ? '使用 AI 将输入 Prompt 升格为专业技术规格' : 'AI Prompt Engineering Enhancement'}
                >
                  <Sparkles className="w-4 h-4 text-amber-500" />
                </button>
                <button
                  type="button"
                  onClick={handleRunChat}
                  disabled={playLoading}
                  className="ui-btn-primary px-5 shrink-0"
                >
                  {playLoading ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Send className="w-3.5 h-3.5" />}
                  <span>{playLoading ? (isZh ? '流式推导中...' : 'Generating...') : (playMode === 'arena' ? (isZh ? '并发对决测试' : 'Run Arena Test') : (isZh ? '发送请求' : 'Send'))}</span>
                </button>
              </div>
            </div>
          )}

          {/* Image Result */}
          {playModality === 'images' && (
            <div className="flex-1 flex flex-col items-center justify-center p-4">
              {imgResult ? (
                <div className="flex flex-col items-center space-y-3">
                  <div className="relative group max-h-[min(460px,50vh)] overflow-hidden rounded-2xl border border-slate-200 dark:border-slate-800 shadow-md">
                    <img
                      src={imgResult.url || `data:image/png;base64,${imgResult.b64_json}`}
                      alt="Generated"
                      className="max-h-[min(440px,46vh)] w-auto object-contain mx-auto"
                    />
                  </div>
                  <a
                    href={imgResult.url || `data:image/png;base64,${imgResult.b64_json}`}
                    download="airoute-image.png"
                    target="_blank"
                    rel="noreferrer"
                    className="ui-btn bg-pink-600 text-white hover:bg-pink-700 shadow-sm"
                  >
                    <Download className="w-3.5 h-3.5" />
                    <span>{isZh ? '下载高清原图' : 'Download High-Res'}</span>
                  </a>
                </div>
              ) : (
                <div className="text-center ui-meta space-y-2">
                  <ImageIcon className="w-12 h-12 stroke-1 mx-auto text-slate-400" />
                  <p>{isZh ? '点击左侧【生成图像】开始 AI 视觉渲染' : 'Click [Generate Image] on the left to start AI rendering'}</p>
                </div>
              )}
            </div>
          )}

          {/* TTS Audio Player Result */}
          {playModality === 'audio_speech' && (
            <div className="flex-1 flex flex-col items-center justify-center p-4 space-y-4">
              {ttsAudioUrl ? (
                <div className="w-full max-w-md p-6 bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-2xl flex flex-col items-center space-y-4 shadow-sm">
                  <div className="w-12 h-12 rounded-2xl bg-cyan-50 dark:bg-cyan-950/60 text-cyan-600 dark:text-cyan-400 flex items-center justify-center shadow-sm">
                    <Volume2 className="w-6 h-6" />
                  </div>
                  <span className="font-bold text-xs text-slate-800 dark:text-slate-200">
                    {isZh ? '语音合成渲染完成 (MP3 直通流)' : 'Audio Synthesis Completed (Direct MP3 Stream)'}
                  </span>
                  <audio controls src={ttsAudioUrl} className="w-full" autoPlay />
                </div>
              ) : (
                <div className="text-center ui-meta space-y-2">
                  <FileAudio className="w-12 h-12 stroke-1 mx-auto text-slate-400" />
                  <p>{isZh ? '点击左侧【生成语音】开始流式音频合成' : 'Click [Synthesize Speech] on the left to start streaming audio'}</p>
                </div>
              )}
            </div>
          )}

          {/* STT Result */}
          {playModality === 'audio_transcription' && (
            <div className="flex-1 flex flex-col p-4 overflow-y-auto">
              {sttResult !== null ? (
                <div className="w-full space-y-2">
                  <span className="ui-meta font-semibold">{isZh ? '转写文本' : 'Transcript'}</span>
                  <pre className="p-4 rounded-2xl bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-xs whitespace-pre-wrap break-words font-mono">{sttResult}</pre>
                </div>
              ) : (
                <div className="flex-1 flex flex-col items-center justify-center text-center ui-meta space-y-2">
                  <Mic className="w-12 h-12 stroke-1 text-slate-400" />
                  <p>{isZh ? '选择音频文件后点击【开始转写】' : 'Choose an audio file, then click Transcribe'}</p>
                </div>
              )}
            </div>
          )}

          {/* Video Result */}
          {playModality === 'videos' && (
            <div className="flex-1 flex flex-col items-center justify-center p-4 space-y-3 overflow-y-auto">
              {videoResult ? (
                <div className="w-full max-w-2xl space-y-2">
                  {(() => {
                    const v = videoResult.video || videoResult.data?.[0] || videoResult;
                    const url = v?.url || v?.video_url || (v?.b64_json ? `data:video/mp4;base64,${v.b64_json}` : '');
                    return url ? (
                      <video controls src={url} className="w-full rounded-2xl border border-slate-200 dark:border-slate-800 shadow-md" />
                    ) : (
                      <pre className="p-4 rounded-2xl bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-[11px] overflow-x-auto font-mono">{JSON.stringify(videoResult, null, 2)}</pre>
                    );
                  })()}
                </div>
              ) : (
                <div className="text-center ui-meta space-y-2">
                  <Video className="w-12 h-12 stroke-1 text-slate-400" />
                  <p>{isZh ? '视频为异步任务：提交后会自动轮询任务状态' : 'Video is asynchronous: the task is polled automatically after submit'}</p>
                </div>
              )}
            </div>
          )}

          {/* Embeddings Result */}
          {playModality === 'embeddings' && (
            <div className="flex-1 flex flex-col p-4 overflow-y-auto">
              {embedResult ? (
                <div className="w-full space-y-3">
                  <div className="flex flex-wrap gap-3 ui-meta font-mono">
                    <span>{isZh ? '向量条数: ' : 'Vectors: '}{(embedResult.data || []).length}</span>
                    <span>{isZh ? '维度: ' : 'Dimensions: '}{(embedResult.data?.[0]?.embedding || []).length}</span>
                    {embedResult.usage?.prompt_tokens != null && <span>{isZh ? '消耗 Token: ' : 'Tokens: '}{embedResult.usage.prompt_tokens}</span>}
                  </div>
                  {(embedResult.data || []).map((item, idx) => (
                    <div key={idx} className="rounded-xl border border-slate-200 dark:border-slate-800 p-3">
                      <span className="ui-meta font-semibold block mb-1">#{item.index ?? idx}</span>
                      <pre className="text-[10px] font-mono whitespace-pre-wrap break-all max-h-28 overflow-y-auto">
                        {JSON.stringify((item.embedding || []).slice(0, 24))}{(item.embedding || []).length > 24 ? ' …' : ''}
                      </pre>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="flex-1 flex flex-col items-center justify-center text-center ui-meta space-y-2">
                  <Cpu className="w-12 h-12 stroke-1 text-slate-400" />
                  <p>{isZh ? '输入文本后点击【生成向量】' : 'Enter text, then click Generate Embeddings'}</p>
                </div>
              )}
            </div>
          )}

          {/* Rerank Result */}
          {playModality === 'rerank' && (
            <div className="flex-1 flex flex-col p-4 overflow-y-auto">
              {rerankResult ? (
                <div className="w-full space-y-2">
                  {(rerankResult.results || []).map((r, idx) => (
                    <div key={idx} className="rounded-xl border border-slate-200 dark:border-slate-800 p-3 flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <span className="ui-meta font-mono block mb-1">#{idx + 1} · index {r.index}</span>
                        <p className="text-xs break-words">
                          {r.document?.text || r.document || (rerankDocs.split('\n').filter(Boolean)[r.index] ?? '')}
                        </p>
                      </div>
                      <span className="shrink-0 px-2 py-0.5 rounded-full bg-amber-50 dark:bg-amber-950/50 text-amber-700 dark:text-amber-300 text-[11px] font-mono font-semibold">
                        {(r.relevance_score ?? 0).toFixed(4)}
                      </span>
                    </div>
                  ))}
                  {(rerankResult.results || []).length === 0 && (
                    <pre className="text-[11px] font-mono">{JSON.stringify(rerankResult, null, 2)}</pre>
                  )}
                </div>
              ) : (
                <div className="flex-1 flex flex-col items-center justify-center text-center ui-meta space-y-2">
                  <Sliders className="w-12 h-12 stroke-1 text-slate-400" />
                  <p>{isZh ? '填写查询与候选文档后点击【执行重排】' : 'Fill in a query and documents, then click Run Rerank'}</p>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
