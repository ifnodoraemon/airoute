import { Component } from 'react';

// The console's last line of defence.
//
// Without a boundary, ANY render-time throw unmounts the whole React tree: React
// has nowhere to hand the error, so the page goes blank and only a refresh brings
// it back. Two realistic ways that happens here:
//
//   * a lazily imported view fails to load — a deploy replaced the hashed chunks
//     while an old shell was cached, or the network/CDN hiccuped, so the dynamic
//     `import()` rejects and React.lazy throws during render;
//   * a view receives a row shape it did not expect (several panels format fields
//     directly) and throws.
//
// One boundary near the top keeps a single bad module from taking the shell with
// it, and offers the two actions that actually help: retry the render, or reload
// the page (which is what fixes a stale chunk).

const CHUNK_ERROR_PATTERN =
  /dynamically imported module|Importing a module script failed|ChunkLoadError|Loading chunk [\w.-]+ failed|Failed to fetch/i;

const COPY = {
  zh: {
    title: '控制台加载失败',
    body: '这个模块在渲染时出错了。可以重试；如果仍然失败，请刷新页面。',
    chunk: '新版本可能已经发布，当前页面引用的前端模块已不存在。刷新页面即可拿到最新版本。',
    retry: '重试',
    reload: '刷新页面',
    detail: '错误摘要',
  },
  en: {
    title: 'The console failed to load',
    body: 'This module threw while rendering. You can retry, or reload the page if it keeps failing.',
    chunk: 'A new version was probably deployed and the front-end module this page references no longer exists. Reload to pick up the latest version.',
    retry: 'Retry',
    reload: 'Reload page',
    detail: 'Error summary',
  },
};

// The boundary can render above the i18n provider (main.jsx), so it cannot read the
// context; the stored preference is the next best source, with Chinese default.
function prefersEnglish() {
  try {
    return localStorage.getItem('airoute_lang') === 'en';
  } catch {
    return false;
  }
}

export default class ErrorBoundary extends Component {
  constructor(props) {
    super(props);
    this.state = { error: null };
    this.handleRetry = this.handleRetry.bind(this);
  }

  static getDerivedStateFromError(error) {
    return { error };
  }

  componentDidCatch(error, info) {
    // Keep the stack in the console for engineers (the UI only shows a summary).
    console.error('Console module crashed:', error, info?.componentStack);
    this.props.onError?.(error, info);
  }

  handleRetry() {
    this.setState({ error: null });
  }

  render() {
    const { error } = this.state;
    if (!error) return this.props.children;

    const isZh = (this.props.lang || (prefersEnglish() ? 'en' : 'zh')) === 'zh';
    const t = isZh ? COPY.zh : COPY.en;
    const message = String(error?.message || error || '');
    const isChunkError = CHUNK_ERROR_PATTERN.test(message);

    return (
      <div className="flex min-h-[240px] w-full items-center justify-center p-6">
        <div
          role="alert"
          className="w-full max-w-lg rounded-2xl border border-rose-200 bg-rose-50/80 p-6 text-rose-900 shadow-sm dark:border-rose-900/50 dark:bg-rose-950/30 dark:text-rose-100"
        >
          <h2 className="text-sm font-semibold">{t.title}</h2>
          <p className="mt-1.5 text-xs leading-relaxed opacity-90">{isChunkError ? t.chunk : t.body}</p>
          <div className="mt-4 flex items-center gap-2">
            <button type="button" onClick={this.handleRetry} className="ui-btn-primary ui-btn-sm">
              {t.retry}
            </button>
            <button
              type="button"
              onClick={() => { if (typeof window !== 'undefined') window.location.reload(); }}
              className="ui-btn-secondary ui-btn-sm"
            >
              {t.reload}
            </button>
          </div>
          {message && (
            <details className="mt-4">
              <summary className="cursor-pointer text-[11px] font-medium opacity-80">{t.detail}</summary>
              <pre className="mt-1.5 max-h-40 overflow-auto whitespace-pre-wrap break-all rounded-lg bg-black/5 p-2 text-[11px] dark:bg-black/30">
                {message}
              </pre>
            </details>
          )}
        </div>
      </div>
    );
  }
}
