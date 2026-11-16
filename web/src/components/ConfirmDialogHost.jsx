import { useEffect, useState } from 'react';
import ConfirmDialog from './ConfirmDialog';
import { subscribeConfirm } from '../lib/confirmBus';
import { useConsoleContext } from '../console/ConsoleContext';

// Renders the single global confirmation dialog driven by confirmBus. Mounted
// once at the console root so every hook/component can request a confirmation
// without rendering its own dialog.
export default function ConfirmDialogHost() {
  const { isZh } = useConsoleContext();
  const [request, setRequest] = useState(null);
  useEffect(() => subscribeConfirm((req) => setRequest(req)), []);
  if (!request) return null;
  const settle = (result) => {
    request.resolve(result);
    setRequest(null);
  };
  return (
    <ConfirmDialog
      open
      isZh={isZh}
      title={request.options.title}
      description={request.options.description}
      confirmLabel={request.options.confirmLabel}
      cancelLabel={request.options.cancelLabel}
      danger={request.options.danger !== false}
      requireText={request.options.requireText}
      onConfirm={() => settle(true)}
      onCancel={() => settle(false)}
    />
  );
}