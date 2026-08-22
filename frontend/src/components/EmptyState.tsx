import { Empty, Typography, Button } from 'antd';
import { BookOutlined } from '@ant-design/icons';
import { useMenu } from '../state/MenuContext';

export default function EmptyState() {
  const { openModal } = useMenu();
  return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%' }}>
      <Empty
        image={Empty.PRESENTED_IMAGE_SIMPLE}
        description="No device selected."
      >
        <Typography.Paragraph type="secondary" style={{ marginBottom: 16 }}>
          Use <strong>Tools → Scan local subnet</strong> to discover devices
          on your LAN, or <strong>Tools → Add device by IP</strong> to register one manually.
          <br />
          Devices already running spotterd are also discovered automatically via multicast.
        </Typography.Paragraph>
        <Button
          icon={<BookOutlined />}
          onClick={() => openModal('setup-guide')}
        >
          How to install spotterd on a device
        </Button>
      </Empty>
    </div>
  );
}