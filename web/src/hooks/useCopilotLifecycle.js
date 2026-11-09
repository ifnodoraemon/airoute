import { useEffect, useRef } from 'react';

export default function useCopilotLifecycle({
  abortControllerRef, initialQuery, isOpen, messages, messagesEndRef, onClearInitialQuery, setIsLoading
}) {
  const wasOpenRef = useRef(isOpen);
  useEffect(() => {
    if (wasOpenRef.current && !isOpen) {
      abortControllerRef.current?.abort();
      abortControllerRef.current = null;
      setIsLoading(false);
    }
    wasOpenRef.current = isOpen;
  }, [abortControllerRef, isOpen, setIsLoading]);
  // Unmounting without a prior close (logout, session expiry) has to abort too.
  useEffect(() => () => {
    abortControllerRef.current?.abort();
    abortControllerRef.current = null;
  }, [abortControllerRef]);

  // Scroll to bottom on new messages
  useEffect(() => {
    if (isOpen) {
      messagesEndRef.current?.scrollIntoView?.({ behavior: 'smooth' });
    }
  }, [messages, isOpen, messagesEndRef]);

  // Handle incoming initial query.
  //
  // handleSend is rebuilt on every render and closes over the current input, so it
  // cannot be a dependency: doing so would re-send the query on every keystroke.
  // The latest version is read through a ref instead.
  //
  // The ref starts empty and is filled in by the effect below handleSend's
  // definition. Reading handleSend here would throw: it is a const declared further
  // down, so it is in the temporal dead zone at this point.
  const handleSendRef = useRef(null);

  useEffect(() => {
    if (isOpen && initialQuery && initialQuery.trim() && handleSendRef.current) {
      handleSendRef.current(initialQuery.trim());
      if (onClearInitialQuery) onClearInitialQuery();
    }
  }, [isOpen, initialQuery, onClearInitialQuery]);
  return {
    handleSendRef
  };
}
