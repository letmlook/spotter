import { useCallback, useEffect } from 'react';
import { notification } from 'antd';
import TitleBar from './components/TitleBar';
import Sidebar from './components/Sidebar';
import DetailPanel from './components/DetailPanel';
import StatusBar from './components/StatusBar';
import DeviceSetupGuide from './components/DeviceSetupGuide';
import AddDeviceByIPDialog from './components/AddDeviceByIPDialog';
import AboutDialog from './components/AboutDialog';
import { useWailsEvents } from './hooks/useWailsEvents';
import { useMenu, MenuProvider } from './state/MenuContext';
import { ScanSubnet } from '../wailsjs/go/main/App';

function AppInner() {
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

  // Register the scan trigger so menu items can invoke a one-click
  // scan without prop-drilling.
  const { setTriggerScan } = useMenu();
  useEffect(() => {
    setTriggerScan(async () => { await ScanSubnet(''); });
  }, [setTriggerScan]);

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

      {/* Global modals — controlled via MenuContext. */}
      <DeviceSetupGuideHost />
      <AddDeviceByIPDialog />
      <AboutDialog />
    </div>
  );
}

// DeviceSetupGuide owns its own modal state but we need to bind it
// to MenuContext.modal === 'setup-guide'. A small host wrapper
// keeps the modals colocated with the provider.
function DeviceSetupGuideHost() {
  const { modal, closeModal } = useMenu();
  return <DeviceSetupGuide open={modal === 'setup-guide'} onClose={closeModal} />;
}

export default function App() {
  return (
    <MenuProvider>
      <AppInner />
    </MenuProvider>
  );
}