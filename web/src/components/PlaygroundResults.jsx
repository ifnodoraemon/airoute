import PlaygroundResultHeader from './PlaygroundResultHeader';
import PlaygroundChatBody from './PlaygroundChatBody';
import { Image as ImageIcon, Volume2, Mic, Video, Cpu, Sliders, Download, FileAudio } from 'lucide-react';

export default function PlaygroundResults({
  activeChatModel, arenaDurationB, arenaModelB, arenaOutputB, arenaReasoningB, arenaTTFTB, arenaThinkingB,
  arenaTokensPerSecB, chatInput, chatInputRef, curlUnsupportedReason, embedResult, handleCopyCurl,
  handleOptimizeChatInput, handleRunChat, handleStopGeneration, imgResult, isZh, lang, messages, offPeak,
  playLoading, playModality, playMode, playModel, playOutput, playReasoningOutput, playStopped, playTTFTMs,
  playThinking, playTokensPerSec, rerankDocs, rerankResult, setArenaOutputB, setChatInput, setImgResult,
  setMessages, setPlayOutput, setPlayReasoningOutput, setPlayStopped, setTtsAudioUrl, showSystemPrompt,
  sttResult, systemPrompt, ttsAudioUrl, videoResult
}) {
  return (
    <div className="lg:col-span-2 bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-2xl p-6 flex flex-col h-[min(700px,calc(100vh-16rem))] min-h-[360px] shadow-sm">
      {/* Header Action Bar */}
      <PlaygroundResultHeader {...{
        curlUnsupportedReason, handleCopyCurl, handleStopGeneration, isZh, playLoading, playModality, playMode,
        playStopped, setArenaOutputB, setImgResult, setMessages, setPlayOutput, setPlayReasoningOutput,
        setPlayStopped, setTtsAudioUrl
      }} />

      {/* Chat Body */}
      {playModality === 'chat' && (
        <PlaygroundChatBody {...{
          activeChatModel, arenaDurationB, arenaModelB, arenaOutputB, arenaReasoningB, arenaTTFTB, arenaThinkingB,
          arenaTokensPerSecB, chatInput, chatInputRef, handleOptimizeChatInput, handleRunChat, isZh, lang, messages,
          offPeak, playLoading, playMode, playModel, playOutput, playReasoningOutput, playTTFTMs, playThinking,
          playTokensPerSec, setChatInput, showSystemPrompt, systemPrompt
        }} />
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
  );
}
