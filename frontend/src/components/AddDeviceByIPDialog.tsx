import { useState } from 'react';
import { Modal, Form, Input, InputNumber, Button, Space, Alert, Typography } from 'antd';
import { useMenu } from '../state/MenuContext';
import { useDevices } from '../state/DeviceContext';
import { useDeviceActions } from '../hooks/useDeviceActions';
import { useI18n } from '../state/I18nContext';

export default function AddDeviceByIPDialog() {
  const { modal, closeModal } = useMenu();
  const { refresh } = useDevices();
  const actions = useDeviceActions();
  const { t } = useI18n();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  return (
    <Modal
      open={modal === 'add-device'}
      onCancel={closeModal}
      footer={null}
      title={t('modal.add.title')}
      width={480}
      destroyOnClose
    >
      <Typography.Paragraph>{t('modal.add.body')}</Typography.Paragraph>

      {error && <Alert type="error" message={error} closable onClose={() => setError(null)} style={{ marginBottom: 12 }} />}

      <Form
        layout="vertical" size="small"
        initialValues={{ port: 9999 }}
        disabled={busy}
        onFinish={async (vals: { ip: string; port: number; username: string }) => {
          setBusy(true); setError(null);
          try {
            await actions.add(vals.ip, vals.port, vals.username);
            await refresh();
            closeModal();
          } catch (e: unknown) {
            setError(String(e));
          } finally { setBusy(false); }
        }}
      >
        <Form.Item label={t('modal.add.ip')} name="ip" rules={[{ required: true, pattern: /^(\d{1,3}\.){3}\d{1,3}$/, message: 'Invalid IPv4' }]}>
          <Input placeholder="10.10.9.165" autoFocus />
        </Form.Item>
        <Form.Item label={t('modal.add.port')} name="port" rules={[{ required: true, type: 'number', min: 1, max: 65535 }]}>
          <InputNumber min={1} max={65535} style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item label={t('modal.add.username')} name="username" rules={[{ required: true }]}>
          <Input placeholder={t('modal.add.username.placeholder')} />
        </Form.Item>
        <Space style={{ width: '100%', justifyContent: 'flex-end' }}>
          <Button onClick={closeModal}>{t('modal.add.cancel')}</Button>
          <Button type="primary" htmlType="submit" loading={busy}>{t('modal.add.submit')}</Button>
        </Space>
      </Form>
    </Modal>
  );
}