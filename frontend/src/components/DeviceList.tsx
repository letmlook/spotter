import { Empty, Input } from 'antd';
import { useDevices } from '../state/DeviceContext';
import DeviceRow from './DeviceRow';

export default function DeviceList() {
  const { state, dispatch } = useDevices();
  const filtered = state.devices.filter((d) => {
    if (!state.searchQuery) return true;
    const q = state.searchQuery.toLowerCase();
    return d.ip.toLowerCase().includes(q) ||
      (d.last_info?.basic?.hostname || '').toLowerCase().includes(q) ||
      d.username.toLowerCase().includes(q);
  });

  return (
    <div style={{ display: 'flex', flexDirection: 'column', flex: 1, minHeight: 0 }}>
      <div style={{ padding: '8px 12px' }}>
        <Input.Search
          placeholder="Search devices"
          allowClear
          size="small"
          value={state.searchQuery}
          onChange={(e) => dispatch({ type: 'SEARCH', query: e.target.value })}
        />
      </div>
      <div style={{ flex: 1, overflow: 'auto', padding: '0 8px' }}>
        {filtered.length === 0 ? (
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={state.devices.length === 0 ? 'No devices' : 'No matches'}
            style={{ marginTop: 24 }}
          />
        ) : (
          filtered.map((d) => (
            <DeviceRow
              key={d.device_id}
              device={d}
              selected={state.selectedId === d.device_id}
              onClick={() => dispatch({ type: 'SELECT', id: d.device_id })}
            />
          ))
        )}
      </div>
    </div>
  );
}
