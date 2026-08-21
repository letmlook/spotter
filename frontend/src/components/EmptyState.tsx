import { Empty, Typography } from 'antd';

export default function EmptyState() {
  return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%' }}>
      <Empty
        image={Empty.PRESENTED_IMAGE_SIMPLE}
        description="No device selected. Use the sidebar toolbar to deploy, scan a subnet, or add by IP."
      >
        <Typography.Text type="secondary">
          Discovered devices from multicast or subnet scans are added automatically.
        </Typography.Text>
      </Empty>
    </div>
  );
}
