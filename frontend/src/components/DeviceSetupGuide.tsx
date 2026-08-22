import { Modal, Typography, Alert, Tabs, Space } from 'antd';
import { CopyOutlined } from '@ant-design/icons';
import { useI18n } from '../state/I18nContext';

const { Paragraph, Text } = Typography;

interface DeviceSetupGuideProps {
  open: boolean;
  onClose: () => void;
}

function CodeBlock({ children }: { children: string }) {
  const text = children.replace(/\n$/, '');
  return (
    <div
      style={{
        position: 'relative',
        background: 'var(--bg-elevated)',
        border: '1px solid var(--border)',
        borderRadius: 4,
        padding: '8px 36px 8px 12px',
        margin: '4px 0',
        fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace',
        fontSize: 12,
        color: 'var(--text-primary)',
        whiteSpace: 'pre-wrap',
        wordBreak: 'break-word',
        overflowWrap: 'anywhere',
        maxWidth: '100%',
        boxSizing: 'border-box',
      }}
    >
      <CopyOutlined
        title="Copy"
        style={{
          position: 'absolute',
          top: 8,
          right: 8,
          cursor: 'pointer',
          color: 'var(--text-secondary)',
        }}
        onClick={() => navigator.clipboard.writeText(text)}
      />
      {text}
    </div>
  );
}

export default function DeviceSetupGuide({ open, onClose }: DeviceSetupGuideProps) {
  const { t } = useI18n();
  const installerWritesPath = '/usr/local/bin/spotterd';
  const addByIpMenu = t('empty.add.shortcut');
  return (
    <Modal
      open={open}
      onCancel={onClose}
      footer={null}
      title={t('modal.guide.title')}
      width={680}
    >
      <Paragraph>
        <Text>{t('modal.guide.body')}</Text>
      </Paragraph>

      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 12 }}
        message={t('modal.guide.prereq')}
        description={
          <Space direction="vertical" size={2}>
            <span>· {t('modal.guide.prereq.1')}</span>
            <span>· {t('modal.guide.prereq.2')}</span>
            <span>· {t('modal.guide.prereq.3')}</span>
          </Space>
        }
      />

      <Tabs
        size="small"
        items={[
          {
            key: 'arm64',
            label: t('modal.guide.tab.arm64'),
            children: (
              <>
                <Alert
                  type="success"
                  showIcon
                  style={{ marginBottom: 12 }}
                  message={t('guide.recommended')}
                  description={
                    <Space direction="vertical" size={4}>
                      <span>{t('guide.from_dev')}</span>
                      <CodeBlock>{`# macOS / Linux — SSH key auth (preferred, no password)
./scripts/deploy.sh <user> <ip>

# macOS / Linux — password auth
./scripts/deploy.sh <user> <password> <ip>

# Windows PowerShell — key auth (Pageant / ssh-agent)
.\\scripts\\deploy.ps1 -User <user> -Ip <ip>

# Windows PowerShell — password auth
.\\scripts\\deploy.ps1 -User <user> -Password <password> -Ip <ip>`}</CodeBlock>
                      <span>{t('guide.script_does')}</span>
                    </Space>
                  }
                />
                <Paragraph style={{ marginTop: 8 }}>
                  <Text strong>{t('guide.hand')}</Text>
                </Paragraph>

                <Paragraph style={{ marginTop: 8 }}>
                  <Text strong>{t('guide.step1.arm64')}</Text>
                </Paragraph>
                <CodeBlock>{`make agent-linux-arm64`}</CodeBlock>

                <Paragraph style={{ marginTop: 12 }}>
                  <Text strong>{t('guide.step2')}</Text>
                </Paragraph>
                <CodeBlock>{`scp bin/spotterd-linux-arm64   user@<device>:/tmp/spotterd
scp scripts/spotterd.service    user@<device>:/tmp/spotterd.service
scp scripts/install.sh          user@<device>:/tmp/install.sh`}</CodeBlock>

                <Paragraph style={{ marginTop: 12 }}>
                  <Text strong>{t('guide.step3')}</Text>
                </Paragraph>
                <CodeBlock>{`ssh user@<device> sudo bash /tmp/install.sh`}</CodeBlock>

                <Paragraph style={{ marginTop: 12 }}>
                  {t('guide.installer_writes', { path: installerWritesPath })}
                </Paragraph>
              </>
            ),
          },
          {
            key: 'amd64',
            label: t('modal.guide.tab.amd64'),
            children: (
              <>
                <Alert
                  type="success"
                  showIcon
                  style={{ marginBottom: 12 }}
                  message={t('guide.recommended')}
                  description={
                    <Space direction="vertical" size={4}>
                      <span>{t('guide.from_dev')}</span>
                      <CodeBlock>{`# macOS / Linux — SSH key auth (preferred, no password)
./scripts/deploy.sh <user> <ip> amd64

# macOS / Linux — password auth
./scripts/deploy.sh <user> <password> <ip> amd64

# Windows PowerShell — key auth (Pageant / ssh-agent)
.\\scripts\\deploy.ps1 -User <user> -Ip <ip> -Arch amd64

# Windows PowerShell — password auth
.\\scripts\\deploy.ps1 -User <user> -Password <password> -Ip <ip> -Arch amd64`}</CodeBlock>
                      <span>{t('guide.script_does')}</span>
                    </Space>
                  }
                />
                <Paragraph style={{ marginTop: 8 }}>
                  <Text strong>{t('guide.hand')}</Text>
                </Paragraph>

                <Paragraph style={{ marginTop: 8 }}>
                  <Text strong>{t('guide.step1.amd64')}</Text>
                </Paragraph>
                <CodeBlock>{`make agent-linux-x64`}</CodeBlock>

                <Paragraph style={{ marginTop: 12 }}>
                  <Text strong>{t('guide.step2')}</Text>
                </Paragraph>
                <CodeBlock>{`scp bin/spotterd-linux-x64     user@<device>:/tmp/spotterd
scp scripts/spotterd.service    user@<device>:/tmp/spotterd.service
scp scripts/install.sh          user@<device>:/tmp/install.sh`}</CodeBlock>

                <Paragraph style={{ marginTop: 12 }}>
                  <Text strong>{t('guide.step3')}</Text>
                </Paragraph>
                <CodeBlock>{`ssh user@<device> sudo bash /tmp/install.sh`}</CodeBlock>

                <Paragraph style={{ marginTop: 12 }}>
                  {t('guide.installer_writes', { path: installerWritesPath })}
                </Paragraph>
              </>
            ),
          },
          {
            key: 'verify',
            label: t('modal.guide.tab.verify'),
            children: (
              <>
                <Paragraph style={{ marginTop: 8 }}>
                  <Text strong>{t('guide.on_device')}</Text> {t('guide.confirm_running')}
                </Paragraph>
                <CodeBlock>{`systemctl status spotterd
curl -s http://127.0.0.1:9999/healthz`}</CodeBlock>

                <Paragraph style={{ marginTop: 12 }}>
                  <Text strong>{t('guide.multicast_blocked')}</Text> {t('guide.add_by_ip_hint', { menu: addByIpMenu, port: '9999' })}
                </Paragraph>

                <Paragraph style={{ marginTop: 12 }}>
                  <Text strong>{t('guide.uninstall_hint')}</Text>
                </Paragraph>
                <CodeBlock>{`ssh user@<device> sudo bash /tmp/uninstall.sh`}</CodeBlock>
              </>
            ),
          },
        ]}
      />
    </Modal>
  );
}