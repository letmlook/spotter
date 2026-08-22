import { useDevices } from '../state/DeviceContext';
import { useI18n } from '../state/I18nContext';
import BasicCard from './BasicCard';
import NetworkCard from './NetworkCard';
import JetsonCard from './JetsonCard';
import EmptyState from './EmptyState';
import DeviceActions from './DeviceActions';

export default function DetailPanel() {
  const { state } = useDevices();
  const { t } = useI18n();
  const device = state.devices.find((d) => d.device_id === state.selectedId);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <div style={{ flex: 1, padding: 16, overflow: 'auto' }}>
        {!device ? <EmptyState /> : (
          <>
            <h2 style={{ margin: '0 0 16px 0', color: 'var(--text-primary)' }}>
              {device.last_info?.basic?.hostname || device.ip}
              <span style={{ marginLeft: 12, fontSize: 13, color: device.online ? '#52c41a' : '#b71c1c' }}>
                {device.online ? t('detail.status.online') : t('detail.status.offline')}
              </span>
            </h2>
            <div style={{ display: 'grid', gap: 12 }}>
              <BasicCard device={device} />
              <NetworkCard device={device} />
              <JetsonCard device={device} />
            </div>
          </>
        )}
      </div>
      {device && <DeviceActions />}
    </div>
  );
}