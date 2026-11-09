// Chat Completions uses one JSON data line per frame. Gemini's multi-line
// event framing remains in geminiPlayground.js.
export async function readSSELines(response, onLine, {
  onBytes, onComplete, ignoreReleaseErrors = false,
} = {}) {
  const reader = response.body.getReader();
  const decoder = new TextDecoder('utf-8');
  let buffer = '';
  try {
    while (true) {
      const { value, done } = await reader.read();
      if (value) {
        onBytes?.(value);
        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() || '';
        for (const line of lines) onLine(line);
      }
      if (done) break;
    }
    buffer += decoder.decode();
    if (buffer.trim()) onLine(buffer);
    onComplete?.();
  } finally {
    await releaseReader(reader, ignoreReleaseErrors);
  }
}

async function releaseReader(reader, ignoreReleaseErrors) {
  try { await reader.cancel?.(); } catch { /* The stream may already be closed. */ }
  try { reader.releaseLock?.(); } catch (error) {
    if (!ignoreReleaseErrors) throw error;
  }
}
