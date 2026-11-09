import { createContext, useContext } from 'react';

export const ConsoleContext = createContext(null);

export function useConsoleContext() {
  return useContext(ConsoleContext);
}
