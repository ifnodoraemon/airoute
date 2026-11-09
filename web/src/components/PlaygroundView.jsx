import usePlaygroundMedia from '../hooks/usePlaygroundMedia';
import usePlaygroundChat from '../hooks/usePlaygroundChat';
import usePlaygroundRequests from '../hooks/usePlaygroundRequests';
import usePlaygroundPrompts from '../hooks/usePlaygroundPrompts';
import usePlaygroundState from '../hooks/usePlaygroundState';
import usePlaygroundCatalog from '../hooks/usePlaygroundCatalog';
import PlaygroundParameters from './PlaygroundParameters';
import PlaygroundResults from './PlaygroundResults';
import PlaygroundModalityBar from './PlaygroundModalityBar';

import { Key } from 'lucide-react';

import GeminiPlayground from './GeminiPlayground';

import { availableKeyModels } from '../lib/keyModels';
import { keyDisplayRef } from '../lib/apiKey';

export default function PlaygroundView({
  keys = [],
  models = [],
  thinkingCapabilities = {},
  modelRoutes = [],
  prices = [],
  showToast,
  initialModel = '',
  initialModality = 'chat',
  initialScenario = null,
  onCreateKey,
  lang = 'zh',

}) {
  const isZh = lang === 'zh';
  const {
    activeChatModel, arenaModelB, chatModels, contextKeys, embedModels, imgModels, offPeak, playModality,
    playMode, playModel, rerankModels, resolveCallCredentials, selectedKeyId, selectedKeyObj, setArenaModelB,
    setPlayModality, setPlayMode, setPlayModel, setSelectedKeyId, sttModels, ttsModels, videoModels
  } = usePlaygroundCatalog({
    initialModality, initialModel, isZh, keys, modelRoutes, models, prices, showToast
  });
  const {
    abortRef, arenaDurationB, arenaOutputB, arenaReasoningB, arenaTTFTB, arenaThinkingB, arenaTokensPerSecB,
    chatInput, chatInputRef, embedInput, embedModel, embedResult, imgModel, imgPrompt, imgQuality, imgResult,
    imgSize, maxTokens, messages, playDurationMs, playLoading, playOutput, playReasoningOutput, playStopped,
    playStream, playTTFTMs, playThinking, playTokensPerSec, rerankDocs, rerankModel, rerankQuery, rerankResult,
    setArenaDurationB, setArenaOutputB, setArenaReasoningB, setArenaTTFTB, setArenaThinkingB,
    setArenaTokensPerSecB, setChatInput, setEmbedInput, setEmbedModel, setEmbedResult, setImgModel,
    setImgPrompt, setImgResult, setImgSize, setMaxTokens, setMessages, setPlayDurationMs, setPlayLoading,
    setPlayOutput, setPlayReasoningOutput, setPlayStopped, setPlayStream, setPlayTTFTMs, setPlayThinking,
    setPlayTokensPerSec, setRerankDocs, setRerankModel, setRerankQuery, setRerankResult, setShowSystemPrompt,
    setSttFile, setSttLanguage, setSttModel, setSttResult, setSystemPrompt, setTemperature, setThinkingByModel,
    setTtsAudioUrl, setTtsInput, setTtsModel, setTtsVoice, setVideoModel, setVideoPrompt, setVideoResult,
    setVideoTaskId, showSystemPrompt, sttFile, sttLanguage, sttModel, sttResult, systemPrompt, temperature,
    thinkingByModel, ttsAudioUrl, ttsInput, ttsModel, ttsSpeed, ttsVoice, videoModel, videoPrompt, videoResult,
    videoTaskId
  } = usePlaygroundState({
    arenaModelB, chatModels, embedModels, imgModels, initialModality, initialModel, initialScenario, isZh,
    playModel, rerankModels, setArenaModelB, setPlayModality, setPlayModel, sttModels, ttsModels, videoModels
  });

  const {
    copyToClipboard, handleOptimizeChatInput, handleOptimizeSystemPrompt
  } = usePlaygroundPrompts({
    chatInput, chatInputRef, isZh, playModality, setChatInput, setShowSystemPrompt, setSystemPrompt, showToast,
    systemPrompt
  });

  /* -------------------------------------------------------------------------
     Request builders — the single source of truth for every outbound call.
     Both the fetch handlers below and "Copy cURL" consume these, so the copied
     command can never drift away from what the playground actually sends.
     ------------------------------------------------------------------------- */
  const {
    buildChatRequest, buildEmbeddingRequest, buildHeaders, buildImageRequest, buildRerankRequest,
    buildTtsRequest, buildVideoRequest, getCurlUnsupportedReason, handleCopyCurl, handleStopGeneration,
    isAbortError, withSystemPrompt
  } = usePlaygroundRequests({
    abortRef, arenaModelB, chatInput, copyToClipboard, embedInput, embedModel, imgModel, imgPrompt, imgQuality,
    imgSize, isZh, maxTokens, messages, playModality, playMode, playModel, playStream, rerankDocs, rerankModel,
    rerankQuery, selectedKeyObj, setPlayStopped, systemPrompt, temperature, thinkingByModel,
    thinkingCapabilities, ttsInput, ttsModel, ttsSpeed, ttsVoice, videoModel, videoPrompt
  });

  // Run chat invocation
  const {
    handleRunChat
  } = usePlaygroundChat({
    abortRef, arenaModelB, buildChatRequest, buildHeaders, chatInput, isAbortError, messages, playMode,
    playModel, playStream, resolveCallCredentials, setArenaDurationB, setArenaOutputB, setArenaReasoningB,
    setArenaTTFTB, setArenaThinkingB, setArenaTokensPerSecB, setChatInput, setMessages, setPlayDurationMs,
    setPlayLoading, setPlayOutput, setPlayReasoningOutput, setPlayStopped, setPlayTTFTMs, setPlayThinking,
    setPlayTokensPerSec, showToast, withSystemPrompt
  });

  // Image generation
  const {
    handleEmbed, handleGenerateImage, handleGenerateTTS, handleGenerateVideo, handleRerank, handleTranscribe
  } = usePlaygroundMedia({
    abortRef, buildEmbeddingRequest, buildHeaders, buildImageRequest, buildRerankRequest, buildTtsRequest,
    buildVideoRequest, embedModel, imgModel, isAbortError, isZh, rerankModel, resolveCallCredentials,
    setEmbedResult, setImgResult, setPlayDurationMs, setPlayLoading, setPlayStopped, setRerankResult,
    setSttResult, setTtsAudioUrl, setVideoResult, setVideoTaskId, showToast, sttFile, sttLanguage, sttModel,
    ttsModel, videoModel
  });

  // Non-empty when the current modality cannot be reproduced as a cURL; used to
  // disable the copy button and explain why in its tooltip.
  const curlUnsupportedReason = getCurlUnsupportedReason();

  return (
    <div className="space-y-4 animate-in fade-in duration-200">
      {/* 1. Modality Selector Bar */}
      <PlaygroundModalityBar {...{
        isZh, playModality, playMode, setPlayModality, setPlayMode, setPlayStopped
      }} />

      {/* 2. Main Playground Grid Layout */}
      <div className="space-y-2 rounded-2xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-[#191B1F] p-4">
            <label className="text-xs font-semibold text-slate-700 dark:text-slate-300 flex items-center gap-1 mb-1" htmlFor="playground-key">
              <Key className="w-3.5 h-3.5 text-indigo-500" />
              {isZh ? 'API 访问密钥' : 'API Key'}
            </label>
            <select id="playground-key" value={selectedKeyId} onChange={e => setSelectedKeyId(e.target.value)}
              disabled={!contextKeys.length || playLoading} className="ui-field font-mono max-w-xl"
              aria-label={isZh ? '选择 API 访问密钥' : 'Select API key'}>
              {!contextKeys.length && <option value="">{isZh ? '暂无可用密钥' : 'No active keys'}</option>}
              {contextKeys.map(k => <option key={k.id} value={String(k.id)}>{k.tenant_id ? `${k.tenant_id} - ` : ''}{keyDisplayRef(k)}</option>)}
            </select>
            <p className="ui-meta">{selectedKeyId
              ? (isZh ? '权限、额度和计费组跟随所选密钥。' : 'Uses the selected key’s model access, quota and billing group.')
              : (isZh ? '创建访问密钥后即可开始调试。' : 'Create an API key to start testing.')}</p>
            {!selectedKeyId && onCreateKey && <button type="button" className="ui-btn-secondary" onClick={onCreateKey}>{isZh ? '创建访问密钥' : 'Create API key'}</button>}
          </div>

      {playModality === 'gemini' && <GeminiPlayground thinkingCapabilities={thinkingCapabilities} models={availableKeyModels(selectedKeyObj, models)} resolveCredentials={resolveCallCredentials} lang={lang} showToast={showToast} />}
      <div className={`grid grid-cols-1 lg:grid-cols-3 gap-6 ${playModality === 'gemini' ? 'hidden' : ''}`}>
        {/* Left Column: Parameter Controls */}
        <PlaygroundParameters {...{
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
        }} />

        {/* Right Columns: Interactive Chat View / Arena View */}
        <PlaygroundResults {...{
          activeChatModel, arenaDurationB, arenaModelB, arenaOutputB, arenaReasoningB, arenaTTFTB, arenaThinkingB,
          arenaTokensPerSecB, chatInput, chatInputRef, curlUnsupportedReason, embedResult, handleCopyCurl,
          handleOptimizeChatInput, handleRunChat, handleStopGeneration, imgResult, isZh, lang, messages, offPeak,
          playLoading, playModality, playMode, playModel, playOutput, playReasoningOutput, playStopped, playTTFTMs,
          playThinking, playTokensPerSec, rerankDocs, rerankResult, setArenaOutputB, setChatInput, setImgResult,
          setMessages, setPlayOutput, setPlayReasoningOutput, setPlayStopped, setTtsAudioUrl, showSystemPrompt,
          sttResult, systemPrompt, ttsAudioUrl, videoResult
        }} />
      </div>
    </div>
  );
}
