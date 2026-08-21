import { useDevices } from '../state/DeviceContext';

export default function StatusBar() {
  const { state } = useDevices();
  const online = state.devices.filter((d) => d.online).length;
  const total = state.devices.length;
  return (
    <div
      style={{
        height: 24, flexShrink: 0,
        padding: '0 16px',
        display: 'flex', alignItems: 'center',
        background: '#0a0a0a', borderTop: '1px solid #303030',
        fontSize: 12, color: '#888',
      }}
    >
      {online} online / {total} total
      {state.loading && <span style={{ marginLeft: 12 }}>refreshing…</span>}
    </div>
  );
}