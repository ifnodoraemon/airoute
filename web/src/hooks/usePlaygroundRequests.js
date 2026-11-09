import { thinkingRequest, assistantHistory } from '../lib/thinking';
import { getGatewayOrigin } from '../config';
import { API_KEY_PLACEHOLDER } from '../lib/apiKey';

export default function usePlaygroundRequests({
  abortRef, arenaModelB, chatInput, copyToClipboard, embedInput, embedModel, imgModel, imgPrompt, imgQuality,
  imgSize, isZh, maxTokens, messages, playModality, playMode, playModel, playStream, rerankDocs, rerankModel,
  rerankQuery, selectedKeyObj, setPlayStopped, systemPrompt, temperature, thinkingByModel,
  thinkingCapabilities, ttsInput, ttsModel, ttsSpeed, ttsVoice, videoModel, videoPrompt
}) {
  const buildHeaders = () => ({ 'Content-Type': 'application/json' });

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

  const withSystemPrompt = (msgs, targetModel=playModel) => [
    ...(systemPrompt.trim() ? [{ role: 'system', content: systemPrompt.trim() }] : []),
    ...msgs.map(message => assistantHistory(message,false,targetModel))
  ];

  const buildChatRequest = (targetModel, payloadMessages) => {
    const body = {
      model: targetModel,
      ...thinkingRequest(thinkingCapabilities[targetModel],thinkingByModel[targetModel]),
      ...(playStream ? {stream_options:{include_usage:true}} : {}),
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
    const lines = [
      `curl -X ${request.method} "${getGatewayOrigin()}${request.path}"`,
      '  -H "Content-Type: application/json"',
    ];
    // Export the public SDK request with a placeholder; a console login cookie
    // and the selected key identifier never belong in a shareable command.
    lines.push(`  -H "Authorization: Bearer ${API_KEY_PLACEHOLDER}"`);
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
      const history=pending ? [...messages, {role:'user',content:pending}] : messages;
      const targets = playMode === 'arena' && arenaModelB && arenaModelB !== playModel
        ? [playModel, arenaModelB]
        : [playModel];
      calls = targets.map(m => ({ request: buildChatRequest(m, withSystemPrompt(history,m)), flags: playStream ? '-N' : '' }));
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
  return {
    buildChatRequest, buildEmbeddingRequest, buildHeaders, buildImageRequest, buildRerankRequest,
    buildTtsRequest, buildVideoRequest, getCurlUnsupportedReason, handleCopyCurl, handleStopGeneration,
    isAbortError, withSystemPrompt
  };
}
