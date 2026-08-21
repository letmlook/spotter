import { Button, Popconfirm } from 'antd';
import { DeleteOutlined } from '@ant-design/icons';
import { ClearRegistry } from '../../wailsjs/go/main/App';
import { useDevices } from '../state/DeviceContext';
import DeviceList from './DeviceList';

export default function Sidebar() {
  const { state, refresh } = useDevices();

  return (
    <aside
      style={{
        width: 260, flexShrink: 0,
        background: '#0a0a0a', borderRight: '1px solid #303030',
        display: 'flex', flexDirection: 'column',
      }}
    >
      <div style={{ padding: '8px 12px', borderBottom: '1px solid #303030' }}>
        <Popconfirm
          title="Clear registry"
          description={`Remove all ${state.devices.length} device(s) from the local registry? This does NOT touch remote devices — use Uninstall for that.`}
          okText="Clear"
          cancelText="Cancel"
          onConfirm={async () => { await ClearRegistry(); await refresh(); }}
          disabled={state.devices.length === 0}
        >
          <Button
            danger
            icon={<DeleteOutlined />}
            block
            disabled={state.devices.length === 0}
          >
            Clear registry
          </Button>
        </Popconfirm>
      </div>
      <DeviceList />
    </aside>
  );
}
