import { useDevices } from '../state/DeviceContext';
import BasicCard from './BasicCard';
import NetworkCard from './NetworkCard';
import JetsonCard from './JetsonCard';
import EmptyState from './EmptyState';
import DeviceActions from './DeviceActions';

export default function DetailPanel() {
  const { state } = useDevices();
  const device = state.devices.find((d) => d.device_id === state.selectedId);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <div style={{ flex: 1, padding: 16, overflow: 'auto' }}>
        {!device ? <EmptyState /> : (
          <>
            <h2 style={{ margin: '0 0 16px 0', color: '#fff' }}>
              {device.last_info?.basic?.hostname || device.ip}
              <span style={{ marginLeft: 12, fontSize: 13 }} className={device.online ? 'online' : 'offline'}>
                {device.online ? 'online' : 'offline'}
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
