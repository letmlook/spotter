import { Card, Table } from 'antd';
import type { RegistryEntry } from '../state/DeviceContext';

export default function NetworkCard({ device }: { device: RegistryEntry }) {
  const net = device.last_info?.network;
  const ifaces = net?.interfaces || [];
  return (
    <Card title="Network" size="small">
      {net?.primary_ip && (
        <div style={{ marginBottom: 12, color: '#ccc' }}>
          Primary IP:&nbsp;<strong style={{ color: '#fff' }}>{net.primary_ip}</strong>
        </div>
      )}
      {ifaces.length === 0 ? (
        <span style={{ color: '#888' }}>No network interfaces reported</span>
      ) : (
        <Table<{ name?: string; mac?: string; addrs?: string[] }>
          rowKey={(r) => r.name || Math.random().toString()}
          dataSource={ifaces}
          size="small"
          pagination={false}
          columns={[
            { title: 'Interface', dataIndex: 'name' },
            { title: 'MAC', dataIndex: 'mac' },
            {
              title: 'Addresses',
              dataIndex: 'addrs',
              render: (a?: string[]) => (a && a.length > 0 ? a.join(', ') : '—'),
            },
          ]}
        />
      )}
    </Card>
  );
}