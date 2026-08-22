import { useState } from 'react';
import { Button } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { useDevices } from '../state/DeviceContext';
import { useDeviceActions } from '../hooks/useDeviceActions';

export default function DeviceActions() {
  const { state, refresh } = useDevices();
  const actions = useDeviceActions();
  const [busy, setBusy] = useState(false);
  const device = state.devices.find((d) => d.device_id === state.selectedId);
  if (!device) return null;

  const onRefresh = async () => {
    setBusy(true);
    try { await actions.refresh(); await refresh(); }
    finally { setBusy(false); }
  };

  return (
    <div style={{ borderTop: '1px solid #303030', padding: '12px 16px', background: '#0a0a0a' }}>
      <div style={{ marginBottom: 8, fontSize: 12, color: '#888', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
        Device actions
      </div>
      <Button icon={<ReloadOutlined />} onClick={onRefresh} loading={busy} block>
        Refresh
      </Button>
    </div>
  );
}