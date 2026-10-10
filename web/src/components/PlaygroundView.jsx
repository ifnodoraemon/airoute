import React, { useState, useEffect, useRef } from 'react';
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
  Play,
  RefreshCw,
  Trash2,
  Copy,
  Check,
  Download,
  FileAudio,
  Send,
  Split,
  ChevronDown,
  ChevronUp,
  Clock,
  Zap,
  DollarSign,
  Plus
} from 'lucide-react';
import { getGatewayOrigin } from '../config';

export default function PlaygroundView({
  keys = [],
  models = [],
  modelRoutes = [],
  adminFetch,
  showToast,
  initialModel = '',
  initialModality = 'chat',
  lang = 'zh',
  t
}) {
  const isZh = lang === 'zh';
  const [playModality, setPlayModality] = useState(initialModality || 'chat');
  const [playMode, setPlayMode] = useState('single'); // 'single' | 'arena'
  const [playApiKey, setPlayApiKey] = useState('');
  const [isCustomKey, setIsCustomKey] = useState(false);
  const [customKeyInput, setCustomKeyInput] = useState('');

  // Auto-select initial key
  useEffect(() => {
    if (!playApiKey && keys.length > 0 && !isCustomKey) {
      setPlayApiKey(keys[0].key);
    }
  }, [keys, playApiKey, isCustomKey]);

  const getEffectiveApiKey = () => {
    if (isCustomKey) return customKeyInput.trim();
    if (playApiKey === '__none__') return '';
    if (playApiKey) return playApiKey;
    if (keys.length > 0) return keys[0].key;
    return '';
  };

  const selectedKeyObj = keys.find(k => k.key === (isCustomKey ? customKeyInput.trim() : (playApiKey === '__none__' ? '' : playApiKey)));

  const inferClientModality = (m) => {
    const lower = (m || '').toLowerCase();
    if (lower.includes('rerank') || lower.includes('bge-reranker')) return 'rerank';
    if (lower.includes('embed') || lower.includes('bge-') || lower.includes('text-embedding')) return 'embeddings';
    if (lower.includes('tts') || lower.includes('speech') || lower.includes('voice') || lower.includes('cosyvoice')) return 'audio_speech';
    if (lower.includes('whisper') || lower.includes('sensevoice') || lower.includes('transcription') || lower.includes('asr')) return 'audio_transcription';
    if (lower.includes('sora') || lower.includes('cogvideox') || lower.includes('kling') || lower.includes('video')) return 'videos';
    if (lower.includes('image') || lower.includes('dall-e') || lower.includes('flux') || lower.includes('stable-diffusion')) return 'images';
    return 'chat';
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
        const allowedSet = new Set(selectedKeyObj.allowed_models);
        const filtered = list.filter(m => allowedSet.has(m));
        const extra = selectedKeyObj.allowed_models.filter(m => inferClientModality(m) === modality && !filtered.includes(m));
        return [...filtered, ...extra];
      }
    }
    return list;
  };

  const chatModels = getPlaygroundModels('chat');
  const imgModels = getPlaygroundModels('images');
  const ttsModels = getPlaygroundModels('audio_speech');

  // Single Model state
  const [playModel, setPlayModel] = useState(initialModel || (chatModels[0] || ''));
  const [arenaModelB, setArenaModelB] = useState(chatModels[1] || chatModels[0] || '');
  const [playStream, setPlayStream] = useState(true);
  const [temperature, setTemperature] = useState(0.7);
  const [maxTokens, setMaxTokens] = useState(2048);
  const [systemPrompt, setSystemPrompt] = useState(
    isZh ? '你是一个极具专业素养的 AI 架构专家与全栈工程师，回答请严谨、精炼、富有洞见。' : 'You are a highly professional AI architecture expert and full-stack engineer. Answer concisely and with precision.'
  );
  const [showSystemPrompt, setShowSystemPrompt] = useState(false);

  // Chat conversation history & streaming output
  const [messages, setMessages] = useState([
    { role: 'user', content: isZh ? '请简要介绍 Airoute 企业级大模型与多模态网关的核心架构优势。' : 'Briefly introduce the architectural advantages of Airoute LLM and multimodal gateway.' }
  ]);
  const [chatInput, setChatInput] = useState('');
  const [playLoading, setPlayLoading] = useState(false);

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
  const [imgPrompt, setImgPrompt] = useState(isZh ? '极简未来数据中心机房，幽蓝光纤与量子矩阵，8k 渲染' : 'Minimalist futuristic data center, cyan optical fiber and quantum matrix, 8k render');
  const [imgSize, setImgSize] = useState('1024x1024');
  const [imgQuality, setImgQuality] = useState('standard');
  const [imgResult, setImgResult] = useState(null);

  const [ttsModel, setTtsModel] = useState(ttsModels[0] || '');
  const [ttsVoice, setTtsVoice] = useState('alloy');
  const [ttsSpeed, setTtsSpeed] = useState(1.0);
  const [ttsInput, setTtsInput] = useState(isZh ? '欢迎使用 Airoute 企业级大模型与多模态网关系统。' : 'Welcome to Airoute enterprise LLM and multimodal gateway system.');
  const [ttsAudioUrl, setTtsAudioUrl] = useState(null);

  // Sync initial model
  useEffect(() => {
    if (chatModels.length > 0 && !chatModels.includes(playModel)) {
      setPlayModel(chatModels[0]);
    }
    if (chatModels.length > 1 && !chatModels.includes(arenaModelB)) {
      setArenaModelB(chatModels[1]);
    }
  }, [chatModels, playModel, arenaModelB]);

  const copyToClipboard = (txt, label = '') => {
    navigator.clipboard.writeText(txt);
    if (showToast) showToast(isZh ? `${label || '内容'} 已复制到剪贴板！` : `${label || 'Content'} copied to clipboard!`, 'success');
  };

  // Run chat invocation
  const handleRunChat = async () => {
    if (!chatInput.trim() && messages.length === 0) return;
    const currentInput = chatInput.trim() || messages[messages.length - 1]?.content || '';
    if (!currentInput) return;

    const newMessages = [...messages, { role: 'user', content: currentInput }];
    setMessages(newMessages);
    setChatInput('');
    setPlayLoading(true);

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

    const payloadMessages = [
      ...(systemPrompt.trim() ? [{ role: 'system', content: systemPrompt.trim() }] : []),
      ...newMessages
    ];

    const runModelStream = async (targetModel, setOutput, setReasoning, setTTFT, setDuration, setTPS) => {
      const startTime = Date.now();
      let firstByteTime = null;
      let fullContent = '';
      let fullReasoning = '';

      try {
        const headers = { 'Content-Type': 'application/json' };
        const key = getEffectiveApiKey();
        if (key) headers['Authorization'] = `Bearer ${key}`;

        const res = await fetch('/v1/chat/completions', {
          method: 'POST',
          headers,
          body: JSON.stringify({
            model: targetModel,
            messages: payloadMessages,
            temperature,
            max_tokens: maxTokens,
            stream: playStream
          })
        });

        if (!res.ok) {
          const errData = await res.json().catch(() => ({}));
          setOutput(`[Error ${res.status}] ${errData.error?.message || res.statusText}`);
          return;
        }

        if (playStream && res.body) {
          const reader = res.body.getReader();
          const decoder = new TextDecoder('utf-8');
          let done = false;
          let buffer = '';

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

              for (const line of lines) {
                const trimmed = line.trim();
                if (trimmed.startsWith('data: ')) {
                  const dataStr = trimmed.slice(6);
                  if (dataStr === '[DONE]') continue;
                  try {
                    const parsed = JSON.parse(dataStr);
                    const delta = parsed.choices?.[0]?.delta;
                    if (delta?.reasoning_content) {
                      fullReasoning += delta.reasoning_content;
                      setReasoning(fullReasoning);
                    }
                    if (delta?.content) {
                      fullContent += delta.content;
                      setOutput(fullContent);
                    }
                  } catch (e) {}
                }
              }
            }
          }
        } else {
          const data = await res.json();
          firstByteTime = Date.now();
          setTTFT(firstByteTime - startTime);
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
        const estTokens = fullContent.length * 0.75;
        const activeSec = (totalMs - (firstByteTime ? firstByteTime - startTime : 0)) / 1000;
        if (activeSec > 0.1) {
          setTPS((estTokens / activeSec).toFixed(1));
        }
      } catch (e) {
        setOutput(`[Network Error] ${e.message}`);
      }
    };

    // Run Model A
    const promises = [runModelStream(playModel, setPlayOutput, setPlayReasoningOutput, setPlayTTFTMs, setPlayDurationMs, setPlayTokensPerSec)];
    // If Arena mode, run Model B in parallel!
    if (playMode === 'arena' && arenaModelB) {
      promises.push(runModelStream(arenaModelB, setArenaOutputB, setArenaReasoningB, setArenaTTFTB, setArenaDurationB, setArenaTokensPerSecB));
    }

    await Promise.all(promises);
    setPlayLoading(false);
  };

  // Image generation
  const handleGenerateImage = async () => {
    const key = getEffectiveApiKey();
    setPlayLoading(true);
    setImgResult(null);
    const start = Date.now();
    try {
      const headers = { 'Content-Type': 'application/json' };
      if (key) headers['Authorization'] = `Bearer ${key}`;
      const res = await fetch('/v1/images/generations', {
        method: 'POST',
        headers,
        body: JSON.stringify({
          model: imgModel,
          prompt: imgPrompt,
          size: imgSize,
          quality: imgQuality
        })
      });
      const data = await res.json();
      setPlayDurationMs(Date.now() - start);
      if (res.ok && data.data && data.data[0]) {
        setImgResult(data.data[0]);
      } else {
        if (showToast) showToast(isZh ? `绘图失败: ${data.error?.message || '未知错误'}` : `Image generation failed: ${data.error?.message || 'Unknown error'}`, 'error');
      }
    } catch (e) {
      if (showToast) showToast(e.message, 'error');
    } finally {
      setPlayLoading(false);
    }
  };

  // Audio Speech (TTS)
  const handleGenerateTTS = async () => {
    const key = getEffectiveApiKey();
    setPlayLoading(true);
    setTtsAudioUrl(null);
    const start = Date.now();
    try {
      const headers = { 'Content-Type': 'application/json' };
      if (key) headers['Authorization'] = `Bearer ${key}`;
      const res = await fetch('/v1/audio/speech', {
        method: 'POST',
        headers,
        body: JSON.stringify({
          model: ttsModel,
          input: ttsInput,
          voice: ttsVoice,
          speed: ttsSpeed,
          response_format: 'mp3'
        })
      });
      setPlayDurationMs(Date.now() - start);
      if (res.ok) {
        const blob = await res.blob();
        const url = URL.createObjectURL(blob);
        setTtsAudioUrl(url);
      } else {
        const err = await res.json().catch(() => ({}));
        if (showToast) showToast(isZh ? `TTS 失败: ${err.error?.message || '未知错误'}` : `TTS failed: ${err.error?.message || 'Unknown error'}`, 'error');
      }
    } catch (e) {
      if (showToast) showToast(e.message, 'error');
    } finally {
      setPlayLoading(false);
    }
  };

  return (
    <div className="space-y-4 animate-in fade-in duration-200">
      {/* 1. Modality Selector Bar */}
      <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-2xl p-1.5 shadow-xs flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap items-center gap-1.5">
          {[
            { id: 'chat', label: isZh ? 'Chat 对话' : 'Chat', icon: MessageSquare, activeColor: 'bg-indigo-600 text-white' },
            { id: 'images', label: isZh ? 'Images 绘图' : 'Images', icon: ImageIcon, activeColor: 'bg-pink-600 text-white' },
            { id: 'audio_speech', label: isZh ? 'TTS 语音合成' : 'TTS Speech', icon: Volume2, activeColor: 'bg-cyan-600 text-white' },
            { id: 'audio_transcription', label: isZh ? 'STT 语音转写' : 'STT Audio', icon: Mic, activeColor: 'bg-teal-600 text-white' },
            { id: 'videos', label: isZh ? 'Video 视频生成' : 'Videos', icon: Video, activeColor: 'bg-purple-600 text-white' },
            { id: 'embeddings', label: isZh ? 'Embedding 向量' : 'Embeddings', icon: Cpu, activeColor: 'bg-emerald-600 text-white' },
            { id: 'rerank', label: isZh ? 'Rerank 重排' : 'Rerank', icon: Sliders, activeColor: 'bg-amber-600 text-white' },
          ].map(tab => {
            const Icon = tab.icon;
            return (
              <button
                key={tab.id}
                onClick={() => setPlayModality(tab.id)}
                className={`flex items-center space-x-1.5 px-3 py-1.5 rounded-xl text-xs font-semibold transition cursor-pointer ${
                  playModality === tab.id
                    ? `${tab.activeColor} shadow-xs`
                    : 'text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800 hover:text-slate-900 dark:hover:text-slate-100'
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
          <div className="flex items-center space-x-1 bg-slate-100 dark:bg-slate-800/80 p-1 rounded-xl">
            <button
              onClick={() => setPlayMode('single')}
              className={`px-2.5 py-1 rounded-lg text-xs font-semibold transition cursor-pointer ${
                playMode === 'single'
                  ? 'bg-white dark:bg-slate-700 text-indigo-600 dark:text-indigo-400 shadow-xs'
                  : 'text-slate-500 dark:text-slate-400 hover:text-slate-800'
              }`}
            >
              {isZh ? '单模型' : 'Single Model'}
            </button>
            <button
              onClick={() => setPlayMode('arena')}
              className={`px-2.5 py-1 rounded-lg text-xs font-semibold flex items-center space-x-1 transition cursor-pointer ${
                playMode === 'arena'
                  ? 'bg-gradient-to-r from-indigo-600 to-purple-600 text-white shadow-xs'
                  : 'text-slate-500 dark:text-slate-400 hover:text-slate-800'
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
        <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-2xl p-6 space-y-4 shadow-xs h-fit">
          <div className="flex items-center justify-between border-b border-slate-100 dark:border-slate-800 pb-3">
            <h3 className="font-bold text-slate-900 dark:text-slate-100 text-sm flex items-center space-x-2">
              <Sparkles className="w-4 h-4 text-indigo-600 dark:text-indigo-400" />
              <span>{isZh ? '参数配置' : 'Hyperparameters'}</span>
            </h3>
          </div>

          {/* Key Selection */}
          <div>
            <label className="text-xs font-semibold text-slate-700 dark:text-slate-300 flex items-center justify-between mb-1">
              <span className="flex items-center space-x-1">
                <Key className="w-3.5 h-3.5 text-indigo-500" />
                <span>{isZh ? 'API 访问密钥' : 'API Key'}</span>
              </span>
              {keys.length > 0 && (
                <button
                  onClick={() => setIsCustomKey(!isCustomKey)}
                  className="text-[11px] text-indigo-600 dark:text-indigo-400 hover:underline cursor-pointer"
                >
                  {isCustomKey ? (isZh ? '下拉选择' : 'Select') : (isZh ? '自定义' : 'Custom')}
                </button>
              )}
            </label>

            {isCustomKey || keys.length === 0 ? (
              <input
                type="text"
                value={customKeyInput}
                onChange={(e) => setCustomKeyInput(e.target.value)}
                placeholder={isZh ? 'sk-airoute-xxxx (留空使用免密直通)' : 'sk-airoute-xxxx (leave empty for unauthenticated)'}
                className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-3 py-2 text-xs font-mono text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500"
              />
            ) : (
              <select
                value={playApiKey}
                onChange={(e) => setPlayApiKey(e.target.value)}
                className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-3 py-2 text-xs font-mono text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500"
              >
                {keys.map((k) => (
                  <option key={k.key || k.id} value={k.key}>
                    {k.tenant_id ? `${k.tenant_id} - ` : ''}{k.key.slice(0, 10)}...{k.key.slice(-4)}
                  </option>
                ))}
                <option value="__none__">{isZh ? '免密直通 (不带 API Key)' : 'Public / Unauthenticated'}</option>
              </select>
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
                  className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-3 py-2 text-xs font-mono text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500"
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
                    className="w-full bg-purple-50/50 dark:bg-purple-950/40 border border-purple-200 dark:border-purple-800 rounded-xl px-3 py-2 text-xs font-mono text-slate-800 dark:text-slate-100 focus:outline-none focus:border-purple-500"
                  >
                    {chatModels.map(m => (
                      <option key={m} value={m}>{m}</option>
                    ))}
                  </select>
                </div>
              )}

              {/* System Prompt Collapsible */}
              <div className="border border-slate-100 dark:border-slate-800 rounded-xl p-2.5">
                <button
                  onClick={() => setShowSystemPrompt(!showSystemPrompt)}
                  className="w-full flex items-center justify-between text-xs font-semibold text-slate-700 dark:text-slate-300 cursor-pointer"
                >
                  <span>{isZh ? '系统人设 (System Prompt)' : 'System Prompt'}</span>
                  {showSystemPrompt ? <ChevronUp className="w-3.5 h-3.5" /> : <ChevronDown className="w-3.5 h-3.5" />}
                </button>
                {showSystemPrompt && (
                  <textarea
                    rows={2}
                    value={systemPrompt}
                    onChange={(e) => setSystemPrompt(e.target.value)}
                    className="mt-2 w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-lg p-2 text-xs text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500"
                  />
                )}
              </div>

              {/* Hyperparameters: Temperature & MaxTokens */}
              <div className="space-y-3 pt-2 border-t border-slate-100 dark:border-slate-800">
                <div>
                  <div className="flex justify-between text-xs font-semibold text-slate-600 dark:text-slate-400 mb-1">
                    <span>{isZh ? '温度 (Temperature)' : 'Temperature'}</span>
                    <span className="font-mono text-indigo-600 dark:text-indigo-400">{temperature}</span>
                  </div>
                  <input
                    type="range"
                    min="0"
                    max="2"
                    step="0.1"
                    value={temperature}
                    onChange={(e) => setTemperature(parseFloat(e.target.value))}
                    className="w-full accent-indigo-600 cursor-pointer"
                  />
                </div>

                <div>
                  <div className="flex justify-between text-xs font-semibold text-slate-600 dark:text-slate-400 mb-1">
                    <span>{isZh ? '最大生成 Token (Max Tokens)' : 'Max Tokens'}</span>
                    <span className="font-mono text-indigo-600 dark:text-indigo-400">{maxTokens}</span>
                  </div>
                  <input
                    type="range"
                    min="256"
                    max="8192"
                    step="256"
                    value={maxTokens}
                    onChange={(e) => setMaxTokens(parseInt(e.target.value))}
                    className="w-full accent-indigo-600 cursor-pointer"
                  />
                </div>

                <div className="flex items-center space-x-2 pt-1">
                  <input
                    type="checkbox"
                    id="streamCheck"
                    checked={playStream}
                    onChange={(e) => setPlayStream(e.target.checked)}
                    className="rounded border-slate-300 text-indigo-600 focus:ring-0 cursor-pointer"
                  />
                  <label htmlFor="streamCheck" className="text-xs font-semibold text-slate-700 dark:text-slate-300 cursor-pointer">
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
                  className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-3 py-2 text-xs font-mono text-slate-800 dark:text-slate-100"
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
                  className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-3 py-2 text-xs text-slate-800 dark:text-slate-100"
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
                  className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl p-2.5 text-xs text-slate-800 dark:text-slate-100 focus:outline-none focus:border-pink-500"
                />
              </div>

              <button
                onClick={handleGenerateImage}
                disabled={playLoading}
                className="w-full py-2.5 bg-pink-600 hover:bg-pink-700 text-white rounded-xl text-xs font-bold shadow-xs transition flex items-center justify-center space-x-1.5 cursor-pointer"
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
                  className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-3 py-2 text-xs font-mono text-slate-800 dark:text-slate-100"
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
                  className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-3 py-2 text-xs text-slate-800 dark:text-slate-100"
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
                  className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl p-2.5 text-xs text-slate-800 dark:text-slate-100"
                />
              </div>

              <button
                onClick={handleGenerateTTS}
                disabled={playLoading}
                className="w-full py-2.5 bg-cyan-600 hover:bg-cyan-700 text-white rounded-xl text-xs font-bold shadow-xs transition flex items-center justify-center space-x-1.5 cursor-pointer"
              >
                {playLoading ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Volume2 className="w-3.5 h-3.5" />}
                <span>{playLoading ? (isZh ? '合成中...' : 'Synthesizing...') : (isZh ? '生成语音' : 'Synthesize Speech')}</span>
              </button>
            </div>
          )}

          {/* Telemetry Summary Footer */}
          <div className="pt-3 border-t border-slate-100 dark:border-slate-800 text-xs text-slate-500 dark:text-slate-400 space-y-1.5">
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
        <div className="lg:col-span-2 bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-2xl p-6 flex flex-col h-[700px] shadow-xs">
          {/* Header Action Bar */}
          <div className="flex items-center justify-between pb-3 border-b border-slate-100 dark:border-slate-800 mb-3 text-xs">
            <div className="flex items-center space-x-2">
              <Terminal className="w-4 h-4 text-indigo-600 dark:text-indigo-400" />
              <span className="font-bold text-slate-800 dark:text-slate-200">
                {playModality === 'chat'
                  ? (playMode === 'arena' ? (isZh ? '双模型横向比对评测竞技场 (Arena)' : 'Model Arena Side-by-Side Comparison') : (isZh ? '连续多轮对话调试' : 'Conversational Testing Stream'))
                  : (isZh ? '多模态结果呈现' : 'Multimodal Output Stream')}
              </span>
            </div>

            <div className="flex items-center space-x-2">
              <button
                onClick={() => {
                  const curlCmd = `curl -X POST "${getGatewayOrigin()}/v1/chat/completions" \\\n  -H "Authorization: Bearer ${getEffectiveApiKey()}" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model": "${playModel}", "messages": [{"role": "user", "content": "Hello"}], "stream": true}'`;
                  copyToClipboard(curlCmd, 'cURL');
                }}
                className="px-2.5 py-1 bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-200 rounded-lg font-medium transition flex items-center space-x-1 cursor-pointer"
              >
                <Code className="w-3 h-3 text-indigo-500" />
                <span>{isZh ? '复制 cURL' : 'Copy cURL'}</span>
              </button>

              <button
                onClick={() => {
                  setMessages([]);
                  setPlayOutput('');
                  setPlayReasoningOutput('');
                  setArenaOutputB('');
                  setImgResult(null);
                  setTtsAudioUrl(null);
                }}
                className="px-2 py-1 text-slate-400 hover:text-rose-600 transition text-[11px] cursor-pointer"
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
                  {/* Messages Bubble Stream */}
                  {messages.map((msg, idx) => (
                    <div
                      key={idx}
                      className={`flex flex-col ${msg.role === 'user' ? 'items-end' : 'items-start'}`}
                    >
                      <div className="text-[10px] text-slate-400 font-mono mb-1">
                        {msg.role === 'user' ? (isZh ? '调用方 (User)' : 'User') : `${playModel} (Assistant)`}
                      </div>
                      <div
                        className={`max-w-[85%] rounded-2xl p-3.5 text-xs leading-relaxed ${
                          msg.role === 'user'
                            ? 'bg-indigo-600 text-white'
                            : 'bg-white dark:bg-slate-800 text-slate-800 dark:text-slate-100 border border-slate-200/80 dark:border-slate-700 shadow-xs'
                        }`}
                      >
                        <div className="whitespace-pre-wrap">{msg.content}</div>
                      </div>
                    </div>
                  ))}

                  {/* Reasoning Content & Assistant Live Delta */}
                  {playReasoningOutput && (
                    <div className="p-3.5 bg-amber-50/70 dark:bg-amber-950/30 border border-amber-200/80 dark:border-amber-800/60 rounded-2xl text-xs text-amber-950 dark:text-amber-200">
                      <div className="font-bold flex items-center space-x-1.5 text-amber-800 dark:text-amber-400 mb-1">
                        <Sparkles className="w-3.5 h-3.5 animate-pulse" />
                        <span>{isZh ? '深度思维链 (Reasoning Content):' : 'Reasoning Process:'}</span>
                      </div>
                      <div className="whitespace-pre-wrap text-[11px] font-mono leading-relaxed">
                        {playReasoningOutput}
                      </div>
                    </div>
                  )}

                  {playOutput && (
                    <div className="flex flex-col items-start">
                      <div className="text-[10px] text-indigo-600 dark:text-indigo-400 font-mono mb-1">
                        {playModel} {isZh ? '(响应输出)' : '(Response)'}
                      </div>
                      <div className="max-w-[90%] rounded-2xl p-3.5 text-xs bg-white dark:bg-slate-800 text-slate-800 dark:text-slate-100 border border-slate-200/80 dark:border-slate-700 shadow-xs whitespace-pre-wrap leading-relaxed">
                        {playOutput}
                      </div>
                    </div>
                  )}
                </div>
              ) : (
                /* Side-by-Side Arena Split View */
                <div className="flex-1 grid grid-cols-2 gap-3 min-h-0">
                  {/* Left Contender: Model A */}
                  <div className="flex flex-col rounded-xl border border-indigo-200 dark:border-indigo-800/80 bg-slate-50/60 dark:bg-slate-900/40 p-3 overflow-y-auto space-y-3">
                    <div className="flex items-center justify-between pb-2 border-b border-indigo-100 dark:border-indigo-900 text-xs">
                      <span className="font-bold text-indigo-600 dark:text-indigo-400 font-mono">
                        Model A: {playModel}
                      </span>
                      <div className="text-[10px] font-mono text-slate-500 space-x-2">
                        <span>TTFT: {playTTFTMs ? `${playTTFTMs}ms` : '-'}</span>
                        <span>{isZh ? '速度' : 'Speed'}: {playTokensPerSec ? `${playTokensPerSec}tps` : '-'}</span>
                      </div>
                    </div>
                    {playReasoningOutput && (
                      <div className="p-2.5 rounded-xl bg-amber-50 dark:bg-amber-950/40 border border-amber-200 text-[11px] text-amber-900 dark:text-amber-200 font-mono whitespace-pre-wrap">
                        {playReasoningOutput}
                      </div>
                    )}
                    <div className="text-xs text-slate-800 dark:text-slate-100 whitespace-pre-wrap leading-relaxed">
                      {playOutput || (
                        <div className="text-center py-24 text-slate-400 text-xs">
                          {isZh ? '等待发送评测指令...' : 'Awaiting prompt evaluation...'}
                        </div>
                      )}
                    </div>
                  </div>

                  {/* Right Contender: Model B */}
                  <div className="flex flex-col rounded-xl border border-purple-200 dark:border-purple-800/80 bg-purple-50/30 dark:bg-purple-950/20 p-3 overflow-y-auto space-y-3">
                    <div className="flex items-center justify-between pb-2 border-b border-purple-100 dark:border-purple-900 text-xs">
                      <span className="font-bold text-purple-600 dark:text-purple-400 font-mono">
                        Model B: {arenaModelB}
                      </span>
                      <div className="text-[10px] font-mono text-slate-500 space-x-2">
                        <span>TTFT: {arenaTTFTB ? `${arenaTTFTB}ms` : '-'}</span>
                        <span>{isZh ? '速度' : 'Speed'}: {arenaTokensPerSecB ? `${arenaTokensPerSecB}tps` : '-'}</span>
                      </div>
                    </div>
                    {arenaReasoningB && (
                      <div className="p-2.5 rounded-xl bg-amber-50 dark:bg-amber-950/40 border border-amber-200 text-[11px] text-amber-900 dark:text-amber-200 font-mono whitespace-pre-wrap">
                        {arenaReasoningB}
                      </div>
                    )}
                    <div className="text-xs text-slate-800 dark:text-slate-100 whitespace-pre-wrap leading-relaxed">
                      {arenaOutputB || (
                        <div className="text-center py-24 text-slate-400 text-xs">
                          {isZh ? '等待发送评测指令...' : 'Awaiting prompt evaluation...'}
                        </div>
                      )}
                    </div>
                  </div>
                </div>
              )}

              {/* Chat Input Bar */}
              <div className="mt-3 flex items-center space-x-2">
                <input
                  type="text"
                  value={chatInput}
                  onChange={(e) => setChatInput(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && !e.shiftKey) {
                      e.preventDefault();
                      handleRunChat();
                    }
                  }}
                  placeholder={playMode === 'arena' ? (isZh ? '输入统一 Prompt，同时对两款模型展开并发比对...' : 'Enter prompt to benchmark both models simultaneously...') : (isZh ? '输入对话 Prompt，按回车发送...' : 'Enter prompt, press Enter to send...')}
                  className="flex-1 bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-4 py-2.5 text-xs text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500"
                />
                <button
                  onClick={handleRunChat}
                  disabled={playLoading}
                  className={`px-5 py-2.5 text-white rounded-xl text-xs font-bold shadow-xs transition flex items-center space-x-1.5 cursor-pointer ${
                    playMode === 'arena'
                      ? 'bg-gradient-to-r from-indigo-600 to-purple-600 hover:from-indigo-700 hover:to-purple-700'
                      : 'bg-indigo-600 hover:bg-indigo-700'
                  }`}
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
                  <div className="relative group max-h-[460px] overflow-hidden rounded-2xl border border-slate-200 dark:border-slate-800 shadow-md">
                    <img
                      src={imgResult.url || `data:image/png;base64,${imgResult.b64_json}`}
                      alt="Generated"
                      className="max-h-[440px] w-auto object-contain mx-auto"
                    />
                  </div>
                  <a
                    href={imgResult.url || `data:image/png;base64,${imgResult.b64_json}`}
                    download="airoute-image.png"
                    target="_blank"
                    rel="noreferrer"
                    className="px-4 py-2 bg-pink-600 hover:bg-pink-700 text-white text-xs font-bold rounded-xl flex items-center space-x-1.5 transition shadow-xs cursor-pointer"
                  >
                    <Download className="w-3.5 h-3.5" />
                    <span>{isZh ? '下载高清原图' : 'Download High-Res'}</span>
                  </a>
                </div>
              ) : (
                <div className="text-center text-slate-400 space-y-2">
                  <ImageIcon className="w-12 h-12 stroke-1 mx-auto text-slate-300" />
                  <p className="text-xs">{isZh ? '点击左侧【生成图像】开始 AI 视觉渲染' : 'Click [Generate Image] on the left to start AI rendering'}</p>
                </div>
              )}
            </div>
          )}

          {/* TTS Audio Player Result */}
          {playModality === 'audio_speech' && (
            <div className="flex-1 flex flex-col items-center justify-center p-4 space-y-4">
              {ttsAudioUrl ? (
                <div className="w-full max-w-md p-6 bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-2xl flex flex-col items-center space-y-4 shadow-xs">
                  <div className="w-12 h-12 rounded-2xl bg-cyan-50 dark:bg-cyan-950/60 text-cyan-600 dark:text-cyan-400 flex items-center justify-center shadow-xs">
                    <Volume2 className="w-6 h-6" />
                  </div>
                  <span className="font-bold text-xs text-slate-800 dark:text-slate-200">
                    {isZh ? '语音合成渲染完成 (MP3 直通流)' : 'Audio Synthesis Completed (Direct MP3 Stream)'}
                  </span>
                  <audio controls src={ttsAudioUrl} className="w-full" autoPlay />
                </div>
              ) : (
                <div className="text-center text-slate-400 space-y-2">
                  <FileAudio className="w-12 h-12 stroke-1 mx-auto text-slate-300" />
                  <p className="text-xs">{isZh ? '点击左侧【生成语音】开始流式音频合成' : 'Click [Synthesize Speech] on the left to start streaming audio'}</p>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
