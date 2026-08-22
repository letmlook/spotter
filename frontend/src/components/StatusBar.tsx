import { useDevices } from '../state/DeviceContext';
import { useI18n } from '../state/I18nContext';

export default function StatusBar() {
  const { state } = useDevices();
  const { t } = useI18n();
  const online = state.devices.filter((d) => d.online).length;
  const total = state.devices.length;
  return (
    <div
      style={{
        height: 24, flexShrink: 0,
        padding: '0 16px',
        display: 'flex', alignItems: 'center',
        background: 'var(--bg-app)', borderTop: '1px solid var(--border)',
        fontSize: 12, color: 'var(--text-secondary)',
      }}
    >
      {t('status.count', { online: String(online), total: String(total) })}
      {state.loading && <span style={{ marginLeft: 12 }}>{t('status.refreshing')}</span>}
    </div>
  );
}