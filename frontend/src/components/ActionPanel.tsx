import { useState } from 'react';
import { Button, Form, Input, InputNumber, Space, Alert } from 'antd';
import { ScanOutlined, ImportOutlined, CloseOutlined } from '@ant-design/icons';
import { useDeviceActions } from '../hooks/useDeviceActions';
import { useDevices } from '../state/DeviceContext';

type ActiveForm = null | 'scan' | 'add';

export default function ActionPanel() {
  const [active, setActive] = useState<ActiveForm>(null);
  const [error, setError] = useState<string | null>(null);
  const { refresh } = useDevices();
  const actions = useDeviceActions();
  const [busy, setBusy] = useState(false);

  const close = () => { setActive(null); setError(null); };

  return (
    <div style={{ borderBottom: '1px solid #303030' }}>
      <Space.Compact block>
        <Button
          type={active === 'scan' ? 'primary' : 'default'}
          icon={<ScanOutlined />}
          block
          onClick={() => setActive(active === 'scan' ? null : 'scan')}
        >
          Scan
        </Button>
        <Button
          type={active === 'add' ? 'primary' : 'default'}
          icon={<ImportOutlined />}
          onClick={() => setActive(active === 'add' ? null : 'add')}
        >
          Add
        </Button>
      </Space.Compact>

      {error && <Alert type="error" message={error} closable onClose={() => setError(null)} style={{ margin: 8 }} />}

      {active === 'scan' && (
        <Form
          layout="vertical" size="small" disabled={busy}
          onFinish={async (vals: { cidr: string }) => {
            setBusy(true); setError(null);
            try {
              await actions.scan(vals.cidr);
              await refresh();
              close();
            } catch (e: unknown) { setError(String(e)); }
            finally { setBusy(false); }
          }}
          style={{ padding: 12 }}
        >
          <Form.Item label="CIDR" name="cidr" rules={[{ required: true, pattern: /^(\d{1,3}\.){3}\d{1,3}\/\d{1,2}$/, message: 'Invalid CIDR' }]}>
            <Input placeholder="192.168.1.0/24" autoFocus />
          </Form.Item>
          <Space style={{ width: '100%', justifyContent: 'flex-end' }}>
            <Button onClick={close} icon={<CloseOutlined />}>Cancel</Button>
            <Button type="primary" htmlType="submit" loading={busy}>Scan</Button>
          </Space>
        </Form>
      )}

      {active === 'add' && (
        <Form
          layout="vertical" size="small" disabled={busy}
          initialValues={{ port: 9999 }}
          onFinish={async (vals: { ip: string; port: number; username: string }) => {
            setBusy(true); setError(null);
            try {
              await actions.add(vals.ip, vals.port, vals.username);
              await refresh();
              close();
            } catch (e: unknown) { setError(String(e)); }
            finally { setBusy(false); }
          }}
          style={{ padding: 12 }}
        >
          <Form.Item label="IP" name="ip" rules={[{ required: true, pattern: /^(\d{1,3}\.){3}\d{1,3}$/ }]}>
            <Input placeholder="10.10.9.165" autoFocus />
          </Form.Item>
          <Form.Item label="HTTP port" name="port" rules={[{ required: true, type: 'number', min: 1, max: 65535 }]}>
            <InputNumber min={1} max={65535} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item label="Username" name="username" rules={[{ required: true }]}>
            <Input placeholder="optional label" />
          </Form.Item>
          <Space style={{ width: '100%', justifyContent: 'flex-end' }}>
            <Button onClick={close} icon={<CloseOutlined />}>Cancel</Button>
            <Button type="primary" htmlType="submit" loading={busy}>Add</Button>
          </Space>
        </Form>
      )}
    </div>
  );
}