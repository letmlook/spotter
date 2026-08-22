import { Modal, Typography, Alert, Tabs, Space } from 'antd';
import { CopyOutlined } from '@ant-design/icons';
import { useI18n } from '../state/I18nContext';

const { Paragraph, Text } = Typography;

interface DeviceSetupGuideProps {
  open: boolean;
  onClose: () => void;
}

function CodeBlock({ children, dark = true }: { children: string; dark?: boolean }) {
  const text = children.replace(/\n$/, '');
  return (
    <div
      style={{
        position: 'relative',
        background: dark ? '#1a1a1a' : 'var(--bg-elevated)',
        border: '1px solid var(--border)',
        borderRadius: 4,
        padding: '8px 36px 8px 12px',
        margin: '4px 0',
        fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace',
        fontSize: 12,
        whiteSpace: 'pre',
        overflowX: 'auto',
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
                  message="Recommended: one-shot deploy script"
                  description={
                    <Space direction="vertical" size={4}>
                      <span>From your dev machine, run:</span>
                      <CodeBlock>{`# macOS / Linux — SSH key auth (preferred, no password)
./scripts/deploy.sh <user> <ip>

# macOS / Linux — password auth
./scripts/deploy.sh <user> <password> <ip>

# Windows PowerShell — key auth (Pageant / ssh-agent)
.\\scripts\\deploy.ps1 -User <user> -Ip <ip>

# Windows PowerShell — password auth
.\\scripts\\deploy.ps1 -User <user> -Password <password> -Ip <ip>`}</CodeBlock>
                      <span>The script uploads spotterd, the systemd unit, and install.sh, then runs the installer and verifies /healthz.</span>
                    </Space>
                  }
                />
                <Paragraph style={{ marginTop: 8 }}>
                  <Text strong>Or, do it by hand:</Text>
                </Paragraph>

                <Paragraph style={{ marginTop: 8 }}>
                  <Text strong>Step 1 —</Text> Build the arm64 binary:
                </Paragraph>
                <CodeBlock>{`make agent-linux-arm64`}</CodeBlock>

                <Paragraph style={{ marginTop: 12 }}>
                  <Text strong>Step 2 —</Text> Copy the binary, systemd unit, and install
                  script to the target:
                </Paragraph>
                <CodeBlock>{`scp bin/spotterd-linux-arm64   user@<device>:/tmp/spotterd
scp scripts/spotterd.service    user@<device>:/tmp/spotterd.service
scp scripts/install.sh          user@<device>:/tmp/install.sh`}</CodeBlock>

                <Paragraph style={{ marginTop: 12 }}>
                  <Text strong>Step 3 —</Text> Run the installer:
                </Paragraph>
                <CodeBlock>{`ssh user@<device> sudo bash /tmp/install.sh`}</CodeBlock>

                <Paragraph style={{ marginTop: 12 }}>
                  The installer writes <Text code>/usr/local/bin/spotterd</Text>, generates a
                  device_id, and enables the systemd unit. The device should appear in the
                  GUI within ~30 seconds.
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
                  message="Recommended: one-shot deploy script"
                  description={
                    <Space direction="vertical" size={4}>
                      <span>From your dev machine, run:</span>
                      <CodeBlock>{`# macOS / Linux — SSH key auth (preferred, no password)
./scripts/deploy.sh <user> <ip> amd64

# macOS / Linux — password auth
./scripts/deploy.sh <user> <password> <ip> amd64

# Windows PowerShell — key auth (Pageant / ssh-agent)
.\\scripts\\deploy.ps1 -User <user> -Ip <ip> -Arch amd64

# Windows PowerShell — password auth
.\\scripts\\deploy.ps1 -User <user> -Password <password> -Ip <ip> -Arch amd64`}</CodeBlock>
                      <span>The script uploads spotterd, the systemd unit, and install.sh, then runs the installer and verifies /healthz.</span>
                    </Space>
                  }
                />
                <Paragraph style={{ marginTop: 8 }}>
                  <Text strong>Or, do it by hand:</Text>
                </Paragraph>

                <Paragraph style={{ marginTop: 8 }}>
                  <Text strong>Step 1 —</Text> Build the amd64 binary:
                </Paragraph>
                <CodeBlock>{`make agent-linux-x64`}</CodeBlock>

                <Paragraph style={{ marginTop: 12 }}>
                  <Text strong>Step 2 —</Text> Copy the binary, systemd unit, and install
                  script to the target:
                </Paragraph>
                <CodeBlock>{`scp bin/spotterd-linux-x64     user@<device>:/tmp/spotterd
scp scripts/spotterd.service    user@<device>:/tmp/spotterd.service
scp scripts/install.sh          user@<device>:/tmp/install.sh`}</CodeBlock>

                <Paragraph style={{ marginTop: 12 }}>
                  <Text strong>Step 3 —</Text> Run the installer:
                </Paragraph>
                <CodeBlock>{`ssh user@<device> sudo bash /tmp/install.sh`}</CodeBlock>

                <Paragraph style={{ marginTop: 12 }}>
                  The installer writes <Text code>/usr/local/bin/spotterd</Text>, generates a
                  device_id, and enables the systemd unit. The device should appear in the
                  GUI within ~30 seconds.
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
                  <Text strong>On the device,</Text> confirm spotterd is running:
                </Paragraph>
                <CodeBlock>{`systemctl status spotterd
curl -s http://127.0.0.1:9999/healthz`}</CodeBlock>

                <Paragraph style={{ marginTop: 12 }}>
                  <Text strong>Multicast blocked?</Text> If the device does not appear
                  automatically, use the <Text strong>Tools → Add device by IP</Text> menu and
                  enter the device IP and port <Text code>9999</Text> manually.
                </Paragraph>

                <Paragraph style={{ marginTop: 12 }}>
                  <Text strong>Uninstall</Text> (if you ever need to remove the agent):
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