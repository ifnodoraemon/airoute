import { Code, Terminal, Square } from 'lucide-react';

export default function PlaygroundResultHeader({
  curlUnsupportedReason, handleCopyCurl, handleStopGeneration, isZh, playLoading, playModality, playMode,
  playStopped, setArenaOutputB, setImgResult, setMessages, setPlayOutput, setPlayReasoningOutput,
  setPlayStopped, setTtsAudioUrl
}) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-2 pb-3 border-b border-slate-100 dark:border-slate-800 mb-3 text-xs">
      <div className="flex items-center space-x-2">
        <Terminal className="w-4 h-4 text-indigo-600 dark:text-indigo-400" />
        <span className="font-bold text-slate-800 dark:text-slate-200">
          {playModality === 'chat'
            ? (playMode === 'arena' ? (isZh ? '双模型对比' : 'Model Arena Side-by-Side Comparison') : (isZh ? '对话' : 'Chat'))
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
  );
}
