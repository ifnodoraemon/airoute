import { ArrowRight, CheckCircle2, Circle } from 'lucide-react';
import { translations } from '../i18n';

// Progress is derived from the same channel, key and traffic data as attention
// notices. Failed calls never count as a successful first request.
export default function SetupChecklist({
  channels = [], keys = [], models = [], stats = {}, attention = {}, isZh,
  setCurrentTab, setShowChannelModal, onOpenOnboarding, onNavigateToErrors,
}) {
  const copy = translations[isZh ? 'zh' : 'en'].common;
  const activeChannels = channels.filter(channel => channel.status === 'active');
  const routable = activeChannels.filter(channel => !['OPEN', 'HALF-OPEN'].includes(String(channel.breaker_status || '').toUpperCase()));
  const hasKey = keys.some(key => key.status === 'active' && !(key.budget > 0 && key.used_cost >= key.budget));
  const steps = [
    { key: 'channel', done: routable.length > 0, label: copy.checklistChannel, action: copy.checklistChannelAction,
      onClick: () => channels.length ? setCurrentTab?.('channels') : setShowChannelModal?.(true) },
    { key: 'route', done: routable.length > 0 && models.length > 0, label: copy.checklistRoute, action: copy.checklistRouteAction,
      onClick: () => setCurrentTab?.('models') },
    { key: 'key', done: hasKey, label: copy.checklistKey, action: copy.checklistKeyAction,
      onClick: () => setCurrentTab?.('keys') },
    { key: 'request', done: stats.success_requests > 0, label: copy.checklistRequest, action: copy.checklistRequestAction,
      onClick: () => onOpenOnboarding ? onOpenOnboarding() : setCurrentTab?.('playground') },
  ];
  const completed = steps.filter(step => step.done).length;
  if (completed === steps.length) return null;
  return (
    <section aria-label={copy.checklistTitle} className="ui-card p-4 space-y-3">
      <div className="flex items-center justify-between gap-3">
        <h3 className="ui-section-title">{copy.checklistTitle}</h3>
        <span className="ui-meta">{copy.checklistProgress.replace('{done}', completed).replace('{total}', steps.length)}</span>
      </div>
      <ol className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {steps.map(step => (
          <li key={step.key} className="flex items-start gap-2 text-xs">
            {step.done ? <CheckCircle2 className="w-4 h-4 text-emerald-500 shrink-0" aria-label={copy.operational} /> : <Circle className="w-4 h-4 text-slate-400 shrink-0" />}
            <div>
              <span>{step.label}</span>
              {!step.done && <button type="button" onClick={step.onClick} className="mt-1 flex items-center gap-1 text-[#355CFF] dark:text-[#91A7FF] hover:underline">{step.action}<ArrowRight className="w-3 h-3" /></button>}
            </div>
          </li>
        ))}
      </ol>
      {attention.tripped > 0 && <p className="ui-meta text-amber-700 dark:text-amber-300">{copy.checklistBreaker}</p>}
      {attention.failed > 0 && <p className="ui-meta">{copy.checklistFailures} <button type="button" className="text-[#355CFF] dark:text-[#91A7FF] hover:underline" onClick={() => onNavigateToErrors ? onNavigateToErrors() : setCurrentTab?.('logs')}>{copy.viewErrors}</button></p>}
    </section>
  );
}
