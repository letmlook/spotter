import { Card, Descriptions } from 'antd';
import type { RegistryEntry } from '../state/DeviceContext';
import { useI18n } from '../state/I18nContext';

export default function JetsonCard({ device }: { device: RegistryEntry }) {
  const { t } = useI18n();
  const j = device.last_info?.jetson;
  if (!j) {
    return (
      <Card title={t('card.jetson.title')} size="small">
        <span style={{ color: 'var(--text-secondary)', fontStyle: 'italic' }}>{t('card.jetson.empty')}</span>
      </Card>
    );
  }
  const items = [
    { key: 'm', label: t('card.jetson.model'), v: j.model },
    { key: 'j', label: t('card.jetson.jetpack'), v: j.jetpack },
    { key: 'l', label: t('card.jetson.l4t'), v: j.l4t },
    { key: 'c', label: t('card.jetson.cuda'), v: j.cuda },
    { key: 'd', label: t('card.jetson.cudnn'), v: j.cudnn },
    { key: 't', label: t('card.jetson.tensorrt'), v: j.tensorrt },
    { key: 'p', label: t('card.jetson.python'), v: j.python },
    { key: 's', label: t('card.jetson.serial'), v: j.serial },
  ].filter((it) => it.v);
  if (items.length === 0) {
    return (
      <Card title={t('card.jetson.title')} size="small">
        <span style={{ color: 'var(--text-secondary)', fontStyle: 'italic' }}>{t('card.jetson.probe_failed')}</span>
      </Card>
    );
  }
  return (
    <Card title={t('card.jetson.title')} size="small">
      <Descriptions column={1} size="small" colon={false} labelStyle={{ color: 'var(--text-secondary)', width: 130 }}>
        {items.map((it) => <Descriptions.Item key={it.key} label={it.label}>{it.v}</Descriptions.Item>)}
      </Descriptions>
    </Card>
  );
}