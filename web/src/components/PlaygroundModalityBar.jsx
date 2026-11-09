import { MessageSquare, Image as ImageIcon, Volume2, Mic, Video, Cpu, Sliders, Sparkles, Split } from 'lucide-react';

export default function PlaygroundModalityBar({
  isZh, playModality, playMode, setPlayModality, setPlayMode, setPlayStopped
}) {
  return (
    <div className="bg-white dark:bg-[#191B1F] border border-[#E4E6E9] dark:border-[#2C3038] rounded-2xl p-1.5 shadow-sm flex flex-wrap items-center justify-between gap-2">
      <div className="flex flex-wrap items-center gap-1.5">
        {[
          { id: 'gemini', label: isZh ? 'Gemini 原生' : 'Gemini Native', icon: Sparkles, activeColor: 'bg-[#355CFF] text-white' },
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
  );
}
