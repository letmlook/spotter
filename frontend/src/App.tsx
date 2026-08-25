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
import { ScanSubnet } from '../wailsjs/go/main/ScannerApp';
import { I18nProvider, useI18n } from './state/I18nContext';
import { ThemeProvider, useTheme } from './state/ThemeContext';

function AppInner() {
  const { t } = useI18n();
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
    if (hostname) descParts.push(`${t('notif.newdev.hostname')}: ${hostname}`);
    if (model) descParts.push(`${t('notif.newdev.model')}: ${model}`);
    if (ip) descParts.push(`${t('notif.newdev.ip')}: ${ip}${port ? ':' + port : ''}`);
    notification.info({
      message: t('notif.newdev.title'),
      description: descParts.length ? descParts.join(' | ') : t('notif.newdev.fallback'),
      placement: 'bottomRight',
    });
  }, [t]);
  useWailsEvents(handleUnknownDevice);

  const { setTriggerScan } = useMenu();
  useEffect(() => {
    setTriggerScan(async () => { await ScanSubnet(''); });
  }, [setTriggerScan]);

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