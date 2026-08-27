import { useState } from 'react';
import { Button, Modal, Space, Tooltip, message } from 'antd';
import { PoweroffOutlined, CloseCircleOutlined, ReloadOutlined } from '@ant-design/icons';
import { useDevices } from '../state/DeviceContext';
import { useI18n } from '../state/I18nContext';
import { useDeviceActions } from '../hooks/useDeviceActions';
import BasicCard from './BasicCard';
import NetworkCard from './NetworkCard';
import JetsonCard from './JetsonCard';
import EmptyState from './EmptyState';
import LogSection from './LogSection';
import PowerAuditList from './PowerAuditList';

type PowerAction = 'reboot' | 'shutdown';

export default function DetailPanel() {
  const { state, refresh } = useDevices();
  const { t } = useI18n();
  const actions = useDeviceActions();
  const [busyAction, setBusyAction] = useState<PowerAction | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  // Bumped every time the operator runs a power action, so
  // PowerAuditList refetches and shows the new entry without
  // a manual reload. The number itself is irrelevant; only
  // the change matters.
  const [auditRevision, setAuditRevision] = useState(0);
  const device = state.devices.find((d) => d.device_id === state.selectedId);
  const hostname = device?.last_info?.basic?.hostname || device?.ip || '';

  const onRefresh = async () => {
    if (!device) return;
    setRefreshing(true);
    try { await actions.refresh(); await refresh(); }
    finally { setRefreshing(false); }
  };

  const runPowerAction = (kind: PowerAction) => {
    if (!device) return;
    const titleKey =
      kind === 'reboot'
        ? 'detail.actions.power.reboot.confirmTitle'
        : 'detail.actions.power.shutdown.confirmTitle';
    const okKey =
      kind === 'reboot'
        ? 'detail.actions.power.reboot.confirmOk'
        : 'detail.actions.power.shutdown.confirmOk';
    const bodyKey =
      kind === 'shutdown'
        ? 'detail.actions.power.shutdown'
        : 'detail.actions.power.reboot';

    Modal.confirm({
      title: t(titleKey).replace('{hostname}', hostname),
      content: t(bodyKey),
      okText: t(okKey),
      okButtonProps: { danger: kind === 'shutdown' },
      cancelText: t('common.cancel'),
      onOk: async () => {
        setBusyAction(kind);
        try {
          if (kind === 'reboot') {
            await actions.reboot(device.device_id);
          } else {
            await actions.shutdown(device.device_id);
          }
          message.success(t('detail.actions.power.toast.success') ?? 'Command sent');
          setAuditRevision((r) => r + 1);
        } catch (e: unknown) {
          const err = e as { message?: string } | null;
          const msg = (err?.message ?? String(e)) as string;
          if (msg.includes('power actions disabled')) {
            message.error(t('detail.actions.power.disabledHint'));
          } else {
            message.error(msg);
          }
        } finally {
          setBusyAction(null);
        }
      },
    });
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <div style={{ flex: 1, padding: 16, overflow: 'auto' }}>
        {!device ? <EmptyState /> : (
          <>
            <div
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                marginBottom: 16,
              }}
            >
              <h2 style={{ margin: 0, color: 'var(--text-primary)' }}>
                {device.last_info?.basic?.hostname || device.ip}
                <span
                  style={{
                    marginLeft: 12,
                    fontSize: 13,
                    color: device.online ? '#52c41a' : '#b71c1c',
                  }}
                >
                  {device.online ? t('detail.status.online') : t('detail.status.offline')}
                </span>
              </h2>
              <Space>
                <Tooltip title={t('detail.refresh')}>
                  <Button
                    size="small"
                    icon={<ReloadOutlined />}
                    loading={refreshing}
                    onClick={onRefresh}
                  >
                    {t('detail.refresh')}
                  </Button>
                </Tooltip>
                <Button
                  size="small"
                  icon={<PoweroffOutlined />}
                  disabled={!device.online || busyAction !== null}
                  loading={busyAction === 'reboot'}
                  onClick={() => runPowerAction('reboot')}
                >
                  {t('detail.actions.power.reboot')}
                </Button>
                <Button
                  size="small"
                  danger
                  icon={<CloseCircleOutlined />}
                  disabled={!device.online || busyAction !== null}
                  loading={busyAction === 'shutdown'}
                  onClick={() => runPowerAction('shutdown')}
                >
                  {t('detail.actions.power.shutdown')}
                </Button>
              </Space>
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: '2fr 1fr', gap: 12 }}>
              <BasicCard device={device} />
              <JetsonCard device={device} />
            </div>
            <div style={{ marginTop: 12 }}>
              <NetworkCard device={device} />
            </div>
            <div
              style={{
                marginTop: 16,
                padding: 12,
                background: 'var(--bg-card)',
                border: '1px solid var(--border)',
                borderRadius: 4,
              }}
            >
              <h3 style={{
                margin: '0 0 8px 0',
                fontSize: 13,
                color: 'var(--text-secondary)',
                textTransform: 'uppercase',
                letterSpacing: '0.05em',
              }}>
                {t('detail.audit.title') || 'Power Activity'}
              </h3>
              <PowerAuditList deviceID={device.device_id} revision={auditRevision} limit={10} />
            </div>
          </>
        )}
      </div>
      {device && (
        <div
          style={{
            borderTop: '1px solid var(--border)',
            padding: '12px 16px',
            background: 'var(--bg-app)',
          }}
        >
          <LogSection deviceID={device.device_id} online={device.online} />
        </div>
      )}
    </div>
  );
}