import { Card, Descriptions } from 'antd';
import type { RegistryEntry } from '../state/DeviceContext';
import { formatUptime } from '../utils/format';

export default function BasicCard({ device }: { device: RegistryEntry }) {
  const info = device.last_info;
  if (!info) {
    return (
      <Card title="Basic" size="small">
        <span style={{ color: '#888' }}>{device.online ? 'Polling…' : 'No info yet'}</span>
      </Card>
    );
  }
  const b = info.basic || {};
  const os = b.os || {};
  const items = [
    { key: 'h', label: 'Hostname', children: b.hostname || '—' },
    { key: 'u', label: 'Username', children: b.username || '—' },
    { key: 'os', label: 'OS', children: os.pretty_name || '—' },
    { key: 'dist', label: 'Distribution', children: (os.id ? `${os.id} ${os.version_id || ''}`.trim() : '—') },
    { key: 'k', label: 'Kernel', children: b.kernel || '—' },
    { key: 'a', label: 'Arch', children: b.arch || '—' },
    { key: 'up', label: 'Uptime', children: formatUptime(b.uptime_seconds) || '—' },
    { key: 'c', label: 'Collected at', children: info.collected_at || '—' },
    { key: 'v', label: 'Agent version', children: info.agent_version || '—' },
    { key: 'id', label: 'Device ID', children: info.device_id || '—' },
  ];
  return (
    <Card title="Basic" size="small">
      <Descriptions column={1} size="small" colon={false} labelStyle={{ color: '#888', width: 130 }}>
        {items.map((it) => <Descriptions.Item key={it.key} label={it.label}>{it.children}</Descriptions.Item>)}
      </Descriptions>
    </Card>
  );
}