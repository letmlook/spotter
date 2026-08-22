import { Card } from 'antd';
import type { RegistryEntry } from '../state/DeviceContext';
import { useI18n } from '../state/I18nContext';

// JetsonCard lists all known Jetson-side fields even when the host
// is not a Jetson. Empty values render as "未安装" so the operator
// can confirm at a glance which probes succeeded vs which are absent
// because the device is a generic Linux box.
const FIELDS: { key: keyof NonNullable<NonNullable<RegistryEntry['last_info']>['jetson']>; labelKey: string }[] = [
  { key: 'model',    labelKey: 'card.jetson.model' },
  { key: 'jetpack',  labelKey: 'card.jetson.jetpack' },
  { key: 'l4t',      labelKey: 'card.jetson.l4t' },
  { key: 'cuda',     labelKey: 'card.jetson.cuda' },
  { key: 'cudnn',    labelKey: 'card.jetson.cudnn' },
  { key: 'tensorrt', labelKey: 'card.jetson.tensorrt' },
  { key: 'python',   labelKey: 'card.jetson.python' },
  { key: 'serial',   labelKey: 'card.jetson.serial' },
];

export default function JetsonCard({ device }: { device: RegistryEntry }) {
  const { t } = useI18n();
  const j = device.last_info?.jetson;

  const cell: React.CSSProperties = {
    display: 'flex',
    alignItems: 'baseline',
    gap: 8,
    minWidth: 0,
    fontSize: 12,
    lineHeight: '20px',
  };
  const label: React.CSSProperties = {
    color: 'var(--text-secondary)',
    flex: '0 0 auto',
    whiteSpace: 'nowrap',
  };
  const value: React.CSSProperties = {
    color: 'var(--text-primary)',
    fontWeight: 500,
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    flex: '1 1 auto',
    minWidth: 0,
  };
  const empty: React.CSSProperties = { ...value, color: 'var(--text-secondary)', fontStyle: 'italic' };

  // Whole-card empty state — only when last_info hasn't arrived.
  if (device.last_info && !j) {
    return (
      <Card title={t('card.jetson.title')} size="small" styles={{ body: { padding: 12 } }}>
        <span style={{ color: 'var(--text-secondary)', fontStyle: 'italic' }}>
          {t('card.jetson.not_jetson')}
        </span>
      </Card>
    );
  }

  return (
    <Card
      title={t('card.jetson.title')}
      size="small"
      styles={{ body: { padding: 12 } }}
    >
      <div style={{
        display: 'grid',
        gridTemplateColumns: '1fr',
        rowGap: 4,
      }}>
        {FIELDS.map(({ key, labelKey }) => {
          const v = j?.[key];
          const present = typeof v === 'string' && v.length > 0;
          return (
            <div key={key} style={cell}>
              <span style={label}>{t(labelKey)}</span>
              <span style={present ? value : empty}>
                {present ? v : t('card.jetson.not_installed')}
              </span>
            </div>
          );
        })}
      </div>
    </Card>
  );
}
