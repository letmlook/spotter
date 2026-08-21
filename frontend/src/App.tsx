import TitleBar from './components/TitleBar';
import Sidebar from './components/Sidebar';
import DetailPanel from './components/DetailPanel';
import { useWailsEvents } from './hooks/useWailsEvents';

export default function App() {
  useWailsEvents();
  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100vh' }}>
      <TitleBar />
      <div style={{ display: 'flex', flex: 1, minHeight: 0 }}>
        <Sidebar />
        <main style={{ flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0, background: '#141414' }}>
          <DetailPanel />
        </main>
      </div>
    </div>
  );
}
