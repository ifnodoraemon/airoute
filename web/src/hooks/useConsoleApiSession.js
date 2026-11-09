import { useCallback } from 'react';
import { useApiSession } from './useApiSession';

export default function useConsoleApiSession({
  adminToken, endSession, getSessionGeneration, viewMode
}) {
  const handleUnauthorized = useCallback(({ token: usedToken }) => {
    // Only a console session holding a credential is torn down here; the landing
    // and auth pages also make unauthenticated calls.
    if (viewMode !== 'console' || !usedToken) return;
    endSession({ notify: true });
  }, [viewMode, endSession]);

  // Bind the shared request layer (src/api/client.js) to this session. It used to
  // be a pair of closures defined in this component (adminFetch/adminJSON) that
  // every view had to receive through props; the props below are kept so existing
  // views retain their contract.
  useApiSession({ token: adminToken, onUnauthorized: handleUnauthorized, getSessionGeneration });
  return {

  };
}
