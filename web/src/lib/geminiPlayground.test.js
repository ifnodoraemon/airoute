import { describe, expect, it } from 'vitest';
import { applyGeminiOutput, createGeminiOutput, geminiDownloadPath, geminiMediaBlob, readGeminiSSE, safeMediaURI } from './geminiPlayground';

describe('Gemini native output', () => {
  it('retains signed empty parts and separates text, thought and media', () => {
    const state = createGeminiOutput();
    const parts = [{ text: 'reason', thought: true }, { text: 'answer' }, { text: '', thoughtSignature: 'signature==' }, { inlineData: { mimeType: 'image/png', data: 'AA==' } }, { fileData: { mimeType: 'video/mp4', fileUri: 'https://example.invalid/video' } }];
    const result = applyGeminiOutput(state, { candidates: [{ content: { parts } }], usageMetadata: { thoughtsTokenCount: 2 } });
    expect(result.text).toBe('answer'); expect(result.reasoning).toBe('reason');
    expect(result.rawParts).toEqual(parts); expect(result.media).toHaveLength(2);
    expect(result.usage.thoughtsTokenCount).toBe(2);
  });

  it('joins separately padded PCM chunks and wraps them in a playable WAV', async () => {
    const state = createGeminiOutput();
    for (const data of ['AAA=', 'AQA=']) applyGeminiOutput(state, { candidates: [{ content: { parts: [{ inlineData: { mimeType: 'audio/L16;rate=16000', data } }] } }] });
    const blob = geminiMediaBlob(state.media[0]);
    expect(blob.type).toBe('audio/wav'); expect(blob.size).toBe(48);
    const bytes = await new Promise(resolve => { const reader = new FileReader(); reader.onload = () => resolve(new Uint8Array(reader.result)); reader.readAsArrayBuffer(blob); });
    expect(String.fromCharCode(...bytes.slice(0, 4))).toBe('RIFF');
    expect(new DataView(bytes.buffer).getUint32(24, true)).toBe(16000);
    expect([...bytes.slice(44)]).toEqual([0, 0, 1, 0]);
  });

  it('parses multiline SSE with split UTF-8 and a final event without a separator', async () => {
    const wire = 'data: {\ndata: "candidates":[{"content":{"parts":[{"text":"你好"}]}}]\ndata: }';
    const encoded = new TextEncoder().encode(wire);
    const body = new ReadableStream({ start(controller) { for (const byte of encoded) controller.enqueue(new Uint8Array([byte])); controller.close(); } });
    const frames = []; await readGeminiSSE({ body }, frame => frames.push(frame));
    expect(frames[0].candidates[0].content.parts[0].text).toBe('你好');
  });

  it('rejects executable media URLs', () => {
    expect(safeMediaURI('javascript:alert(1)')).toBe(''); expect(safeMediaURI('https://example.invalid/video')).toContain('https://');
    expect(geminiDownloadPath('https://generativelanguage.googleapis.com/v1beta/files/owned:download?key=secret')).toBe('/v1beta/files/owned:download?alt=media');
    expect(geminiDownloadPath('https://untrusted.invalid/v1beta/files/owned')).toBe('');
  });
});
