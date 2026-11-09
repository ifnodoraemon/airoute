import { ConsoleContext } from './console/ConsoleContext';
import ConsoleApplication from './console/ConsoleApplication';
import useConsoleWorkspace from './hooks/useConsoleWorkspace';

export { ViewLoader } from './console/consoleComponents';

export default function App() {
  const workspace = useConsoleWorkspace();
  return (
    <ConsoleContext.Provider value={workspace}>
      <ConsoleApplication />
    </ConsoleContext.Provider>
  );
}
