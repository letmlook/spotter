import { useState } from 'react';
import { Empty, Typography, Button, Space } from 'antd';
import { BookOutlined } from '@ant-design/icons';
import DeviceSetupGuide from './DeviceSetupGuide';

export default function EmptyState() {
  const [guideOpen, setGuideOpen] = useState(false);

  return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%' }}>
      <Empty
        image={Empty.PRESENTED_IMAGE_SIMPLE}
        description="No device selected. Use the sidebar Scan / Add buttons."
      >
        <Space direction="vertical" size={12} align="center">
          <Typography.Text type="secondary">
            Scan auto-detects your local subnet. Add by IP for manual entry.
            <br />
            Devices already running spotterd are also discovered via multicast.
          </Typography.Text>
          <Button
            icon={<BookOutlined />}
            onClick={() => setGuideOpen(true)}
          >
            How to install spotterd on a device
          </Button>
        </Space>
      </Empty>
      <DeviceSetupGuide open={guideOpen} onClose={() => setGuideOpen(false)} />
    </div>
  );
}