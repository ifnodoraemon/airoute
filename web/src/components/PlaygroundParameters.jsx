import { Mic, Video, Cpu, Sliders, Sparkles, RefreshCw, ChevronDown, ChevronUp } from 'lucide-react';
import ThinkingControls from './ThinkingControls';
import PlaygroundHyperparameters from './PlaygroundHyperparameters';
import PlaygroundImageControls from './PlaygroundImageControls';
import PlaygroundSpeechControls from './PlaygroundSpeechControls';

export default function PlaygroundParameters({
  arenaModelB, chatModels, embedInput, embedModel, embedModels, handleEmbed, handleGenerateImage,
  handleGenerateTTS, handleGenerateVideo, handleOptimizeSystemPrompt, handleRerank, handleTranscribe,
  imgModel, imgModels, imgPrompt, imgSize, isZh, lang, maxTokens, playDurationMs, playLoading, playModality,
  playMode, playModel, playStream, playTTFTMs, playTokensPerSec, rerankDocs, rerankModel, rerankModels,
  rerankQuery, setArenaModelB, setEmbedInput, setEmbedModel, setImgModel, setImgPrompt, setImgSize,
  setMaxTokens, setPlayModel, setPlayStream, setRerankDocs, setRerankModel, setRerankQuery,
  setShowSystemPrompt, setSttFile, setSttLanguage, setSttModel, setSystemPrompt, setTemperature,
  setThinkingByModel, setTtsInput, setTtsModel, setTtsVoice, setVideoModel, setVideoPrompt, showSystemPrompt,
  sttFile, sttLanguage, sttModel, sttModels, systemPrompt, temperature, thinkingByModel, thinkingCapabilities,
  ttsInput, ttsModel, ttsModels, ttsVoice, videoModel, videoModels, videoPrompt, videoTaskId
}) {
  return (
    <div className="bg-white dark:bg-[#191B1F] border border-[#E4E6E9] dark:border-[#2C3038] rounded-2xl p-6 space-y-4 shadow-sm h-fit">
      <div className="flex items-center justify-between border-b border-[#E4E6E9] dark:border-[#2C3038] pb-3">
        <h3 className="font-bold text-[#202124] dark:text-[#F1F2F4] text-sm flex items-center space-x-2">
          <Sparkles className="w-4 h-4 text-[#355CFF] dark:text-[#91A7FF]" />
          <span>{isZh ? '参数配置' : 'Hyperparameters'}</span>
        </h3>
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
                <span>{isZh ? '系统提示词' : 'System Prompt'}</span>
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

          <ThinkingControls maxTokens={maxTokens} capability={thinkingCapabilities[playModel]} value={thinkingByModel[playModel]} onChange={value => setThinkingByModel(prev => ({...prev,[playModel]:value}))} disabled={playLoading} lang={lang} />
          {playMode === 'arena' && arenaModelB && <div className="border-t pt-2 space-y-2"><p className="text-xs font-semibold">{arenaModelB}</p><ThinkingControls maxTokens={maxTokens} capability={thinkingCapabilities[arenaModelB]} value={thinkingByModel[arenaModelB]} onChange={value => setThinkingByModel(prev => ({...prev,[arenaModelB]:value}))} disabled={playLoading} lang={lang} /></div>}
          {/* Hyperparameters: Temperature & MaxTokens */}
          <PlaygroundHyperparameters {...{
            isZh, maxTokens, playStream, setMaxTokens, setPlayStream, setTemperature, temperature
          }} />
        </>
      )}

      {/* Image Modality Controls */}
      {playModality === 'images' && (
        <PlaygroundImageControls {...{
          handleGenerateImage, imgModel, imgModels, imgPrompt, imgSize, isZh, playLoading, setImgModel, setImgPrompt,
          setImgSize
        }} />
      )}

      {/* TTS Modality Controls */}
      {playModality === 'audio_speech' && (
        <PlaygroundSpeechControls {...{
          handleGenerateTTS, isZh, playLoading, setTtsInput, setTtsModel, setTtsVoice, ttsInput, ttsModel, ttsModels,
          ttsVoice
        }} />
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
  );
}
