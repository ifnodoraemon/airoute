import { useEffect, useState } from 'react';
import { geminiDownloadPath, geminiMediaBlob, safeMediaURI } from '../lib/geminiPlayground';

export default function GeminiMediaOutput({ item, index, lang = 'zh', apiKey = '' }) {
  const { data, mime, uri, chunks } = item;
  const [source, setSource] = useState('');
  const [error, setError] = useState('');
  useEffect(() => {
    const abort = new AbortController();
    let objectURL = '';
    setError('');
    const load = async () => {
      try {
        const path = uri && geminiDownloadPath(uri);
        if (uri && !path) { setSource(safeMediaURI(uri)); return; }
        let blob;
        if (path) {
          const response = await fetch(path, { headers: apiKey ? { Authorization: `Bearer ${apiKey}` } : {}, signal: abort.signal });
          if (!response.ok) throw new Error(`Media download: HTTP ${response.status}`);
          blob = await response.blob();
        } else blob = geminiMediaBlob({ data, mime, chunks });
        if (!abort.signal.aborted) { objectURL = URL.createObjectURL(blob); setSource(objectURL); }
      } catch (failure) { if (!abort.signal.aborted) setError(failure.message); }
    };
    load();
    return () => { abort.abort(); if (objectURL) URL.revokeObjectURL(objectURL); };
  }, [data, mime, uri, chunks, apiKey]);
  const label = `${lang === 'zh' ? '生成内容' : 'Generated media'} ${index + 1}`;
  if (!source) return error || uri ? <p className="text-xs break-all">{error || uri}</p> : null;
  return (
    <figure className="space-y-2 rounded-xl border border-slate-200 dark:border-slate-700 p-3">
      {item.mime?.startsWith('image/') && <img src={source} alt={label} className="max-h-96 rounded-lg object-contain" />}
      {item.mime?.startsWith('audio/') && <audio controls src={source} aria-label={label} className="w-full" />}
      {item.mime?.startsWith('video/') && <video controls src={source} aria-label={label} className="max-h-96 rounded-lg" />}
      <a className="text-xs text-indigo-600 underline" href={source} download={`gemini-output-${index + 1}`} target="_blank" rel="noreferrer">{lang === 'zh' ? '下载' : 'Download'} · {item.mime}</a>
    </figure>
  );
}
