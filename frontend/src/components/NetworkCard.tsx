import { Card, Table } from 'antd';
import type { RegistryEntry } from '../state/DeviceContext';
import { useI18n } from '../state/I18nContext';

export default function NetworkCard({ device }: { device: RegistryEntry }) {
  const { t } = useI18n();
  const net = device.last_info?.network;
  const ifaces = net?.interfaces || [];
  return (
    <Card title={t('card.network.title')} size="small">
      {net?.primary_ip && (
        <div style={{ marginBottom: 12, color: 'var(--text-secondary)' }}>
          {t('card.network.primary_ip')}&nbsp;<strong style={{ color: 'var(--text-primary)' }}>{net.primary_ip}</strong>
        </div>
      )}
      {ifaces.length === 0 ? (
        <span style={{ color: 'var(--text-secondary)' }}>{t('card.network.empty')}</span>
      ) : (
        <Table<{ name?: string; mac?: string; addrs?: string[] }>
          rowKey={(r) => r.name || Math.random().toString()}
          dataSource={ifaces}
          size="small"
          pagination={false}
          columns={[
            { title: t('card.network.interface'), dataIndex: 'name' },
            { title: t('card.network.mac'), dataIndex: 'mac' },
            {
              title: t('card.network.addresses'),
              dataIndex: 'addrs',
              render: (a?: string[]) => (a && a.length > 0 ? a.join(', ') : '—'),
            },
          ]}
        />
      )}
    </Card>
  );
}