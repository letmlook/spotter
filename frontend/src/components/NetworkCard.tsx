import { Card } from 'antd';
import type { RegistryEntry } from '../state/DeviceContext';
import { useI18n } from '../state/I18nContext';

export default function NetworkCard({ device }: { device: RegistryEntry }) {
  const { t } = useI18n();
  const net = device.last_info?.network;
  const ifaces = net?.interfaces || [];
  const row: React.CSSProperties = {
    display: 'flex',
    alignItems: 'baseline',
    gap: 8,
    padding: '4px 0',
    borderBottom: '1px dashed var(--border)',
    fontSize: 12,
  };
  const rowLast: React.CSSProperties = { ...row, borderBottom: 'none' };
  const name: React.CSSProperties = {
    color: 'var(--text-primary)',
    fontWeight: 600,
    width: 120,
    flex: '0 0 auto',
  };
  const mac: React.CSSProperties = {
    color: 'var(--text-secondary)',
    fontFamily: 'ui-monospace, Menlo, monospace',
    fontSize: 11,
    width: 160,
    flex: '0 0 auto',
  };
  return (
    <Card
      title={t('card.network.title')}
      size="small"
      styles={{ body: { padding: 12 } }}
    >
      {net?.primary_ip && (
        <div style={{
          marginBottom: 10,
          paddingBottom: 8,
          borderBottom: '1px solid var(--border)',
          fontSize: 13,
        }}>
          <span style={{ color: 'var(--text-secondary)', marginRight: 8 }}>{t('card.network.primary_ip')}</span>
          <strong style={{ color: 'var(--text-primary)' }}>{net.primary_ip}</strong>
        </div>
      )}
      {ifaces.length === 0 ? (
        <span style={{ color: 'var(--text-secondary)', fontSize: 12 }}>{t('card.network.empty')}</span>
      ) : (
        <div>
          {ifaces.map((iface, idx) => {
            const isLast = idx === ifaces.length - 1;
            const addrs = (iface.addrs ?? []).filter((a) => !!a);
            return (
              <div key={iface.name || idx} style={isLast ? rowLast : row}>
                <div style={{
                  display: 'flex',
                  flexDirection: 'column',
                  minWidth: 280,
                  flex: '0 0 auto',
                  gap: 2,
                }}>
                  <span style={name}>{iface.name || '—'}</span>
                  {iface.mac && (
                    <span style={{ ...mac, width: 'auto' }}>{iface.mac}</span>
                  )}
                </div>
                <div style={{
                  display: 'flex',
                  flexDirection: 'column',
                  flex: '1 1 auto',
                  fontFamily: 'ui-monospace, Menlo, monospace',
                  fontSize: 11,
                  color: 'var(--text-primary)',
                  gap: 2,
                }}>
                  {addrs.length === 0 ? (
                    <span style={{ color: 'var(--text-secondary)' }}>—</span>
                  ) : (
                    addrs.map((a, i) => (
                      <span key={i} style={{ whiteSpace: 'nowrap' }}>{a}</span>
                    ))
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </Card>
  );
}
