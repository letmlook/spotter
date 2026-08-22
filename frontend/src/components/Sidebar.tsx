import { Button, Tooltip, Popconfirm, Space } from 'antd';
import { ScanOutlined, DeleteOutlined } from '@ant-design/icons';
import { ClearRegistry } from '../../wailsjs/go/main/App';
import { useMenu } from '../state/MenuContext';
import { useDevices } from '../state/DeviceContext';
import { notification } from 'antd';
import DeviceList from './DeviceList';

export default function Sidebar() {
  const { state, refresh } = useDevices();
  const { triggerScan } = useMenu();

  const onQuickScan = async () => {
    try {
      await triggerScan();
      await refresh();
      notification.success({ message: 'Scan complete', placement: 'bottomRight', duration: 2 });
    } catch (e) {
      notification.error({ message: 'Scan failed', description: String(e), placement: 'bottomRight' });
    }
  };

  return (
    <aside
      style={{
        width: 260, flexShrink: 0,
        background: '#0a0a0a', borderRight: '1px solid #303030',
        display: 'flex', flexDirection: 'column',
      }}
    >
      <div style={{
        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
        padding: '10px 12px', borderBottom: '1px solid #303030',
      }}>
        <span style={{ color: '#888', fontSize: 12, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
          Devices
        </span>
        <Space size={4}>
          <Tooltip title="Scan local subnet (Tools → Scan local subnet)">
            <Button
              size="small"
              icon={<ScanOutlined />}
              onClick={onQuickScan}
              aria-label="Scan local subnet"
            />
          </Tooltip>
          <Popconfirm
            title="Clear registry"
            description={`Remove all ${state.devices.length} device(s) from the local registry?`}
            okText="Clear"
            cancelText="Cancel"
            onConfirm={async () => { await ClearRegistry(); await refresh(); }}
            disabled={state.devices.length === 0}
          >
            <Tooltip title="Clear registry">
              <Button
                size="small"
                danger
                icon={<DeleteOutlined />}
                disabled={state.devices.length === 0}
                aria-label="Clear registry"
              />
            </Tooltip>
          </Popconfirm>
        </Space>
      </div>
      <DeviceList />
    </aside>
  );
}