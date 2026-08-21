import { useDevices } from '../state/DeviceContext';
import BasicCard from './BasicCard';
import NetworkCard from './NetworkCard';
import JetsonCard from './JetsonCard';
import EmptyState from './EmptyState';

export default function DetailPanel() {
  const { state } = useDevices();
  const device = state.devices.find((d) => d.device_id === state.selectedId);
  if (!device) return <EmptyState />;

  const hostname = device.last_info?.basic?.hostname || device.ip;
  return (
    <div style={{ padding: 16, overflow: 'auto', height: '100%' }}>
      <h2 style={{ margin: '0 0 16px 0', color: '#fff' }}>
        {hostname}
        <span style={{ marginLeft: 12, fontSize: 13 }} className={device.online ? 'online' : 'offline'}>
          {device.online ? 'online' : 'offline'}
        </span>
      </h2>
      <div style={{ display: 'grid', gap: 12 }}>
        <BasicCard device={device} />
        <NetworkCard device={device} />
        <JetsonCard device={device} />
      </div>
    </div>
  );
}
