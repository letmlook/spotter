import { useDevices } from '../state/DeviceContext';
import BasicCard from './BasicCard';
import NetworkCard from './NetworkCard';
import JetsonCard from './JetsonCard';
import EmptyState from './EmptyState';

export default function DetailPanel({ onAction }: { onAction: (which: 'deploy' | 'scan' | 'add') => void }) {
  const { state } = useDevices();
  const device = state.devices.find((d) => d.device_id === state.selectedId);

  if (!device) return <EmptyState onAction={onAction} />;

  const hostname = device.last_info?.basic?.hostname || device.ip;
  const statusText = device.online ? 'online' : 'offline';

  return (
    <div style={{ padding: 16, overflow: 'auto', height: '100%' }}>
      <h2 style={{ margin: '0 0 16px 0', color: '#fff' }}>
        {hostname}
        <span style={{ marginLeft: 12, fontSize: 13, color: device.online ? '#52c41a' : '#b71c1c' }}>{statusText}</span>
      </h2>
      <div style={{ display: 'grid', gap: 12 }}>
        <BasicCard device={device} />
        <NetworkCard device={device} />
        <JetsonCard device={device} />
      </div>
    </div>
  );
}