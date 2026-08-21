import { Empty, Button, Space } from 'antd';
import { PlusOutlined, ScanOutlined, ImportOutlined } from '@ant-design/icons';

export default function EmptyState({ onAction }: { onAction: (which: 'deploy' | 'scan' | 'add') => void }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%' }}>
      <Empty
        image={Empty.PRESENTED_IMAGE_SIMPLE}
        description="No device selected. Use the toolbar to discover or add one."
      >
        <Space>
          <Button icon={<PlusOutlined />} onClick={() => onAction('deploy')}>Deploy</Button>
          <Button icon={<ScanOutlined />} onClick={() => onAction('scan')}>Scan subnet</Button>
          <Button icon={<ImportOutlined />} onClick={() => onAction('add')}>Add by IP</Button>
        </Space>
      </Empty>
    </div>
  );
}