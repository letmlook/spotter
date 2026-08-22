import { Modal, Typography, Space } from 'antd';
import { useMenu } from '../state/MenuContext';
import { useI18n } from '../state/I18nContext';

const { Title, Paragraph, Text } = Typography;

export default function AboutDialog() {
  const { modal, closeModal } = useMenu();
  const { t } = useI18n();
  return (
    <Modal
      open={modal === 'about'}
      onCancel={closeModal}
      footer={null}
      width={460}
      centered
      title={null}
    >
      <div style={{ textAlign: 'center', paddingTop: 8 }}>
        <svg width="72" height="72" viewBox="0 0 32 32" fill="none" xmlns="http://www.w3.org/2000/svg" aria-label="Spotter logo">
          <path d="M 7 23.5 A 12.5 12.5 0 0 1 25 23.5" stroke="#1677ff" strokeOpacity="0.30" strokeWidth="2" strokeLinecap="round"/>
          <path d="M 11 22 A 8.5 8.5 0 0 1 21 22" stroke="#1677ff" strokeOpacity="0.60" strokeWidth="2" strokeLinecap="round"/>
          <path d="M 14.5 20.5 A 4.5 4.5 0 0 1 17.5 20.5" stroke="#69b1ff" strokeWidth="2" strokeLinecap="round"/>
          <circle cx="16" cy="20.75" r="1.75" fill="#69b1ff"/>
          <circle cx="16" cy="20.75" r="0.75" fill="#0a0a0a"/>
        </svg>
        <Title level={3} style={{ marginTop: 12, marginBottom: 0 }}>{t('app.title')}</Title>
        <Text type="secondary">v0.1.0</Text>
      </div>

      <Paragraph style={{ marginTop: 20, textAlign: 'center' }}>
        {t('modal.about.tagline')}
        <br />
        {t('modal.about.tagline2')}
      </Paragraph>

      <Space direction="vertical" size={4} style={{ width: '100%', fontSize: 12 }}>
        <div><Text type="secondary">{t('modal.about.client')}:</Text> Windows · macOS · Linux</div>
        <div><Text type="secondary">{t('modal.about.agent')}:</Text> Linux (systemd) — arm64 · amd64</div>
      </Space>

      <Paragraph style={{ marginTop: 20, marginBottom: 0, textAlign: 'center', fontSize: 11 }}>
        <Text type="secondary">{t('modal.about.copyright')}</Text>
      </Paragraph>
    </Modal>
  );
}