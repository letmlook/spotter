import { Card, Descriptions } from 'antd';
import type { RegistryEntry } from '../state/DeviceContext';
import { formatUptime } from '../utils/format';
import { useI18n } from '../state/I18nContext';

export default function BasicCard({ device }: { device: RegistryEntry }) {
  const { t } = useI18n();
  const info = device.last_info;
  if (!info) {
    return (
      <Card title={t('card.basic.title')} size="small">
        <span style={{ color: 'var(--text-secondary)' }}>
          {device.online ? t('card.basic.polling') : t('card.basic.no_info')}
        </span>
      </Card>
    );
  }
  const b = info.basic || {};
  const os = b.os || {};
  const items = [
    { key: 'h', label: t('card.basic.hostname'), children: b.hostname || '—' },
    { key: 'u', label: t('card.basic.username'), children: b.username || '—' },
    { key: 'os', label: t('card.basic.os'), children: os.pretty_name || '—' },
    { key: 'dist', label: t('card.basic.distribution'), children: (os.id ? `${os.id} ${os.version_id || ''}`.trim() : '—') },
    { key: 'k', label: t('card.basic.kernel'), children: b.kernel || '—' },
    { key: 'a', label: t('card.basic.arch'), children: b.arch || '—' },
    { key: 'up', label: t('card.basic.uptime'), children: formatUptime(b.uptime_seconds) || '—' },
    { key: 'c', label: t('card.basic.collected_at'), children: info.collected_at || '—' },
    { key: 'v', label: t('card.basic.agent_version'), children: info.agent_version || '—' },
    { key: 'id', label: t('card.basic.device_id'), children: info.device_id || '—' },
  ];
  return (
    <Card title={t('card.basic.title')} size="small">
      <Descriptions column={2} size="small" colon={false} labelStyle={{ color: 'var(--text-secondary)', width: 110 }}>
        {items.map((it) => <Descriptions.Item key={it.key} label={it.label}>{it.children}</Descriptions.Item>)}
      </Descriptions>
    </Card>
  );
}