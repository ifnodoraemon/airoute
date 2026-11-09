import { Volume2, RefreshCw } from 'lucide-react';

export default function PlaygroundSpeechControls({
  handleGenerateTTS, isZh, playLoading, setTtsInput, setTtsModel, setTtsVoice, ttsInput, ttsModel, ttsModels,
  ttsVoice
}) {
  return (
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
  );
}
