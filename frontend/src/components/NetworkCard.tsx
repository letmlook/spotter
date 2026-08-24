import { Card } from 'antd';
import type { RegistryEntry } from '../state/DeviceContext';
import { useI18n } from '../state/I18nContext';

// Map a Linux interface name to a short label and accent tone used
// by the status dot + type chip. Keeps the row meaningful at a glance
// even when the operator doesn't recognize `enP2p1s0` / `mgbe1.0`
// directly. Unrecognized names fall back to a neutral `net` chip so
// the row still renders consistently instead of dropping the column.
function classifyIface(name: string): { type: string; tone: string } {
  const n = name.toLowerCase();
  if (n.startsWith('wl') || n.startsWith('wlan')) return { type: 'wifi',    tone: '#1677ff' };
  if (n.startsWith('usb'))                         return { type: 'usb',     tone: '#722ed1' };
  if (n.startsWith('can'))                         return { type: 'can',     tone: '#fa8c16' };
  if (n.startsWith('docker'))                      return { type: 'docker',  tone: '#13c2c2' };
  if (n.startsWith('br-') || n.startsWith('l4tbr') || n.endsWith('br0'))
    return { type: 'bridge', tone: '#52c41a' };
  if (n.startsWith('lo'))                          return { type: 'loop', tone: '#8c8c8c' };
  if (n.startsWith('en') || n.startsWith('eth') || n.startsWith('mgbe'))
    return { type: 'eth', tone: '#1677ff' };
  return { type: 'net', tone: '#8c8c8c' };
}

// True if the address string is an IPv6 literal — used to dim the
// link-local / SLAAC addresses so the routable IPv4 stays primary.
function isV6(addr: string): boolean {
  return addr.includes(':');
}

export default function NetworkCard({ device }: { device: RegistryEntry }) {
  const { t } = useI18n();
  const net = device.last_info?.network;
  const ifaces = net?.interfaces || [];

  // Split into active (has at least one address) and inactive (no
  // IPs). Inactive rows collapse to a single-line tag list so they
  // stop eating vertical space the operator actually needs.
  const active   = ifaces.filter((i) => (i.addrs ?? []).some((a) => !!a));
  const inactive = ifaces.filter((i) => !(i.addrs ?? []).some((a) => !!a));

  // Which interface owns the primary IP? Surfaced next to the hero
  // so the operator knows where the connection lives without having
  // to cross-reference the active list manually.
  const primaryIp = net?.primary_ip?.split('/')[0];
  const primaryIface = primaryIp
    ? ifaces.find((i) => (i.addrs ?? []).some((a) => a.split('/')[0] === primaryIp))
    : undefined;

  // ---- Style primitives (inline to match the rest of the card files).
  const row: React.CSSProperties = {
    display: 'grid',
    gridTemplateColumns: 'minmax(0, auto) minmax(0, 1fr)',
    columnGap: 14,
    alignItems: 'baseline',
    padding: '6px 0',
    borderBottom: '1px dashed var(--border)',
  };
  const rowLast: React.CSSProperties = { ...row, borderBottom: 'none' };

  const left: React.CSSProperties = {
    display: 'flex',
    alignItems: 'center',
    gap: 8,
    minWidth: 0,
  };
  const name: React.CSSProperties = {
    fontSize: 12,
    fontWeight: 600,
    color: 'var(--text-primary)',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
  };
  const chip: React.CSSProperties = {
    fontSize: 10,
    lineHeight: '14px',
    height: 14,
    padding: '0 6px',
    borderRadius: 3,
    color: 'var(--text-secondary)',
    background: 'var(--bg-hover)',
    whiteSpace: 'nowrap',
    flex: '0 0 auto',
  };
  const right: React.CSSProperties = {
    display: 'flex',
    flexDirection: 'column',
    gap: 2,
    minWidth: 0,
  };
  const mono: React.CSSProperties = {
    fontFamily: 'ui-monospace, Menlo, monospace',
    fontSize: 11,
    color: 'var(--text-secondary)',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
  };
  const addrMono = (isIpv6: boolean): React.CSSProperties => ({
    fontFamily: 'ui-monospace, Menlo, monospace',
    fontSize: 11,
    color: isIpv6 ? 'var(--text-secondary)' : 'var(--text-primary)',
    fontWeight: isIpv6 ? 400 : 500,
    whiteSpace: 'nowrap',
  });

  // Status dot — type-tinted when live, neutral gray when down.
  const dot = (tone: string, live: boolean): React.CSSProperties => ({
    width: 6,
    height: 6,
    borderRadius: '50%',
    background: live ? tone : 'var(--scrollbar-thumb)',
    flex: '0 0 auto',
  });

  // Group header — small uppercase eyebrow with a count chip.
  const groupHeader = (label: string, count: number): React.ReactNode => (
    <div style={{
      display: 'flex',
      alignItems: 'baseline',
      gap: 8,
      margin: '14px 0 4px',
      paddingBottom: 4,
      borderBottom: '1px solid var(--border)',
    }}>
      <span style={{
        fontSize: 10,
        fontWeight: 600,
        letterSpacing: 0.5,
        textTransform: 'uppercase',
        color: 'var(--text-muted)',
      }}>
        {label}
      </span>
      <span style={{ fontSize: 10, color: 'var(--text-secondary)' }}>
        {count}
      </span>
    </div>
  );

  const renderRow = (
    iface: NonNullable<typeof ifaces[number]>,
    isLast: boolean,
  ): React.ReactNode => {
    const addrs = (iface.addrs ?? []).filter((a) => !!a);
    const live = addrs.length > 0;
    const cls = classifyIface(iface.name || '');
    return (
      <div key={iface.name || 'unknown'} style={isLast ? rowLast : row}>
        <div style={left}>
          <span style={dot(cls.tone, live)} />
          <span style={name}>{iface.name || '—'}</span>
          <span style={chip}>{cls.type}</span>
        </div>
        <div style={right}>
          {iface.mac && <span style={mono}>{iface.mac}</span>}
          {addrs.length === 0 ? (
            <span style={{ ...mono, fontStyle: 'italic', opacity: 0.55 }}>—</span>
          ) : (
            <div style={{ display: 'flex', flexWrap: 'wrap', columnGap: 12, rowGap: 2 }}>
              {addrs.map((a, i) => (
                <span key={i} style={addrMono(isV6(a))}>{a}</span>
              ))}
            </div>
          )}
        </div>
      </div>
    );
  };

  // Hero label strips trailing colon (and full-width colon) so it
  // composes with the gap we control below, regardless of locale.
  const primaryLabel = (t('card.network.primary_ip') || '').replace(/[:：]\s*$/, '');

  return (
    <Card
      title={t('card.network.title')}
      size="small"
      styles={{ body: { padding: 12 } }}
    >
      {/* Hero — primary IP with a left accent bar and source label. */}
      {net?.primary_ip && (
        <div style={{
          display: 'flex',
          alignItems: 'baseline',
          gap: 12,
          padding: '6px 10px',
          background: 'var(--bg-hover)',
          borderLeft: '2px solid #1677ff',
          borderRadius: '0 4px 4px 0',
          marginBottom: 4,
          flexWrap: 'wrap',
        }}>
          <span style={{
            fontSize: 10,
            fontWeight: 600,
            letterSpacing: 0.5,
            textTransform: 'uppercase',
            color: 'var(--text-muted)',
            flex: '0 0 auto',
          }}>
            {primaryLabel}
          </span>
          <span style={{
            fontFamily: 'ui-monospace, Menlo, monospace',
            fontSize: 13,
            fontWeight: 600,
            color: 'var(--text-primary)',
            whiteSpace: 'nowrap',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
          }}>
            {net.primary_ip}
          </span>
          {primaryIface?.name && (
            <span style={{
              fontFamily: 'ui-monospace, Menlo, monospace',
              fontSize: 11,
              color: 'var(--text-secondary)',
              whiteSpace: 'nowrap',
              marginLeft: 'auto',
            }}>
              ▸ {primaryIface.name}
            </span>
          )}
        </div>
      )}

      {ifaces.length === 0 ? (
        <span style={{
          color: 'var(--text-secondary)',
          fontSize: 12,
          marginTop: 8,
          display: 'block',
        }}>
          {t('card.network.empty')}
        </span>
      ) : (
        <>
          {active.length > 0 && (
            <>
              {groupHeader(t('card.network.active'), active.length)}
              {active.map((iface, idx) =>
                renderRow(iface, idx === active.length - 1 && inactive.length === 0),
              )}
            </>
          )}
          {inactive.length > 0 && (
            <>
              {groupHeader(t('card.network.inactive'), inactive.length)}
              <div style={{
                display: 'flex',
                flexWrap: 'wrap',
                gap: 6,
                padding: '4px 0 2px',
                opacity: 0.6,
              }}>
                {inactive.map((iface) => (
                  <span key={iface.name} style={{
                    fontFamily: 'ui-monospace, Menlo, monospace',
                    fontSize: 11,
                    padding: '2px 8px',
                    borderRadius: 10,
                    background: 'var(--bg-hover)',
                    color: 'var(--text-secondary)',
                    whiteSpace: 'nowrap',
                  }}>
                    {iface.name}
                  </span>
                ))}
              </div>
            </>
          )}
        </>
      )}
    </Card>
  );
}
