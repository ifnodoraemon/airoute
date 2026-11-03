// The console's transient selections: which quick-access key is highlighted, which
// log row is expanded, which trace snapshot is open, and the generated curl
// command.
//
// Grouped because they share one rule: they are all tenant-scoped, so logging out
// has to clear them. That clearing used to be four lines in the middle of a
// twenty-line reset function, where adding a fifth selection meant remembering to
// extend it.

import { useCallback, useState } from 'react';

export function useConsoleSelections() {
  const [activeQuickKey, setActiveQuickKey] = useState(null);
  const [activeLogDetail, setActiveLogDetail] = useState(null);
  const [traceSnapshotId, setTraceSnapshotId] = useState('');
  const [curlExportCmd, setCurlExportCmd] = useState('');
  const [playgroundScenario, setPlaygroundScenario] = useState(null);

  const reset = useCallback(() => {
    setActiveQuickKey(null);
    setActiveLogDetail(null);
    setTraceSnapshotId('');
    setCurlExportCmd('');
    setPlaygroundScenario(null);
  }, []);

  return {
    activeQuickKey,
    setActiveQuickKey,
    activeLogDetail,
    setActiveLogDetail,
    traceSnapshotId,
    setTraceSnapshotId,
    curlExportCmd,
    setCurlExportCmd,
    playgroundScenario,
    setPlaygroundScenario,
    reset,
  };
}
