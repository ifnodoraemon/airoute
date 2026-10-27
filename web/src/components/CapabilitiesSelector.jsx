import { Layers, MessageSquare, Zap, Terminal, Sparkles, Image, Volume2, Mic, Cpu, Sliders, Video } from 'lucide-react';

const CAPABILITIES = [
  { id: 'openai_chat', name: 'Chat', nameZh: '对话', path: '/v1/chat/completions', icon: MessageSquare, color: 'text-indigo-500' },
  { id: 'openai_response', name: 'Responses', nameZh: '智能体响应', path: '/v1/responses', icon: Zap, color: 'text-emerald-500' },
  { id: 'openai_text', name: 'Text Completions', nameZh: 'Text 补全', path: '/v1/completions', icon: Terminal, color: 'text-sky-500' },
  { id: 'anthropic_messages', name: 'Claude Messages', nameZh: 'Claude 消息', path: '/v1/messages', icon: Sparkles, color: 'text-amber-500' },
  // A marker, not an extra route: it records that the upstream already speaks the
  // Anthropic protocol natively, so the gateway forwards instead of translating.
  // It is opt-in — a new channel does not claim native support by default.
  {
    id: 'anthropic_native',
    name: 'Anthropic Native',
    nameZh: 'Anthropic 原生',
    path: 'upstream-native',
    hint: 'Upstream speaks Anthropic natively (no protocol translation)',
    hintZh: '上游原生支持 Anthropic 协议，网关不再做协议转换',
    icon: Sparkles,
    color: 'text-orange-500',
  },
  { id: 'images', name: 'Images', nameZh: '图像生成', path: '/v1/images/generations', icon: Image, color: 'text-pink-500' },
  { id: 'audio_speech', name: 'TTS', nameZh: '语音合成', path: '/v1/audio/speech', icon: Volume2, color: 'text-cyan-500' },
  { id: 'audio_transcription', name: 'STT', nameZh: '语音识别', path: '/v1/audio/transcriptions', icon: Mic, color: 'text-teal-500' },
  { id: 'embeddings', name: 'Embeddings', nameZh: '向量嵌入', path: '/v1/embeddings', icon: Cpu, color: 'text-emerald-500' },
  { id: 'rerank', name: 'Rerank', nameZh: '文档重排', path: '/v1/rerank', icon: Sliders, color: 'text-amber-500' },
  { id: 'videos', name: 'Videos', nameZh: '视频生成', path: '/v1/videos/generations', icon: Video, color: 'text-purple-500' },
];

export default function CapabilitiesSelector({ protocols = [], onToggle, onSetProtocols, lang = 'zh' }) {
  const isZh = lang === 'zh';

  // Protocol ids the console does not know about (a newer gateway, or a value set
  // through the API) must survive a bulk action: silently dropping them would make
  // saving this form rewrite a channel's capabilities.
  const unknownProtocols = protocols.filter(id => !CAPABILITIES.some(cap => cap.id === id));

  const handleSelectAll = () => {
    onSetProtocols([...CAPABILITIES.map(c => c.id), ...unknownProtocols]);
  };

  const handleSelectChatOnly = () => {
    onSetProtocols(['openai_chat', 'openai_response', 'openai_text', 'anthropic_messages', ...unknownProtocols]);
  };

  const handleClear = () => {
    onSetProtocols([...unknownProtocols]);
  };

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <label className="text-xs font-semibold text-slate-700 dark:text-slate-300 flex items-center space-x-1.5">
          <Layers className="w-3.5 h-3.5 text-[#355CFF] dark:text-[#91A7FF]" />
          <span>{isZh ? `协议能力 (${protocols.length}/${CAPABILITIES.length})` : `Protocol Capabilities (${protocols.length}/${CAPABILITIES.length})`}</span>
        </label>
        <div className="flex items-center space-x-2 text-xs">
          <button
            type="button"
            onClick={handleSelectAll}
            className="text-[#355CFF] dark:text-[#91A7FF] hover:underline"
          >
            {isZh ? '全选' : 'Select All'}
          </button>
          <span className="text-slate-300 dark:text-slate-700">·</span>
          <button
            type="button"
            onClick={handleSelectChatOnly}
            className="text-[#355CFF] dark:text-[#91A7FF] hover:underline"
          >
            {isZh ? '仅对话' : 'Chat Only'}
          </button>
          <span className="text-slate-300 dark:text-slate-700">·</span>
          <button
            type="button"
            onClick={handleClear}
            className="text-rose-500 hover:underline"
          >
            {isZh ? '清空' : 'Clear'}
          </button>
        </div>
      </div>

      <div className="grid grid-cols-2 sm:grid-cols-5 gap-2 pt-0.5">
        {CAPABILITIES.map(cap => {
          const isChecked = protocols.includes(cap.id);
          const Icon = cap.icon;
          return (
            <button
              key={cap.id}
              type="button"
              onClick={() => onToggle(cap.id)}
              title={isZh ? (cap.hintZh || cap.path) : (cap.hint || cap.path)}
              className={`p-2.5 rounded-lg border text-left transition flex flex-col justify-between ${
                isChecked
                  ? 'bg-[#EEF2FF] dark:bg-[#242D48] border-[#355CFF]/30 dark:border-[#91A7FF]/40 text-[#202124] dark:text-[#F1F2F4] shadow-sm'
                  : 'bg-white dark:bg-[#191B1F] border-[#E4E6E9] dark:border-[#2C3038] text-[#656B75] dark:text-[#A1A7B0] hover:border-[#355CFF]/40 opacity-70 hover:opacity-100'
              }`}
            >
              <div className="flex items-center justify-between w-full">
                <Icon className={`w-4 h-4 ${isChecked ? cap.color : 'text-slate-400'}`} />
                <span className={`w-3.5 h-3.5 rounded-md border flex items-center justify-center text-[10px] ${
                  isChecked
                    ? 'bg-[#355CFF] border-[#355CFF] text-white'
                    : 'border-[#E4E6E9] dark:border-[#2C3038]'
                }`}>
                  {isChecked ? '✓' : ''}
                </span>
              </div>
              <div className="mt-2">
                <div className="text-xs font-semibold leading-tight">{isZh ? (cap.nameZh || cap.name) : cap.name}</div>
                <div className="text-xs text-slate-500 dark:text-slate-400 font-mono truncate mt-0.5">{cap.path}</div>
              </div>
            </button>
          );
        })}
      </div>
    </div>
  );
}
