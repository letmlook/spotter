import TitleBar from './components/TitleBar';
import Sidebar from './components/Sidebar';
import { useWailsEvents } from './hooks/useWailsEvents';

export default function App() {
  useWailsEvents();
  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100vh' }}>
      <TitleBar />
      <div style={{ display: 'flex', flex: 1, minHeight: 0 }}>
        <Sidebar />
        <div style={{ flex: 1, padding: 24 }}>Main area placeholder</div>
      </div>
    </div>
  );
}
