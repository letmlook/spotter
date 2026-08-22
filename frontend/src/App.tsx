import { useCallback, useEffect, useMemo } from 'react';
import { ConfigProvider, notification, theme as antdTheme } from 'antd';
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
import { I18nProvider, useI18n } from './state/I18nContext';
import { ThemeProvider, useTheme } from './state/ThemeContext';

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

  // Register the one-click scan trigger so menu items can fire it.
  const { setTriggerScan } = useMenu();
  useEffect(() => {
    setTriggerScan(async () => { await ScanSubnet(''); });
  }, [setTriggerScan]);

  // AntD ConfigProvider needs to react to theme changes. The
  // darkAlgorithm / defaultAlgorithm branches re-tokenise every
  // AntD component (Modal, Dropdown, Form, Button, etc.).
  const { theme: themeName } = useTheme();
  const algorithm = useMemo(
    () => (themeName === 'light' ? antdTheme.defaultAlgorithm : antdTheme.darkAlgorithm),
    [themeName],
  );

  return (
    <ConfigProvider
      theme={{
        algorithm,
        token: { colorPrimary: '#1677ff', borderRadius: 6 },
      }}
    >
      <div style={{ display: 'flex', flexDirection: 'column', height: '100vh', background: 'var(--bg-main)' }}>
        <TitleBar />
        <div style={{ display: 'flex', flex: 1, minHeight: 0 }}>
          <Sidebar />
          <main style={{ flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0, background: 'var(--bg-main)' }}>
            <DetailPanel />
            <StatusBar />
          </main>
        </div>
        <DeviceSetupGuideHost />
        <AddDeviceByIPDialog />
        <AboutDialog />
      </div>
    </ConfigProvider>
  );
}

function DeviceSetupGuideHost() {
  const { modal, closeModal } = useMenu();
  return <DeviceSetupGuide open={modal === 'setup-guide'} onClose={closeModal} />;
}

export default function App() {
  return (
    <ThemeProvider>
      <I18nProvider>
        <MenuProvider>
          <AppInner />
        </MenuProvider>
      </I18nProvider>
    </ThemeProvider>
  );
}