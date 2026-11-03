import { useEffect, useRef, useState } from 'react';
import { applyGeminiOutput, createGeminiOutput, geminiOutputNotice, readGeminiSSE } from '../lib/geminiPlayground';
import { assistantContent } from '../lib/assistantContent';
import MarkdownRenderer from './MarkdownRenderer';
import GeminiMediaOutput from './GeminiMediaOutput';
import { consoleFetch } from '../api/console';
import { extractErrorMessage } from '../api/client';

const example = JSON.stringify({ contents: [{ role: 'user', parts: [{ text: '你好，请介绍一下你自己。' }] }], generationConfig: {} }, null, 2);

export default function GeminiPlayground({ models = [], resolveCredentials, lang = 'zh', showToast }) {
  const isZh = lang === 'zh';
  const [model, setModel] = useState(models[0] || '');
  const [body, setBody] = useState(example);
  const [stream, setStream] = useState(true);
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState(createGeminiOutput);
  const [error, setError] = useState('');
  const [outcome, setOutcome] = useState('idle');
  const [mediaKeyId, setMediaKeyId] = useState('');
  const controller = useRef(null);
  useEffect(() => () => controller.current?.abort(), []);
  const activeModel = models.includes(model) ? model : models[0] || '';
  const display = assistantContent(result.text, result.reasoning, !loading);
  const notice = outcome === 'complete' ? geminiOutputNotice(result, isZh) : '';

  const attach = async (file) => {
    if (!file) return;
    try {
      const data = await new Promise((resolve, reject) => {
        const reader = new FileReader(); reader.onload = () => resolve(String(reader.result).split(',')[1]); reader.onerror = reject; reader.readAsDataURL(file);
      });
      const request = JSON.parse(body);
      if (!request.contents?.length) request.contents = [{ role: 'user', parts: [] }];
      const last = request.contents.at(-1);
      if (!last.parts) last.parts = [];
      last.parts.push({ inlineData: { mimeType: file.type || 'application/octet-stream', data } });
      setBody(JSON.stringify(request, null, 2));
    } catch (failure) { setError(failure.message); }
  };

  const run = async () => {
    const credentials = resolveCredentials();
    if (!credentials || !activeModel || loading) return;
    let request;
    try { request = JSON.parse(body); } catch { setError(isZh ? '请求 JSON 格式不正确' : 'Invalid request JSON'); return; }
    const abort = new AbortController(); controller.current = abort;
    setLoading(true); setError(''); setOutcome('idle'); setResult(createGeminiOutput()); setMediaKeyId(credentials.keyId);
    const output = createGeminiOutput();
    try {
      const action = stream ? 'streamGenerateContent?alt=sse' : 'generateContent';
      const response = await consoleFetch(`/v1beta/models/${encodeURIComponent(activeModel)}:${action}`, credentials.keyId, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(request), signal: abort.signal,
      });
      if (!response.ok) { const failure = await response.json(); throw new Error(extractErrorMessage(failure, response.statusText)); }
      const accept = frame => setResult(applyGeminiOutput(output, frame));
      if (stream && response.body) await readGeminiSSE(response, accept); else accept(await response.json());
    } catch (failure) {
      if (failure.name !== 'AbortError') { setError(failure.message); showToast?.(failure.message, 'error'); }
    } finally { if (controller.current === abort) { controller.current = null; setLoading(false); setOutcome(abort.signal.aborted ? 'stopped' : 'complete'); } }
  };

  const appendHistory = () => {
    try {
      const request = JSON.parse(body);
      request.contents = [...(request.contents || []), { role: 'model', parts: result.rawParts }, { role: 'user', parts: [{ text: '' }] }];
      setBody(JSON.stringify(request, null, 2));
    } catch { setError(isZh ? '请求 JSON 格式不正确' : 'Invalid request JSON'); }
  };

  return (
    <section className="grid lg:grid-cols-2 gap-4" aria-label={isZh ? 'Gemini 原生调试' : 'Gemini native playground'}>
      <div className="space-y-3 rounded-2xl border border-slate-200 dark:border-slate-700 p-5">
        <label className="block text-xs font-semibold">{isZh ? '模型' : 'Model'}
          <select aria-label={isZh ? 'Gemini 模型' : 'Gemini model'} value={activeModel} onChange={event => setModel(event.target.value)} className="ui-field mt-1">{models.map(name => <option key={name}>{name}</option>)}</select>
        </label>
        <label className="block text-xs font-semibold">{isZh ? '原生请求 JSON' : 'Native request JSON'}
          <textarea aria-label={isZh ? 'Gemini 请求 JSON' : 'Gemini request JSON'} value={body} onChange={event => setBody(event.target.value)} rows={16} className="ui-field font-mono mt-1" />
        </label>
        <label className="block text-xs">{isZh ? '附加图片、音频、视频或文档' : 'Attach image, audio, video or document'}
          <input type="file" accept="image/*,audio/*,video/*,application/pdf" onChange={event => attach(event.target.files?.[0])} className="block mt-2 text-xs" />
        </label>
        <label className="flex gap-2 text-xs"><input type="checkbox" checked={stream} onChange={event => setStream(event.target.checked)} />{isZh ? '流式响应' : 'Streaming'}</label>
        <div className="flex gap-2">
          <button className="ui-btn-primary" onClick={run} disabled={loading || !activeModel}>{isZh ? '发送原生请求' : 'Send native request'}</button>
          {loading && <button className="ui-btn-secondary" onClick={() => controller.current?.abort()}>{isZh ? '停止' : 'Stop'}</button>}
        </div>
        {error && <p role="alert" className="text-sm text-rose-600">{error}</p>}
      </div>
      <div className="space-y-4 rounded-2xl border border-slate-200 dark:border-slate-700 p-5 overflow-auto">
        {notice && !error && <p role="status" className="text-sm text-amber-700 dark:text-amber-300">{notice}</p>}
        {outcome === 'stopped' && <p role="status" className="text-sm text-slate-500">{isZh ? '生成已停止，已保留收到的内容。' : 'Generation stopped. Received content is preserved.'}</p>}
        {(result.feedback?.safetyRatings || result.safetyRatings.length > 0) && <details><summary>{isZh ? '安全评估' : 'Safety ratings'}</summary><pre className="text-xs whitespace-pre-wrap">{JSON.stringify(result.feedback?.safetyRatings || result.safetyRatings, null, 2)}</pre></details>}
        {display.reasoning && <details open><summary>{isZh ? '思考内容' : 'Reasoning'}</summary><pre className="whitespace-pre-wrap text-xs">{display.reasoning}</pre></details>}
        {display.content && <MarkdownRenderer content={display.content} lang={lang} />}
        {result.media.map((item, index) => <GeminiMediaOutput key={index} item={item} index={index} lang={lang} keyId={mediaKeyId} />)}
        {result.tools.length > 0 && <pre className="text-xs whitespace-pre-wrap">{JSON.stringify(result.tools, null, 2)}</pre>}
        {result.usage && <pre className="text-xs whitespace-pre-wrap text-slate-500">{JSON.stringify(result.usage, null, 2)}</pre>}
        {!loading && outcome === 'complete' && !notice && !error && result.rawParts.length > 0 && <button className="ui-btn-secondary" onClick={appendHistory}>{isZh ? '将完整回复加入下一轮' : 'Append complete response to history'}</button>}
      </div>
    </section>
  );
}
