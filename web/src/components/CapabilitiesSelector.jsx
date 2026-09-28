import React from 'react';
import { Layers, MessageSquare, Zap, Terminal, Sparkles, Image, Volume2, Mic, Cpu, Sliders, Video } from 'lucide-react';

const CAPABILITIES = [
  { id: 'openai_chat', name: 'Chat', path: '/v1/chat/completions', icon: MessageSquare, color: 'text-indigo-500' },
  { id: 'openai_response', name: 'Responses', path: '/v1/responses', icon: Zap, color: 'text-emerald-500' },
  { id: 'anthropic_messages', name: 'Claude Messages', path: '/v1/messages', icon: Sparkles, color: 'text-amber-500' },
  { id: 'images', name: 'Images', path: '/v1/images/generations', icon: Image, color: 'text-pink-500' },
  { id: 'audio_speech', name: 'TTS', path: '/v1/audio/speech', icon: Volume2, color: 'text-cyan-500' },
  { id: 'audio_transcription', name: 'STT', path: '/v1/audio/transcriptions', icon: Mic, color: 'text-teal-500' },
  { id: 'embeddings', name: 'Embeddings', path: '/v1/embeddings', icon: Cpu, color: 'text-emerald-500' },
  { id: 'rerank', name: 'Rerank', path: '/v1/rerank', icon: Sliders, color: 'text-amber-500' },
  { id: 'videos', name: 'Videos', path: '/v1/videos/generations', icon: Video, color: 'text-purple-500' },
];

export default function CapabilitiesSelector({ protocols = [], onToggle, onSetProtocols }) {
  const handleSelectAll = () => {
    onSetProtocols(CAPABILITIES.map(c => c.id));
  };

  const handleSelectChatOnly = () => {
    onSetProtocols(['openai_chat', 'openai_response', 'anthropic_messages']);
  };

  const handleClear = () => {
    onSetProtocols([]);
  };

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <label className="text-xs font-semibold text-slate-700 dark:text-slate-300 flex items-center space-x-1.5">
          <Layers className="w-3.5 h-3.5 text-indigo-500" />
          <span>协议能力 ({protocols.length}/{CAPABILITIES.length})</span>
        </label>
        <div className="flex items-center space-x-2 text-[11px]">
          <button
            type="button"
            onClick={handleSelectAll}
            className="text-indigo-600 dark:text-indigo-400 hover:underline"
          >
            全选
          </button>
          <span className="text-slate-300 dark:text-slate-700">·</span>
          <button
            type="button"
            onClick={handleSelectChatOnly}
            className="text-indigo-600 dark:text-indigo-400 hover:underline"
          >
            仅对话
          </button>
          <span className="text-slate-300 dark:text-slate-700">·</span>
          <button
            type="button"
            onClick={handleClear}
            className="text-rose-500 hover:underline"
          >
            清空
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
              className={`p-2.5 rounded-2xl border text-left transition flex flex-col justify-between ${
                isChecked
                  ? 'bg-indigo-50/70 dark:bg-indigo-950/50 border-indigo-300 dark:border-indigo-700 text-indigo-950 dark:text-indigo-200 shadow-2xs'
                  : 'bg-white dark:bg-slate-900 border-slate-200 dark:border-slate-800 text-slate-500 hover:border-slate-300 dark:hover:border-slate-700 opacity-60 hover:opacity-90'
              }`}
            >
              <div className="flex items-center justify-between w-full">
                <Icon className={`w-4 h-4 ${isChecked ? cap.color : 'text-slate-400'}`} />
                <span className={`w-3.5 h-3.5 rounded-md border flex items-center justify-center text-[10px] ${
                  isChecked
                    ? 'bg-indigo-600 border-indigo-600 text-white'
                    : 'border-slate-300 dark:border-slate-700'
                }`}>
                  {isChecked ? '✓' : ''}
                </span>
              </div>
              <div className="mt-2">
                <div className="text-xs font-semibold leading-tight">{cap.name}</div>
                <div className="text-[10px] text-slate-400 font-mono truncate mt-0.5">{cap.path}</div>
              </div>
            </button>
          );
        })}
      </div>
    </div>
  );
}
