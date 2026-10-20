import React, { useState, useMemo } from 'react';
import { marked } from 'marked';
import {
  Copy,
  Check,
  Info,
  Zap,
  AlertTriangle,
  ShieldAlert,
  ExternalLink,
  Square,
  CheckSquare,
  FileCode,
  Terminal,
  Code
} from 'lucide-react';
import { copyToClipboard as crossOriginCopy } from '../config';

function safeMarkdownUrl(value, image = false) {
  const url = (value || '').trim();
  if (image && /^data:image\/(png|jpeg|gif|webp);base64,[a-z0-9+/=]+$/i.test(url)) return url;
  if (url.startsWith('#')) return image ? null : url;
  try {
    const parsed = new URL(url, window.location.origin);
    if (parsed.protocol === 'http:' || parsed.protocol === 'https:' || (!image && parsed.protocol === 'mailto:')) return url;
  } catch {
    // Invalid URLs are rendered as text.
  }
  return null;
}

function highlightMatches(code, regex, classify) {
  const nodes = [];
  let offset = 0;
  for (const match of code.matchAll(regex)) {
    if (match.index > offset) nodes.push(code.slice(offset, match.index));
    nodes.push(<span key={match.index} className={classify(match[0])}>{match[0]}</span>);
    offset = match.index + match[0].length;
  }
  if (offset < code.length) nodes.push(code.slice(offset));
  return nodes;
}

// 1. Syntax colorizer helper for code blocks. React escapes every code fragment.
function renderHighlightedCode(code = '', lang = '') {
  const normalizedLang = (lang || '').toLowerCase().trim();

  // JSON syntax highlighting
  if (normalizedLang === 'json') {
    return <span>{highlightMatches(
      code,
      /("(\\u[a-zA-Z0-9]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(true|false|null)\b|-?\d+(?:\.\d*)?(?:[eE][+-]?\d+)?)/g,
      (match) => {
        let cls = 'text-amber-400'; // number
        if (/^"/.test(match)) {
          if (/:$/.test(match)) {
            cls = 'text-sky-400 font-semibold'; // key
          } else {
            cls = 'text-emerald-400'; // string
          }
        } else if (/true|false/.test(match)) {
          cls = 'text-purple-400 font-bold'; // boolean
        } else if (/null/.test(match)) {
          cls = 'text-rose-400 font-bold'; // null
        }
        return cls;
      }
    )}</span>;
  }

  // Shell / Bash / cURL syntax highlighting
  if (['bash', 'sh', 'shell', 'zsh', 'curl'].includes(normalizedLang)) {
    const lines = code.split('\n');
    return <span>{lines.map((line, index) => {
      const trimmed = line.trim();
      if (trimmed.startsWith('#')) {
        return <React.Fragment key={index}><span className="text-slate-500 italic">{line}</span>{index < lines.length - 1 && '\n'}</React.Fragment>;
      }
      return <React.Fragment key={index}>{highlightMatches(line, /(\$[A-Z0-9_]+|\${[A-Z0-9_]+}|--?[a-zA-Z0-9_-]+)/g, match => match.startsWith('-') ? 'text-sky-300 font-medium' : 'text-amber-400 font-semibold')}{index < lines.length - 1 && '\n'}</React.Fragment>;
    })}</span>;
  }

  // Python syntax highlighting
  if (['python', 'py'].includes(normalizedLang)) {
    const keywords = /\b(def|class|import|from|as|return|if|elif|else|for|while|try|except|finally|with|in|is|not|and|or|lambda|yield|True|False|None|async|await)\b/g;
    const lines = code.split('\n');
    return <span>{lines.map((line, index) => {
      const trimmed = line.trim();
      if (trimmed.startsWith('#')) {
        return <React.Fragment key={index}><span className="text-slate-500 italic">{line}</span>{index < lines.length - 1 && '\n'}</React.Fragment>;
      }
      return <React.Fragment key={index}>{highlightMatches(line, keywords, () => 'text-purple-400 font-bold')}{index < lines.length - 1 && '\n'}</React.Fragment>;
    })}</span>;
  }

  // Default plaintext code
  return <span>{code}</span>;
}

// 2. Inline Markdown Token Renderer
export function RenderInline({ tokens = [] }) {
  if (!tokens || tokens.length === 0) return null;

  return tokens.map((token, idx) => {
    switch (token.type) {
      case 'text':
        return <span key={idx}>{token.text}</span>;

      case 'strong':
        return (
          <strong key={idx} className="font-bold text-slate-900 dark:text-white">
            <RenderInline tokens={token.tokens} />
          </strong>
        );

      case 'em':
        return (
          <em key={idx} className="italic text-slate-800 dark:text-slate-200">
            <RenderInline tokens={token.tokens} />
          </em>
        );

      case 'codespan':
        return (
          <code
            key={idx}
            className="px-1.5 py-0.5 mx-0.5 rounded-md font-mono text-[11px] font-semibold bg-indigo-50 dark:bg-indigo-950/60 text-indigo-700 dark:text-indigo-300 border border-indigo-200/70 dark:border-indigo-800/70"
          >
            {token.text}
          </code>
        );

      case 'link': {
        const href = safeMarkdownUrl(token.href);
        if (!href) return <span key={idx}><RenderInline tokens={token.tokens} /></span>;
        const isHash = href.startsWith('#');
        return (
          <a
            key={idx}
            href={href}
            target={isHash ? '_self' : '_blank'}
            rel={isHash ? '' : 'noopener noreferrer'}
            className="inline-flex items-center gap-0.5 text-indigo-600 dark:text-indigo-400 hover:text-indigo-500 font-medium underline underline-offset-2 decoration-indigo-400/50 hover:decoration-indigo-500 transition-colors"
          >
            <RenderInline tokens={token.tokens} />
            {!isHash && <ExternalLink className="w-2.5 h-2.5 inline shrink-0 opacity-70" />}
          </a>
        );
      }

      case 'del':
        return (
          <del key={idx} className="line-through text-slate-500 dark:text-slate-400">
            <RenderInline tokens={token.tokens} />
          </del>
        );

      case 'image': {
        const src = safeMarkdownUrl(token.href, true);
        if (!src) return <span key={idx}>{token.text || ''}</span>;
        return (
          <span key={idx} className="inline-block my-1.5 max-w-full">
            <img
              src={src}
              alt={token.text || 'image'}
              className="rounded-xl border border-slate-200 dark:border-slate-800 max-h-60 object-contain shadow-xs"
            />
            {token.text && (
              <span className="block text-xs text-center text-slate-500 dark:text-slate-400 mt-1 italic">
                {token.text}
              </span>
            )}
          </span>
        );
      }

      case 'html': {
        const raw = token.text || token.raw || '';
        const kbdMatch = raw.match(/^<kbd>([^<]+)<\/kbd>$/i);
        if (kbdMatch) {
          return (
            <kbd
              key={idx}
              className="px-1.5 py-0.5 mx-0.5 text-xs font-mono font-semibold bg-slate-100 dark:bg-slate-800 text-slate-800 dark:text-slate-200 border border-slate-300/80 dark:border-slate-700 rounded-md shadow-2xs"
            >
              {kbdMatch[1]}
            </kbd>
          );
        }
        return <span key={idx}>{raw}</span>;
      }

      case 'escape':
        return <span key={idx}>{token.text}</span>;

      case 'br':
        return <br key={idx} />;

      default:
        return <span key={idx}>{token.text || token.raw}</span>;
    }
  });
}

// 3. Code Block Component with Copy, Line Numbers & Syntax Formatting
function CodeBlock({ code = '', lang = '' }) {
  const [copied, setCopied] = useState(false);
  const [isWrapped, setIsWrapped] = useState(false);

  const cleanLang = (lang || 'code').toLowerCase().trim();

  const handleCopy = async () => {
    await crossOriginCopy(code);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const lineCount = useMemo(() => (code || '').split('\n').length, [code]);

  return (
    <div className="my-3 rounded-xl border border-slate-800 bg-slate-950 text-slate-100 overflow-hidden shadow-md">
      {/* Code Header Bar */}
      <div className="flex items-center justify-between px-3.5 py-2 bg-slate-900 border-b border-slate-800/80 text-xs text-slate-400 font-mono">
        <div className="flex items-center space-x-2">
          {cleanLang === 'json' ? (
            <FileCode className="w-3.5 h-3.5 text-amber-400" />
          ) : ['bash', 'sh', 'curl'].includes(cleanLang) ? (
            <Terminal className="w-3.5 h-3.5 text-emerald-400" />
          ) : (
            <Code className="w-3.5 h-3.5 text-indigo-400" />
          )}
          <span className="font-semibold uppercase tracking-wider text-xs text-slate-300">
            {cleanLang}
          </span>
          {lineCount > 1 && (
            <span className="text-xs text-slate-400">
              ({lineCount} lines)
            </span>
          )}
        </div>

        <div className="flex items-center space-x-2">
          <button
            type="button"
            onClick={() => setIsWrapped(!isWrapped)}
            className="text-xs text-slate-400 hover:text-slate-200 transition cursor-pointer px-1.5 py-0.5 rounded hover:bg-slate-800"
            title={isWrapped ? '切换单行滚动' : '切换自动换行'}
          >
            {isWrapped ? 'Nowrap' : 'Wrap'}
          </button>
          <button
            type="button"
            onClick={handleCopy}
            className="inline-flex items-center gap-1 text-slate-300 hover:text-white transition cursor-pointer px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-xs shadow-xs"
            title="复制代码"
          >
            {copied ? (
              <Check className="w-3 h-3 text-emerald-400" />
            ) : (
              <Copy className="w-3 h-3 text-slate-400" />
            )}
            <span>{copied ? '已复制' : '复制'}</span>
          </button>
        </div>
      </div>

      {/* Code Content */}
      <pre
        className={`p-3.5 text-[11px] font-mono leading-relaxed select-text max-h-[380px] overflow-y-auto ${
          isWrapped ? 'whitespace-pre-wrap break-all' : 'overflow-x-auto whitespace-pre'
        }`}
      >
        <code>{renderHighlightedCode(code, cleanLang)}</code>
      </pre>
    </div>
  );
}

// 4. Recursive List Renderer with Task Support
function RenderList({ token, depth = 0 }) {
  const isOrdered = token.ordered;
  const Tag = isOrdered ? 'ol' : 'ul';
  const listClass = isOrdered
    ? 'list-decimal pl-5 space-y-1.5 my-2 text-slate-700 dark:text-slate-300'
    : 'list-disc pl-5 space-y-1.5 my-2 text-slate-700 dark:text-slate-300';

  return (
    <Tag className={listClass} start={isOrdered ? (token.start || 1) : undefined}>
      {token.items.map((item, itemIdx) => {
        const isTask = item.task;
        const isChecked = item.checked;

        return (
          <li
            key={itemIdx}
            className={`leading-relaxed ${
              isTask ? 'list-none -ml-4 flex items-start space-x-2 my-1' : ''
            }`}
          >
            {isTask && (
              <span className="mt-0.5 shrink-0 inline-flex items-center">
                {isChecked ? (
                  <CheckSquare className="w-3.5 h-3.5 text-indigo-600 dark:text-indigo-400" />
                ) : (
                  <Square className="w-3.5 h-3.5 text-slate-400 dark:text-slate-500" />
                )}
              </span>
            )}
            <div className="flex-1 min-w-0">
              {item.tokens ? (
                item.tokens.map((subToken, subIdx) => {
                  if (subToken.type === 'checkbox') return null;
                  if (subToken.type === 'list') {
                    // Recursive nested list
                    return <RenderList key={subIdx} token={subToken} depth={depth + 1} />;
                  }
                  if (subToken.type === 'text') {
                    return (
                      <RenderInline
                        key={subIdx}
                        tokens={subToken.tokens || [{ type: 'text', text: subToken.text }]}
                      />
                    );
                  }
                  if (subToken.type === 'paragraph') {
                    return (
                      <div key={subIdx} className="my-0.5">
                        <RenderInline tokens={subToken.tokens} />
                      </div>
                    );
                  }
                  return <RenderBlockToken key={subIdx} token={subToken} />;
                })
              ) : (
                <span>{item.text}</span>
              )}
            </div>
          </li>
        );
      })}
    </Tag>
  );
}

// 5. Blockquote / GitHub Callout Banner Component
function BlockquoteBlock({ tokens = [], rawText = '' }) {
  const text = rawText.trim();
  const lower = text.toLowerCase();

  let alertType = null;
  if (text.startsWith('[!NOTE]') || lower.startsWith('> [!note]')) alertType = 'note';
  else if (text.startsWith('[!TIP]') || lower.startsWith('> [!tip]')) alertType = 'tip';
  else if (text.startsWith('[!IMPORTANT]') || lower.startsWith('> [!important]')) alertType = 'important';
  else if (text.startsWith('[!WARNING]') || lower.startsWith('> [!warning]')) alertType = 'warning';
  else if (text.startsWith('[!CAUTION]') || text.startsWith('[!DANGER]') || lower.startsWith('> [!caution]')) alertType = 'danger';

  // If this is a GitHub Alert Callout, prepare clean inner tokens
  if (alertType) {
    const cleanedTokens = tokens.map((tok, i) => {
      if (i === 0 && tok.type === 'paragraph' && tok.tokens) {
        // Strip the [!TAG] tag from the first token
        const newSubTokens = tok.tokens.map((subTok, subI) => {
          if (subI === 0 && subTok.type === 'text') {
            const stripped = subTok.text.replace(/^>?\s*\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION|DANGER)\]\s*\n?/i, '');
            return { ...subTok, text: stripped, raw: stripped };
          }
          return subTok;
        });
        return { ...tok, tokens: newSubTokens };
      }
      return tok;
    });

    const alertStyles = {
      note: {
        border: 'border-blue-200 dark:border-blue-800/60',
        bg: 'bg-blue-50/70 dark:bg-blue-950/30',
        text: 'text-blue-900 dark:text-blue-200',
        title: 'text-blue-700 dark:text-blue-300',
        icon: <Info className="w-3.5 h-3.5 shrink-0" />,
        label: 'NOTE'
      },
      tip: {
        border: 'border-emerald-200 dark:border-emerald-800/60',
        bg: 'bg-emerald-50/70 dark:bg-emerald-950/30',
        text: 'text-emerald-900 dark:text-emerald-200',
        title: 'text-emerald-700 dark:text-emerald-300',
        icon: <Zap className="w-3.5 h-3.5 shrink-0" />,
        label: 'TIP'
      },
      important: {
        border: 'border-purple-200 dark:border-purple-800/60',
        bg: 'bg-purple-50/70 dark:bg-purple-950/30',
        text: 'text-purple-900 dark:text-purple-200',
        title: 'text-purple-700 dark:text-purple-300',
        icon: <Info className="w-3.5 h-3.5 shrink-0" />,
        label: 'IMPORTANT'
      },
      warning: {
        border: 'border-amber-200 dark:border-amber-800/60',
        bg: 'bg-amber-50/70 dark:bg-amber-950/30',
        text: 'text-amber-950 dark:text-amber-200',
        title: 'text-amber-700 dark:text-amber-300',
        icon: <AlertTriangle className="w-3.5 h-3.5 shrink-0" />,
        label: 'WARNING'
      },
      danger: {
        border: 'border-rose-200 dark:border-rose-800/60',
        bg: 'bg-rose-50/70 dark:bg-rose-950/30',
        text: 'text-rose-950 dark:text-rose-200',
        title: 'text-rose-700 dark:text-rose-300',
        icon: <ShieldAlert className="w-3.5 h-3.5 shrink-0" />,
        label: 'CAUTION'
      }
    };

    const cfg = alertStyles[alertType] || alertStyles.note;

    return (
      <div className={`my-3 p-3 rounded-xl border ${cfg.border} ${cfg.bg} ${cfg.text} text-xs`}>
        <div className={`flex items-center gap-1.5 font-bold ${cfg.title} mb-1.5 text-xs`}>
          {cfg.icon}
          <span>{cfg.label}</span>
        </div>
        <div className="leading-relaxed space-y-1.5">
          {cleanedTokens.map((tok, idx) => (
            <RenderBlockToken key={idx} token={tok} />
          ))}
        </div>
      </div>
    );
  }

  // Standard Blockquote with full markdown rendering
  return (
    <blockquote className="my-3 pl-3.5 py-1.5 border-l-4 border-indigo-500 bg-slate-50/80 dark:bg-slate-900/60 rounded-r-xl text-slate-700 dark:text-slate-300 text-xs italic leading-relaxed space-y-1">
      {tokens && tokens.length > 0 ? (
        tokens.map((tok, idx) => <RenderBlockToken key={idx} token={tok} />)
      ) : (
        <span>{text}</span>
      )}
    </blockquote>
  );
}

// 6. Table Component with Clean Borders and Styling
function TableBlock({ header = [], rows = [], align = [] }) {
  return (
    <div className="my-3 overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-[#0F1420] shadow-2xs">
      <table className="w-full text-left border-collapse text-xs">
        <thead className="bg-slate-50 dark:bg-slate-900 text-slate-800 dark:text-slate-200 font-semibold border-b border-slate-200 dark:border-slate-800">
          <tr>
            {header.map((col, idx) => (
              <th
                key={idx}
                className="py-2.5 px-3.5 text-xs font-semibold tracking-tight"
                style={{ textAlign: align[idx] || 'left' }}
              >
                <RenderInline tokens={col.tokens} />
              </th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-slate-100 dark:divide-slate-800/80">
          {rows.map((row, rIdx) => (
            <tr
              key={rIdx}
              className="hover:bg-slate-50/60 dark:hover:bg-slate-800/40 transition-colors"
            >
              {row.map((cell, cIdx) => (
                <td
                  key={cIdx}
                  className="py-2.5 px-3.5 text-slate-700 dark:text-slate-300 text-xs leading-relaxed"
                  style={{ textAlign: align[cIdx] || 'left' }}
                >
                  <RenderInline tokens={cell.tokens} />
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// 7. Render Block Token Dispatcher
function RenderBlockToken({ token }) {
  if (!token) return null;

  switch (token.type) {
    case 'heading': {
      if (token.depth === 1) {
        return (
          <h1 className="text-base font-bold text-slate-900 dark:text-white mt-4 mb-2 pb-1.5 border-b border-slate-200/80 dark:border-slate-800">
            <RenderInline tokens={token.tokens} />
          </h1>
        );
      }
      if (token.depth === 2) {
        return (
          <h2 className="text-sm font-bold text-slate-900 dark:text-white mt-3.5 mb-1.5">
            <RenderInline tokens={token.tokens} />
          </h2>
        );
      }
      if (token.depth === 3) {
        return (
          <h3 className="text-xs font-bold text-indigo-600 dark:text-indigo-400 mt-3 mb-1">
            <RenderInline tokens={token.tokens} />
          </h3>
        );
      }
      return (
        <h4 className="text-xs font-semibold text-slate-800 dark:text-slate-200 mt-2 mb-0.5">
          <RenderInline tokens={token.tokens} />
        </h4>
      );
    }

    case 'paragraph':
      return (
        <p className="my-1.5 leading-relaxed">
          <RenderInline tokens={token.tokens} />
        </p>
      );

    case 'code':
      return <CodeBlock code={token.text} lang={token.lang} />;

    case 'blockquote':
      return (
        <BlockquoteBlock
          tokens={token.tokens}
          rawText={token.text || token.raw}
        />
      );

    case 'list':
      return <RenderList token={token} />;

    case 'table':
      return (
        <TableBlock
          header={token.header}
          rows={token.rows}
          align={token.align}
        />
      );

    case 'hr':
      return <hr className="my-3 border-slate-200 dark:border-slate-800" />;

    case 'space':
      return null;

    default:
      return (
        <div className="whitespace-pre-wrap leading-relaxed">
          {token.raw}
        </div>
      );
  }
}

// 8. Main Markdown Renderer Component
export default function MarkdownRenderer({ content = '' }) {
  // Auto-close unclosed markdown code fences if streaming cut off in the middle
  const normalizedContent = useMemo(() => {
    let text = content;
    const tripleBackticks = (text.match(/```/g) || []).length;
    if (tripleBackticks % 2 !== 0) {
      text += '\n```';
    }
    return text;
  }, [content]);

  if (!content) return null;

  let tokens = [];
  try {
    tokens = marked.lexer(normalizedContent);
  } catch (err) {
    return <div className="whitespace-pre-wrap leading-relaxed">{content}</div>;
  }

  return (
    <div className="space-y-2 text-xs leading-relaxed text-slate-800 dark:text-slate-200 select-text">
      {tokens.map((token, idx) => (
        <RenderBlockToken key={idx} token={token} />
      ))}
    </div>
  );
}
