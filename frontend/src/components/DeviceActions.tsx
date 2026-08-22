import { useState } from 'react';
import { Button } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { useDevices } from '../state/DeviceContext';
import { useDeviceActions } from '../hooks/useDeviceActions';
import { useI18n } from '../state/I18nContext';

export default function DeviceActions() {
  const { state, refresh } = useDevices();
  const actions = useDeviceActions();
  const { t } = useI18n();
  const [busy, setBusy] = useState(false);
  const device = state.devices.find((d) => d.device_id === state.selectedId);
  if (!device) return null;

  const onRefresh = async () => {
    setBusy(true);
    try { await actions.refresh(); await refresh(); }
    finally { setBusy(false); }
  };

  return (
    <div style={{ borderTop: '1px solid var(--border)', padding: '12px 16px', background: 'var(--bg-app)' }}>
      <div style={{ marginBottom: 8, fontSize: 12, color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
        {t('detail.actions')}
      </div>
      <Button icon={<ReloadOutlined />} onClick={onRefresh} loading={busy} block>
        {t('detail.refresh')}
      </Button>
    </div>
  );
}