import { useEffect, useState } from 'react';
import { Modal, Form, Input, InputNumber, Select, Switch, message, Button, Space, Tooltip } from 'antd';
import { InfoCircleOutlined } from '@ant-design/icons';
import { useI18n } from '../state/I18nContext';
import { Get as GetSettings, Set as SetSettings } from '../../wailsjs/go/main/SettingsApp';

interface SettingsShape {
  multicast_group?: string;
  device_port?: number;
  scan_timeout?: number; // Go time.Duration is serialised as int64 ns (number on JS side)
  http_timeout?: number;
  poll_interval?: number;
  mcast_interval?: number;
  theme?: string;
  language?: string;
  auth_token?: string;
  enable_mdns?: boolean;
}

interface Props {
  open: boolean;
  onClose: () => void;
}

const num = (v: unknown): number => {
  const n = typeof v === 'string' ? Number(v) : (v as number | undefined);
  return Number.isFinite(n as number) ? (n as number) : 0;
};

export default function SettingsDialog({ open, onClose }: Props) {
  const { t } = useI18n();
  const [form] = Form.useForm<SettingsShape>();
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open) return;
    setLoading(true);
    GetSettings()
      .then((raw) => {
        const v = raw as unknown as SettingsShape;
        form.setFieldsValue({
          multicast_group: v.multicast_group ?? '239.255.42.42:9999',
          device_port: v.device_port ?? 9999,
          scan_timeout: num(v.scan_timeout),
          http_timeout: num(v.http_timeout),
          poll_interval: num(v.poll_interval),
          mcast_interval: num(v.mcast_interval),
          theme: v.theme ?? 'system',
          language: v.language ?? 'zh-CN',
          auth_token: v.auth_token ?? '',
          enable_mdns: v.enable_mdns ?? false,
        });
      })
      .catch((err: unknown) => {
        message.error(t('settings.load.failed') + ': ' + String(err));
      })
      .finally(() => setLoading(false));
  }, [open, form, t]);

  const onSave = async () => {
    try {
      const values = await form.validateFields();
      setSaving(true);
      await SetSettings(values as unknown as never);
      message.success(t('settings.save.ok'));
      onClose();
    } catch (err) {
      const detail = (err as { errorFields?: unknown[]; message?: string })?.message
        ?? (Array.isArray((err as { errorFields?: unknown[] })?.errorFields)
          ? (err as { errorFields: { errors: unknown[] }[] }).errorFields.map((e) => e.errors.join('; ')).join('; ')
          : null)
        ?? String(err);
      message.error(t('settings.save.failed') + ': ' + detail);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal
      title={t('settings.title')}
      open={open}
      onCancel={onClose}
      destroyOnClose
      footer={[
        <Button key="cancel" onClick={onClose}>{t('common.cancel')}</Button>,
        <Button key="save" type="primary" loading={saving} onClick={onSave}>{t('common.save')}</Button>,
      ]}
    >
      {loading ? (
        <div>{t('common.loading')}</div>
      ) : (
        <Form form={form} layout="vertical">
          <Form.Item
            name="multicast_group"
            label={t('settings.field.multicast')}
            rules={[{ required: true, message: t('settings.field.multicast.required') }]}
          >
            <Input placeholder="239.255.42.42:9999" />
          </Form.Item>
          <Form.Item
            name="device_port"
            label={t('settings.field.devicePort')}
            rules={[{ required: true, type: 'number', min: 1, max: 65535 }]}
          >
            <InputNumber style={{ width: '100%' }} />
          </Form.Item>
          <Space>
            <Form.Item name="scan_timeout" label={t('settings.field.scanTimeout')} rules={[{ required: true, type: 'number', min: 0 }]}>
              <InputNumber addonAfter="ns" style={{ width: 200 }} />
            </Form.Item>
            <Form.Item name="http_timeout" label={t('settings.field.httpTimeout')} rules={[{ required: true, type: 'number', min: 0 }]}>
              <InputNumber addonAfter="ns" style={{ width: 200 }} />
            </Form.Item>
          </Space>
          <Space>
            <Form.Item name="poll_interval" label={t('settings.field.pollInterval')} rules={[{ required: true, type: 'number', min: 0 }]}>
              <InputNumber addonAfter="ns" style={{ width: 200 }} />
            </Form.Item>
            <Form.Item name="mcast_interval" label={t('settings.field.mcastInterval')} rules={[{ required: true, type: 'number', min: 0 }]}>
              <InputNumber addonAfter="ns" style={{ width: 200 }} />
            </Form.Item>
          </Space>
          <Form.Item name="theme" label={t('settings.field.theme')}>
            <Select options={[
              { value: 'system', label: t('menu.view.theme.system') },
              { value: 'light', label: t('menu.view.theme.light') },
              { value: 'dark', label: t('menu.view.theme.dark') },
            ]} />
          </Form.Item>
          <Form.Item name="language" label={t('settings.field.language')}>
            <Select options={[
              { value: 'zh-CN', label: '简体中文' },
              { value: 'en', label: 'English' },
            ]} />
          </Form.Item>
          <Form.Item name="auth_token" label={t('settings.field.authToken')} help={t('settings.field.authToken.help')}>
            <Input.Password autoComplete="off" />
          </Form.Item>
          <Form.Item
            name="enable_mdns"
            label={
              <Space size={4}>
                <span>{t('settings.field.enableMdns')}</span>
                <Tooltip title={t('settings.field.enableMdns.help')}>
                  <InfoCircleOutlined style={{ color: 'var(--text-secondary)' }} />
                </Tooltip>
              </Space>
            }
            valuePropName="checked"
          >
            <Switch />
          </Form.Item>
        </Form>
      )}
    </Modal>
  );
}
