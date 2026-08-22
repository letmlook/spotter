import { useState } from 'react';
import { Button, Form, Input, InputNumber, Space, Alert } from 'antd';
import { ScanOutlined, ImportOutlined, CloseOutlined } from '@ant-design/icons';
import { useDeviceActions } from '../hooks/useDeviceActions';
import { useDevices } from '../state/DeviceContext';

type ActiveForm = null | 'scan-custom' | 'add';

export default function ActionPanel() {
  const [active, setActive] = useState<ActiveForm>(null);
  const [error, setError] = useState<string | null>(null);
  const { refresh } = useDevices();
  const actions = useDeviceActions();
  const [quickBusy, setQuickBusy] = useState(false);
  const [formBusy, setFormBusy] = useState(false);

  const close = () => { setActive(null); setError(null); };

  // One-click scan: backend auto-detects the local subnet.
  const onQuickScan = async () => {
    setQuickBusy(true);
    setError(null);
    try {
      await actions.scan();
      await refresh();
    } catch (e: unknown) {
      setError(`Quick scan failed: ${e}`);
    } finally {
      setQuickBusy(false);
    }
  };

  return (
    <div style={{ borderBottom: '1px solid #303030' }}>
      <Space.Compact block>
        <Button
          icon={<ScanOutlined />}
          onClick={onQuickScan}
          loading={quickBusy}
          block
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

      <div style={{ padding: '4px 12px 0' }}>
        <Button
          type="link"
          size="small"
          onClick={() => setActive(active === 'scan-custom' ? null : 'scan-custom')}
          style={{ padding: 0, height: 'auto', fontSize: 11 }}
        >
          {active === 'scan-custom' ? '− Hide custom CIDR' : '+ Scan custom CIDR…'}
        </Button>
      </div>

      {error && <Alert type="error" message={error} closable onClose={() => setError(null)} style={{ margin: 8 }} />}

      {active === 'scan-custom' && (
        <Form
          layout="vertical" size="small" disabled={formBusy}
          onFinish={async (vals: { cidr: string }) => {
            setFormBusy(true); setError(null);
            try {
              await actions.scan(vals.cidr);
              await refresh();
              close();
            } catch (e: unknown) { setError(String(e)); }
            finally { setFormBusy(false); }
          }}
          style={{ padding: 12 }}
        >
          <Form.Item label="CIDR" name="cidr" rules={[{ required: true, pattern: /^(\d{1,3}\.){3}\d{1,3}\/\d{1,2}$/, message: 'Invalid CIDR' }]}>
            <Input placeholder="192.168.3.0/24" autoFocus />
          </Form.Item>
          <Space style={{ width: '100%', justifyContent: 'flex-end' }}>
            <Button onClick={close} icon={<CloseOutlined />}>Cancel</Button>
            <Button type="primary" htmlType="submit" loading={formBusy}>Scan</Button>
          </Space>
        </Form>
      )}

      {active === 'add' && (
        <Form
          layout="vertical" size="small" disabled={formBusy}
          initialValues={{ port: 9999 }}
          onFinish={async (vals: { ip: string; port: number; username: string }) => {
            setFormBusy(true); setError(null);
            try {
              await actions.add(vals.ip, vals.port, vals.username);
              await refresh();
              close();
            } catch (e: unknown) { setError(String(e)); }
            finally { setFormBusy(false); }
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
            <Button type="primary" htmlType="submit" loading={formBusy}>Add</Button>
          </Space>
        </Form>
      )}
    </div>
  );
}