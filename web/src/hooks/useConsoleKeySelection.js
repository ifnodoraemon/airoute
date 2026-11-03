import { useMemo, useState } from 'react';

// A console selection is a non-secret record ID. Authentication comes from the
// login session; the server checks ownership and the current key policy.
export function useConsoleKeySelection(keys = []) {
  const availableKeys = useMemo(() => keys.filter(key => key.id != null && (!key.status || key.status === 'active')), [keys]);
  const [chosenId, setKeyId] = useState('');
  const selectedKey = availableKeys.find(key => String(key.id) === chosenId) || availableKeys[0] || null;
  return { availableKeys, selectedKey, keyId: selectedKey ? String(selectedKey.id) : '', setKeyId };
}
