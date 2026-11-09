import { copyToClipboard as crossOriginCopy } from '../config';
import { useEffect } from 'react';

export default function usePlaygroundPrompts({
  chatInput, chatInputRef, isZh, playModality, setChatInput, setShowSystemPrompt, setSystemPrompt, showToast,
  systemPrompt
}) {
  const copyToClipboard = async (txt, label = '') => {
    await crossOriginCopy(txt);
    if (showToast) showToast(isZh ? `${label || '内容'} 已复制到剪贴板！` : `${label || 'Content'} copied to clipboard!`, 'success');
  };

  const handleOptimizeSystemPrompt = () => {
    const base = systemPrompt.trim() || (isZh ? '全栈技术专家与架构师' : 'Expert Full-Stack Architect');
    const optimized = isZh
      ? `# 角色定位 (Role)
你是一名顶尖的企业级 AI 架构师与专业任务执行专家，具备深厚工程化落地与严谨的逻辑推理能力。

# 核心任务 (Objective)
深度理解并高精度执行用户指令，兼顾代码健壮性、架构拓展性与清晰可维护性：
【${base}】

# 上下文约束与最佳实践 (Constraints)
1. 严禁捏造事实或虚构不存在的技术参数（零幻觉原则）。
2. 如涉及关键数据或技术选型，必须给出可量化的决策权衡依据。
3. 遵循安全性与合规性原则，避免返回不合规的高危操作指令。

# 输出规范 (Output Format)
- 采用清晰的 GitHub Markdown 格式组织。
- 若包含代码，需附带详尽的关键行注释。
- 提供结构化结论及后续可直接行动项 (Action Items)。`
      : `# Role
You are a distinguished enterprise AI architect and software engineer with deep technical rigor.

# Objective
Execute the following mission with precision, maintainability, and zero speculation:
[${base}]

# Constraints
1. Zero-hallucination principle: Do not invent non-existent APIs or parameters.
2. Provide measurable trade-offs for all architectural decisions.
3. Adhere to strict security and safety standards.

# Output Format
- GitHub Markdown formatting with syntax-highlighted code blocks.
- Actionable next steps.`;

    setSystemPrompt(optimized);
    setShowSystemPrompt(true);
    if (showToast) showToast(isZh ? '提示词已优化' : 'System Prompt enhanced with AI prompt engineering!', 'success');
  };

  const handleOptimizeChatInput = () => {
    if (!chatInput.trim()) return;
    const base = chatInput.trim();
    const optimized = isZh
      ? `请针对以下需求展开专业、深入且结构化的解答：\n\n【核心目标】：${base}\n\n【具体要求】：\n1. 阐明核心底层原理与技术选型权衡\n2. 给出经过生产验证的代码实现或最佳实践架构方案\n3. 提示潜在边界陷阱（如高并发、网络延迟、内存开销）及对应防御措施`
      : `Please provide a rigorous, in-depth, and structured response for:\n\n[Core Goal]: ${base}\n\n[Requirements]:\n1. Explain underlying principles and architectural trade-offs\n2. Provide production-grade implementation or configuration examples\n3. Highlight edge-case pitfalls (concurrency, latency, failure recovery) and remediations`;

    setChatInput(optimized);
    if (showToast) showToast(isZh ? '输入 Prompt 已完成 AI 结构化规范增强！' : 'Prompt enriched with structural requirements!', 'success');
  };

  // Auto-grow composer: starts at one row, expands with the content up to
  // ~200px, then scrolls internally.
  useEffect(() => {
    const el = chatInputRef.current;
    if (!el) return;
    el.style.height = 'auto';
    el.style.height = `${Math.min(el.scrollHeight, 200)}px`;
  }, [chatInput, chatInputRef, playModality]);
  return {
    copyToClipboard, handleOptimizeChatInput, handleOptimizeSystemPrompt
  };
}
