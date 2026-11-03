const THOUGHT_BLOCKS = [
  ['<think>', '</think>'],
  ['<|channel>thought', '<channel|>'],
];

export function textContent(value) {
  if (typeof value === 'string') return value;
  if (!Array.isArray(value)) return '';
  return value.map(part => typeof part === 'string' ? part : (typeof part?.text === 'string' ? part.text : '')).join('');
}

function withoutPartialTag(text, marker, complete) {
  if (complete) return text;
  for (let size = Math.min(text.length, marker.length - 1); size > 0; size -= 1) {
    if (text.endsWith(marker.slice(0, size))) return text.slice(0, -size);
  }
  return text;
}

// This is a presentation adapter. Native API payloads retain their original
// contents; only the playground separates inline thought channels for display.
export function assistantContent(raw, explicitReasoning = '', complete = false) {
  const value = textContent(raw);
  const trimmed = value.trimStart();
  for (const [start, end] of THOUGHT_BLOCKS) {
    if (!complete && start.startsWith(trimmed)) return { content: '', reasoning: explicitReasoning };
    if (!trimmed.startsWith(start)) continue;
    const remainder = trimmed.slice(start.length).replace(/^\r?\n/, '');
    const stop = remainder.indexOf(end);
    if (stop < 0) {
      return { content: '', reasoning: explicitReasoning || withoutPartialTag(remainder, end, complete) };
    }
    const reasoning = remainder.slice(0, stop).trim();
    const content = remainder.slice(stop + end.length).replace(/^\r?\n/, '').replace(/^<\|channel>final\r?\n?/, '');
    return { content, reasoning: explicitReasoning || reasoning };
  }
  return { content: value, reasoning: explicitReasoning };
}
