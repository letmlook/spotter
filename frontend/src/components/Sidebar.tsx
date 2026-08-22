import { Button, Tooltip, Popconfirm, Space, notification } from 'antd';
import { ScanOutlined, DeleteOutlined } from '@ant-design/icons';
import { ClearRegistry } from '../../wailsjs/go/main/App';
import { useMenu } from '../state/MenuContext';
import { useDevices } from '../state/DeviceContext';
import { useI18n } from '../state/I18nContext';
import DeviceList from './DeviceList';

export default function Sidebar() {
  const { state, refresh } = useDevices();
  const { triggerScan } = useMenu();
  const { t } = useI18n();

  const onQuickScan = async () => {
    try {
      await triggerScan();
      await refresh();
      notification.success({ message: t('notif.scan.done'), placement: 'bottomRight', duration: 2 });
    } catch (e) {
      notification.error({ message: t('notif.scan.fail'), description: String(e), placement: 'bottomRight' });
    }
  };

  return (
    <aside
      style={{
        width: 260, flexShrink: 0,
        background: 'var(--bg-app)', borderRight: '1px solid var(--border)',
        display: 'flex', flexDirection: 'column',
      }}
    >
      <div style={{
        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
        padding: '10px 12px', borderBottom: '1px solid var(--border)',
      }}>
        <span style={{ color: 'var(--text-secondary)', fontSize: 12, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
          {t('sidebar.devices')}
        </span>
        <Space size={4}>
          <Tooltip title={t('sidebar.scan')}>
            <Button
              size="small"
              icon={<ScanOutlined />}
              onClick={onQuickScan}
              aria-label={t('sidebar.scan')}
            />
          </Tooltip>
          <Popconfirm
            title={t('notif.clear.confirm.title')}
            description={t('notif.clear.confirm.body')}
            okText={t('notif.clear.confirm.ok')}
            cancelText={t('common.cancel')}
            onConfirm={async () => { await ClearRegistry(); await refresh(); }}
            disabled={state.devices.length === 0}
          >
            <Tooltip title={t('sidebar.clear')}>
              <Button
                size="small"
                danger
                icon={<DeleteOutlined />}
                disabled={state.devices.length === 0}
                aria-label={t('sidebar.clear')}
              />
            </Tooltip>
          </Popconfirm>
        </Space>
      </div>
      <DeviceList />
    </aside>
  );
}