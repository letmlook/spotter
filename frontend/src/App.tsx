import { useCallback } from 'react';
import { notification } from 'antd';
import TitleBar from './components/TitleBar';
import Sidebar from './components/Sidebar';
import DetailPanel from './components/DetailPanel';
import StatusBar from './components/StatusBar';
import { useWailsEvents } from './hooks/useWailsEvents';

export default function App() {
  const handleUnknownDevice = useCallback((payload: unknown) => {
    const data = payload as {
      Info?: { basic?: { hostname?: string }; jetson?: { model?: string } };
      IP?: string;
      Port?: number;
    };
    const hostname = data?.Info?.basic?.hostname;
    const model = data?.Info?.jetson?.model;
    const ip = data?.IP;
    const port = data?.Port;
    const descParts: string[] = [];
    if (hostname) descParts.push(`hostname: ${hostname}`);
    if (model) descParts.push(`model: ${model}`);
    if (ip) descParts.push(`ip: ${ip}${port ? ':' + port : ''}`);
    notification.info({
      message: 'New device discovered',
      description: descParts.length ? descParts.join(' | ') : 'A new device has been discovered on the network.',
      placement: 'bottomRight',
    });
  }, []);
  useWailsEvents(handleUnknownDevice);
  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100vh' }}>
      <TitleBar />
      <div style={{ display: 'flex', flex: 1, minHeight: 0 }}>
        <Sidebar />
        <main style={{ flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0, background: '#141414' }}>
          <DetailPanel />
          <StatusBar />
        </main>
      </div>
    </div>
  );
}
