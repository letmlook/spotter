import { useState } from 'react';
import { Button, Modal, Space, message } from 'antd';
import { PoweroffOutlined, CloseCircleOutlined } from '@ant-design/icons';
import { useDevices } from '../state/DeviceContext';
import { useI18n } from '../state/I18nContext';
import { useDeviceActions } from '../hooks/useDeviceActions';
import BasicCard from './BasicCard';
import NetworkCard from './NetworkCard';
import JetsonCard from './JetsonCard';
import EmptyState from './EmptyState';
import DeviceActions from './DeviceActions';

type PowerAction = 'reboot' | 'shutdown';

export default function DetailPanel() {
  const { state } = useDevices();
  const { t } = useI18n();
  const actions = useDeviceActions();
  const [busyAction, setBusyAction] = useState<PowerAction | null>(null);
  const device = state.devices.find((d) => d.device_id === state.selectedId);
  const hostname = device?.last_info?.basic?.hostname || device?.ip || '';

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
            <div style={{ display: 'grid', gap: 12 }}>
              <BasicCard device={device} />
              <NetworkCard device={device} />
              <JetsonCard device={device} />
            </div>
          </>
        )}
      </div>
      {device && <DeviceActions />}
    </div>
  );
}