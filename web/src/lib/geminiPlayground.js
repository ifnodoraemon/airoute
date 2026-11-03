export function createGeminiOutput() {
  return { text: '', reasoning: '', media: [], tools: [], usage: null, rawParts: [], feedback: null, finishReason: '', safetyRatings: [] };
}

export function applyGeminiOutput(state, frame) {
  if (frame.error) throw new Error(frame.error.message || 'Gemini request failed');
  if (frame.usageMetadata) state.usage = frame.usageMetadata;
  if (frame.promptFeedback) state.feedback = frame.promptFeedback;
  for (const candidate of frame.candidates || []) {
    if ((candidate.index || 0) !== 0) continue;
    if (candidate.finishReason) state.finishReason = candidate.finishReason;
    if (candidate.safetyRatings) state.safetyRatings = candidate.safetyRatings;
    for (const part of candidate.content?.parts || []) {
      state.rawParts.push(part);
      if (part.text) state[part.thought ? 'reasoning' : 'text'] += part.text;
      if (part.functionCall) state.tools.push(part.functionCall);
      const media = part.inlineData || part.inline_data;
      const file = part.fileData || part.file_data;
      if (media) {
        const mime = media.mimeType || media.mime_type || 'application/octet-stream';
        const previous = state.media.at(-1);
        if (/^audio\/(pcm|l16)/i.test(mime) && previous?.mime === mime && !previous.uri) {
          previous.chunks = [...(previous.chunks || [previous.data]), media.data || ''];
        } else state.media.push({ mime, data: media.data || '' });
      }
      if (file) state.media.push({ mime: file.mimeType || file.mime_type, uri: file.fileUri || file.file_uri });
    }
  }
  return { ...state, media: state.media.map(item => ({ ...item })), tools: [...state.tools] };
}

export function geminiOutputNotice(state, isZh) {
  const blocked = state.feedback?.blockReason;
  if (blocked && blocked !== 'BLOCK_REASON_UNSPECIFIED') return isZh
    ? `请求被拦截（${blocked}），请调整输入后重试。`
    : `Request blocked (${blocked}). Revise your input and retry.`;
  if (state.finishReason === 'MAX_TOKENS') return isZh
    ? '已达到输出长度上限（MAX_TOKENS），可增加输出上限后重试。'
    : 'Output limit reached (MAX_TOKENS). Increase the output limit to retry.';
  if (state.finishReason && !['STOP', 'FINISH_REASON_UNSPECIFIED'].includes(state.finishReason)) return isZh
    ? `生成已结束（${state.finishReason}），请检查请求和安全评估。`
    : `Generation ended (${state.finishReason}). Check your request and safety ratings.`;
  if (!state.text && !state.reasoning && !state.media.length && !state.tools.length) return isZh
    ? '请求已结束，未返回内容。请调整输入或检查模型配置后重试。'
    : 'Request completed without content. Revise your input or check the model configuration.';
  return '';
}

export async function readGeminiSSE(response, onFrame) {
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  const consume = (event) => {
    const data = event.split(/\r?\n/).filter(line => line.startsWith('data:')).map(line => line.slice(5).trimStart()).join('\n');
    if (data && data !== '[DONE]') onFrame(JSON.parse(data));
  };
  try {
    while (true) {
      const { value, done } = await reader.read();
      if (value) buffer += decoder.decode(value, { stream: true });
      if (done) buffer += decoder.decode();
      const events = buffer.split(/\r?\n\r?\n/);
      buffer = events.pop() || '';
      events.forEach(consume);
      if (done) break;
    }
    if (buffer.trim()) consume(buffer);
  } finally {
    await reader.cancel?.();
    reader.releaseLock?.();
  }
}

export function geminiMediaBlob(item) {
  const chunks = (item.chunks || [item.data || '']).map(data => Uint8Array.from(atob(data), char => char.charCodeAt(0)));
  const length = chunks.reduce((total, chunk) => total + chunk.length, 0);
  const bytes = new Uint8Array(length);
  let position = 0; chunks.forEach(chunk => { bytes.set(chunk, position); position += chunk.length; });
  if (!/^audio\/(pcm|l16)/i.test(item.mime)) return new Blob([bytes], { type: item.mime });
  const rate = Number(item.mime.match(/rate=(\d+)/i)?.[1] || 24000);
  const header = new ArrayBuffer(44);
  const view = new DataView(header);
  const text = (offset, value) => [...value].forEach((char, index) => view.setUint8(offset + index, char.charCodeAt(0)));
  text(0, 'RIFF'); view.setUint32(4, bytes.length + 36, true); text(8, 'WAVE'); text(12, 'fmt ');
  view.setUint32(16, 16, true); view.setUint16(20, 1, true); view.setUint16(22, 1, true);
  view.setUint32(24, rate, true); view.setUint32(28, rate * 2, true); view.setUint16(32, 2, true); view.setUint16(34, 16, true);
  text(36, 'data'); view.setUint32(40, bytes.length, true);
  return new Blob([header, bytes], { type: 'audio/wav' });
}

export function safeMediaURI(uri) {
  try { const parsed = new URL(uri); return ['http:', 'https:', 'blob:'].includes(parsed.protocol) ? uri : ''; } catch { return ''; }
}

// Private Google media must use the gateway's credential-bound resource route.
// Never attach the caller's bearer token to a URL supplied by an upstream.
export function geminiDownloadPath(uri) {
  try {
    const parsed = new URL(uri, window.location.origin);
    if (parsed.origin !== window.location.origin && parsed.hostname !== 'generativelanguage.googleapis.com') return '';
    const match = parsed.pathname.match(/^\/(?:download\/)?v1beta\/(files|generatedFiles)\/([\w.-]+)(?::download)?$/);
    if (!match) return '';
    return `/v1beta/${match[1]}/${match[2]}:download?alt=media`;
  } catch { return ''; }
}
