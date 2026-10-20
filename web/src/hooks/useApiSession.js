// Binds the request layer to the current session.
//
// The client (src/api/client.js) holds no React state, so something has to hand
// it the live token and the 401 policy. Doing that with a plain effect that
// depends on those values re-registers the client on every render; capturing
// them directly instead is the stale-closure bug the old in-component
// adminFetch had.
//
// This hook registers exactly once and exposes the latest values through refs,
// so a request always sees the current token and the current handler.

import { useEffect, useRef } from 'react';
import { configureApiClient } from '../api/client';

/**
 * @param {object}   options
 * @param {string}   options.token           current bearer token ('' for cookie sessions)
 * @param {Function} options.onUnauthorized  called as ({response, token}) on every 401
 */
export function useApiSession({ token, onUnauthorized }) {
  const tokenRef = useRef(token);
  const unauthorizedRef = useRef(onUnauthorized);

  useEffect(() => {
    tokenRef.current = token;
  }, [token]);

  useEffect(() => {
    unauthorizedRef.current = onUnauthorized;
  }, [onUnauthorized]);

  // Registered once: the indirections above are what keep it current.
  useEffect(() => {
    configureApiClient({
      token: () => tokenRef.current,
      unauthorized: (ctx) => unauthorizedRef.current(ctx),
    });
  }, []);
}
