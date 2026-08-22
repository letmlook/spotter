import { Card } from 'antd';
import type { RegistryEntry } from '../state/DeviceContext';
import { formatUptime } from '../utils/format';
import { useI18n } from '../state/I18nContext';

export default function BasicCard({ device }: { device: RegistryEntry }) {
  const { t } = useI18n();
  const info = device.last_info;
  if (!info) {
    return (
      <Card title={t('card.basic.title')} size="small">
        <span style={{ color: 'var(--text-secondary)', fontSize: 12 }}>
          {device.online ? t('card.basic.polling') : t('card.basic.no_info')}
        </span>
      </Card>
    );
  }
  const b = info.basic || {};
  const os = b.os || {};

  // We render two rows at the top (hostname + dist) and a compact
  // 2-column key/value grid below. Avoid line-wrap on values by
  // using `white-space: nowrap` + ellipsis, and putting the long
  // device_id on its own row with a fixed-width monospace block.
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
  const dist = (os.id ? `${os.id} ${os.version_id || ''}`.trim() : '') || os.pretty_name || '—';

  return (
    <Card
      title={t('card.basic.title')}
      size="small"
      styles={{ body: { padding: 12 } }}
    >
      <div style={{
        display: 'flex',
        alignItems: 'baseline',
        gap: 8,
        marginBottom: 10,
        paddingBottom: 8,
        borderBottom: '1px solid var(--border)',
      }}>
        <span style={{ fontSize: 18, fontWeight: 600, color: 'var(--text-primary)' }}>
          {b.hostname || '—'}
        </span>
        <span style={{
          fontSize: 11,
          color: 'var(--text-secondary)',
          padding: '1px 8px',
          background: 'var(--bg-hover)',
          borderRadius: 10,
          whiteSpace: 'nowrap',
        }}>
          {dist}
        </span>
      </div>
      <div style={{
        display: 'grid',
        gridTemplateColumns: '1fr 1fr',
        columnGap: 16,
        rowGap: 4,
      }}>
        <div style={cell}>
          <span style={label}>{t('card.basic.username')}</span>
          <span style={value}>{b.username || '—'}</span>
        </div>
        <div style={cell}>
          <span style={label}>{t('card.basic.arch')}</span>
          <span style={value}>{b.arch || '—'}</span>
        </div>
        <div style={cell}>
          <span style={label}>{t('card.basic.kernel')}</span>
          <span style={{ ...value, fontFamily: 'ui-monospace, Menlo, monospace', fontSize: 11 }}>
            {b.kernel || '—'}
          </span>
        </div>
        <div style={cell}>
          <span style={label}>{t('card.basic.uptime')}</span>
          <span style={value}>{formatUptime(b.uptime_seconds) || '—'}</span>
        </div>
        <div style={cell}>
          <span style={label}>{t('card.basic.os')}</span>
          <span style={value}>{os.pretty_name || '—'}</span>
        </div>
        <div style={cell}>
          <span style={label}>{t('card.basic.collected_at')}</span>
          <span style={value}>{info.collected_at || '—'}</span>
        </div>
      </div>
      <div style={{ ...cell, marginTop: 10, paddingTop: 8, borderTop: '1px solid var(--border)' }}>
        <span style={label}>{t('card.basic.device_id')}</span>
        <code style={{
          ...value,
          fontFamily: 'ui-monospace, Menlo, monospace',
          fontSize: 11,
          background: 'var(--bg-hover)',
          padding: '1px 6px',
          borderRadius: 3,
        }}>
          {info.device_id || '—'}
        </code>
      </div>
      <div style={{ ...cell, marginTop: 4 }}>
        <span style={label}>{t('card.basic.agent_version')}</span>
        <span style={value}>{info.agent_version || '—'}</span>
      </div>
    </Card>
  );
}
