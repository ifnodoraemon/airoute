import { useState } from 'react';
import useModalA11y from '../hooks/useModalA11y';

export default function SkillTrialModal({ skill, onTrial, onClose, lang = 'zh' }) {
  const isZh = lang === 'zh';
  const [prompt, setPrompt] = useState('');
  useModalA11y(true, onClose);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/60 p-4 backdrop-blur-sm" onClick={e => { if (e.target === e.currentTarget) onClose(); }}>
      <section role="dialog" aria-modal="true" aria-labelledby="skill-trial-title" className="ui-card w-full max-w-xl space-y-4 p-6 bg-white dark:bg-[#191B1F]">
        <h2 id="skill-trial-title" className="text-base font-semibold">{isZh ? '技能模型实测' : 'Test skill with a model'}: {skill.name}</h2>
        <p className="text-xs leading-relaxed">
          {isZh ? '将 SKILL.md 作为系统指引带入 Playground，选择模型与 API 密钥后发送真实请求。响应、耗时和扣费可在调用日志中核验。需要操作本地文件或工具的任务，请通过客户端接入指南执行。' : 'Open Playground with SKILL.md as system instructions, then select a model and API key to send a real request. Verify the response, latency and charge in call logs. Use the client guide for tasks requiring local files or tools.'}
        </p>
        <label className="block space-y-2 text-xs">
          <span>{isZh ? '测试任务' : 'Test task'}</span>
          <textarea className="ui-field" rows={4} value={prompt} onChange={e => setPrompt(e.target.value)} />
        </label>
        <div className="flex justify-end gap-2">
          <button className="ui-btn-secondary" onClick={onClose}>{isZh ? '取消' : 'Cancel'}</button>
          <button className="ui-btn-primary" disabled={!prompt.trim() || !skill.manifest?.trim() || !onTrial} onClick={() => onTrial({ skillId: skill.id, systemPrompt: skill.manifest, prompt: prompt.trim() })}>
            {isZh ? '前往 Playground 实测' : 'Open Playground to test'}
          </button>
        </div>
      </section>
    </div>
  );
}
