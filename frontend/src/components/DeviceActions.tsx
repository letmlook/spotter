import { useEffect, useState } from 'react';
import { Button, Input, Space, message } from 'antd';
import { ReloadOutlined, DeleteOutlined } from '@ant-design/icons';
import { useDevices } from '../state/DeviceContext';
import { useDeviceActions } from '../hooks/useDeviceActions';

export default function DeviceActions() {
  const { state, refresh } = useDevices();
  const actions = useDeviceActions();
  const [password, setPassword] = useState('');
  const [username, setUsername] = useState('');
  const [busy, setBusy] = useState(false);

  const device = state.devices.find((d) => d.device_id === state.selectedId);

  // Clear password & username on selection change.
  useEffect(() => {
    setPassword('');
    setUsername('');
  }, [state.selectedId]);

  if (!device) return null;

  const onRefresh = async () => {
    setBusy(true);
    try { await actions.refresh(); await refresh(); }
    finally { setBusy(false); }
  };

  const onUninstall = async () => {
    if (!password) { message.warning('Enter SSH password first'); return; }
    setBusy(true);
    try {
      await actions.uninstall(device.device_id, username || device.username, password);
      message.success(`Uninstalled ${device.ip}`);
      setPassword('');
      await refresh();
    } catch (e: unknown) {
      message.error(`Uninstall failed: ${e}`);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div style={{ borderTop: '1px solid #303030', padding: '12px 16px', background: '#0a0a0a' }}>
      <div style={{ marginBottom: 8, fontSize: 12, color: '#888', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
        Device actions
      </div>
      <Space.Compact style={{ width: '100%' }}>
        <Input.Password
          placeholder="SSH password (not persisted)"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
      </Space.Compact>
      <Space style={{ marginTop: 8, width: '100%', justifyContent: 'space-between' }}>
        <Button icon={<ReloadOutlined />} onClick={onRefresh} disabled={busy}>
          Refresh
        </Button>
        <Button danger icon={<DeleteOutlined />} onClick={onUninstall} loading={busy} disabled={!password}>
          Uninstall spotterd
        </Button>
      </Space>
    </div>
  );
}
