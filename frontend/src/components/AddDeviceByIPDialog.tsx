import { useState } from 'react';
import { Modal, Form, Input, InputNumber, Button, Space, Alert, Typography } from 'antd';
import { useMenu } from '../state/MenuContext';
import { useDevices } from '../state/DeviceContext';
import { useDeviceActions } from '../hooks/useDeviceActions';

export default function AddDeviceByIPDialog() {
  const { modal, closeModal } = useMenu();
  const { refresh } = useDevices();
  const actions = useDeviceActions();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  return (
    <Modal
      open={modal === 'add-device'}
      onCancel={closeModal}
      footer={null}
      title="Add device by IP"
      width={480}
      destroyOnClose
    >
      <Typography.Paragraph>
        Manually register a device when auto-discovery (multicast / subnet scan) is blocked.
      </Typography.Paragraph>

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
        <Form.Item label="IP" name="ip" rules={[{ required: true, pattern: /^(\d{1,3}\.){3}\d{1,3}$/, message: 'Invalid IPv4' }]}>
          <Input placeholder="10.10.9.165" autoFocus />
        </Form.Item>
        <Form.Item label="HTTP port" name="port" rules={[{ required: true, type: 'number', min: 1, max: 65535 }]}>
          <InputNumber min={1} max={65535} style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item label="Username" name="username" rules={[{ required: true }]}>
          <Input placeholder="optional label" />
        </Form.Item>
        <Space style={{ width: '100%', justifyContent: 'flex-end' }}>
          <Button onClick={closeModal}>Cancel</Button>
          <Button type="primary" htmlType="submit" loading={busy}>Add</Button>
        </Space>
      </Form>
    </Modal>
  );
}