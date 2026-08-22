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
        background: 'var(--bg-app)', borderTop: '1px solid var(--border)',
        fontSize: 12, color: 'var(--text-secondary)',
      }}
    >
      {online} online / {total} total
      {state.loading && <span style={{ marginLeft: 12 }}>refreshing…</span>}
    </div>
  );
}