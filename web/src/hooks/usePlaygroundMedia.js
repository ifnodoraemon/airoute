import { consoleFetch } from '../api/console';

export default function usePlaygroundMedia({
  abortRef, buildEmbeddingRequest, buildHeaders, buildImageRequest, buildRerankRequest, buildTtsRequest,
  buildVideoRequest, embedModel, imgModel, isAbortError, isZh, rerankModel, resolveCallCredentials,
  setEmbedResult, setImgResult, setPlayDurationMs, setPlayLoading, setPlayStopped, setRerankResult,
  setSttResult, setTtsAudioUrl, setVideoResult, setVideoTaskId, showToast, sttFile, sttLanguage, sttModel,
  ttsModel, videoModel
}) {
  const handleGenerateImage = async () => {
    const credentials = resolveCallCredentials('images', [imgModel]);
    if (!credentials) return;
    setPlayLoading(true);
    setPlayStopped(false);
    setImgResult(null);
    const controller = new AbortController();
    abortRef.current = controller;
    const start = Date.now();
    try {
      const request = buildImageRequest();
      const res = await consoleFetch(request.path, credentials.keyId, {
        method: request.method,
        headers: buildHeaders(),
        body: JSON.stringify(request.body),
        signal: controller.signal
      });
      const data = await res.json();
      setPlayDurationMs(Date.now() - start);
      if (res.ok && data.data && data.data[0]) {
        setImgResult(data.data[0]);
      } else {
        if (showToast) showToast(isZh ? `绘图失败: ${data.error?.message || '未知错误'}` : `Image generation failed: ${data.error?.message || 'Unknown error'}`, 'error');
      }
    } catch (e) {
      if (!isAbortError(e) && showToast) showToast(e.message, 'error');
    } finally {
      setPlayLoading(false);
      abortRef.current = null;
    }
  };

  // Audio Speech (TTS)
  const handleGenerateTTS = async () => {
    const credentials = resolveCallCredentials('audio_speech', [ttsModel]);
    if (!credentials) return;
    setPlayLoading(true);
    setPlayStopped(false);
    setTtsAudioUrl(null);
    const controller = new AbortController();
    abortRef.current = controller;
    const start = Date.now();
    try {
      const request = buildTtsRequest();
      const res = await consoleFetch(request.path, credentials.keyId, {
        method: request.method,
        headers: buildHeaders(),
        body: JSON.stringify(request.body),
        signal: controller.signal
      });
      setPlayDurationMs(Date.now() - start);
      if (res.ok) {
        const blob = await res.blob();
        const url = URL.createObjectURL(blob);
        // Release the previous audio object URL to avoid leaking memory on
        // every synthesis, and remember the current one for unmount cleanup.
        setTtsAudioUrl((prev) => {
          if (prev) URL.revokeObjectURL(prev);
          return url;
        });
      } else {
        const err = await res.json().catch(() => ({}));
        if (showToast) showToast(isZh ? `TTS 失败: ${err.error?.message || '未知错误'}` : `TTS failed: ${err.error?.message || 'Unknown error'}`, 'error');
      }
    } catch (e) {
      if (!isAbortError(e) && showToast) showToast(e.message, 'error');
    } finally {
      setPlayLoading(false);
      abortRef.current = null;
    }
  };

  // Audio Transcription (STT) — multipart upload
  const handleTranscribe = async () => {
    if (!sttFile) {
      if (showToast) showToast(isZh ? '请先选择音频文件' : 'Please choose an audio file first', 'warning');
      return;
    }
    const credentials = resolveCallCredentials('audio_transcription', [sttModel]);
    if (!credentials) return;
    setPlayLoading(true);
    setPlayStopped(false);
    setSttResult(null);
    const controller = new AbortController();
    abortRef.current = controller;
    const start = Date.now();
    try {
      const form = new FormData();
      form.append('file', sttFile);
      form.append('model', sttModel);
      if (sttLanguage.trim()) form.append('language', sttLanguage.trim());
      const res = await consoleFetch('/v1/audio/transcriptions', credentials.keyId, {
        method: 'POST',
        // No Content-Type header: the browser must set the multipart boundary.
        headers: {},
        body: form,
        signal: controller.signal
      });
      const data = await res.json().catch(() => ({}));
      setPlayDurationMs(Date.now() - start);
      if (res.ok) {
        setSttResult(data.text || '');
      } else if (showToast) {
        showToast(isZh ? `转写失败: ${data.error?.message || res.statusText}` : `Transcription failed: ${data.error?.message || res.statusText}`, 'error');
      }
    } catch (e) {
      if (!isAbortError(e) && showToast) showToast(e.message, 'error');
    } finally {
      setPlayLoading(false);
      abortRef.current = null;
    }
  };

  // Video generation. The gateway returns either a finished asset or an async
  // task id that must be polled through /v1/videos/tasks/{id}.
  const handleGenerateVideo = async () => {
    const credentials = resolveCallCredentials('videos', [videoModel]);
    if (!credentials) return;
    setPlayLoading(true);
    setPlayStopped(false);
    setVideoResult(null);
    setVideoTaskId('');
    const controller = new AbortController();
    abortRef.current = controller;
    const start = Date.now();
    try {
      const request = buildVideoRequest();
      const res = await consoleFetch(request.path, credentials.keyId, {
        method: request.method,
        headers: buildHeaders(),
        body: JSON.stringify(request.body),
        signal: controller.signal
      });
      const data = await res.json().catch(() => ({}));
      setPlayDurationMs(Date.now() - start);
      if (!res.ok) {
        if (showToast) showToast(isZh ? `视频生成失败: ${data.error?.message || res.statusText}` : `Video generation failed: ${data.error?.message || res.statusText}`, 'error');
        return;
      }
      const taskId = data.id || data.task_id || data.data?.id || data.data?.task_id;
      if (taskId) {
        setVideoTaskId(taskId);
        await pollVideoTask(taskId, request.body.model, controller.signal, credentials.keyId);
      } else {
        setVideoResult(data);
      }
    } catch (e) {
      if (!isAbortError(e) && showToast) showToast(e.message, 'error');
    } finally {
      setPlayLoading(false);
      abortRef.current = null;
    }
  };

  const pollVideoTask = async (taskId, taskModel, signal, callKeyId) => {
    for (let attempt = 0; attempt < 40; attempt += 1) {
      await new Promise((resolve, reject) => {
        const abort = () => { clearTimeout(timer); reject(new DOMException('Stopped', 'AbortError')); };
        const timer = setTimeout(() => { signal.removeEventListener('abort', abort); resolve(); }, 3000);
        if (signal.aborted) { abort(); return; }
        signal.addEventListener('abort', abort, { once: true });
      });
      signal.throwIfAborted();
      const res = await consoleFetch(`/v1/videos/tasks/${encodeURIComponent(taskId)}?model=${encodeURIComponent(taskModel)}`, callKeyId, {
        headers: buildHeaders(), signal,
      });
      const data = await res.json();
      signal.throwIfAborted();
      if (!res.ok) throw new Error(data.error?.message || `Video task query failed (HTTP ${res.status})`);
      const status = String(data.status || data.data?.status || '').toLowerCase();
      setVideoResult(data.data || data);
      if (['succeeded', 'completed', 'success'].includes(status)) return;
      if (['failed', 'error', 'cancelled', 'canceled'].includes(status)) throw new Error(data.error?.message || (isZh ? '视频任务失败或已取消' : 'Video task failed or was cancelled'));
    }
    showToast?.(isZh ? '视频任务仍在处理中，请稍后查询' : 'Video task still running; check again later', 'warning');
  };

  // Embeddings
  const handleEmbed = async () => {
    const credentials = resolveCallCredentials('embeddings', [embedModel]);
    if (!credentials) return;
    setPlayLoading(true);
    setPlayStopped(false);
    setEmbedResult(null);
    const controller = new AbortController();
    abortRef.current = controller;
    const start = Date.now();
    try {
      const request = buildEmbeddingRequest();
      const res = await consoleFetch(request.path, credentials.keyId, {
        method: request.method,
        headers: buildHeaders(),
        body: JSON.stringify(request.body),
        signal: controller.signal
      });
      const data = await res.json().catch(() => ({}));
      setPlayDurationMs(Date.now() - start);
      if (res.ok) setEmbedResult(data);
      else if (showToast) showToast(isZh ? `向量化失败: ${data.error?.message || res.statusText}` : `Embedding failed: ${data.error?.message || res.statusText}`, 'error');
    } catch (e) {
      if (!isAbortError(e) && showToast) showToast(e.message, 'error');
    } finally {
      setPlayLoading(false);
      abortRef.current = null;
    }
  };

  // Rerank
  const handleRerank = async () => {
    const credentials = resolveCallCredentials('rerank', [rerankModel]);
    if (!credentials) return;
    setPlayLoading(true);
    setPlayStopped(false);
    setRerankResult(null);
    const controller = new AbortController();
    abortRef.current = controller;
    const start = Date.now();
    try {
      const request = buildRerankRequest();
      const res = await consoleFetch(request.path, credentials.keyId, {
        method: request.method,
        headers: buildHeaders(),
        body: JSON.stringify(request.body),
        signal: controller.signal
      });
      const data = await res.json().catch(() => ({}));
      setPlayDurationMs(Date.now() - start);
      if (res.ok) setRerankResult(data);
      else if (showToast) showToast(isZh ? `重排失败: ${data.error?.message || res.statusText}` : `Rerank failed: ${data.error?.message || res.statusText}`, 'error');
    } catch (e) {
      if (!isAbortError(e) && showToast) showToast(e.message, 'error');
    } finally {
      setPlayLoading(false);
      abortRef.current = null;
    }
  };
  return {
    handleEmbed, handleGenerateImage, handleGenerateTTS, handleGenerateVideo, handleRerank, handleTranscribe
  };
}
