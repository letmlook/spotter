import { Empty, Typography, Button } from 'antd';
import { BookOutlined } from '@ant-design/icons';
import { useMenu } from '../state/MenuContext';
import { useI18n } from '../state/I18nContext';

export default function EmptyState() {
  const { openModal } = useMenu();
  const { t } = useI18n();
  return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%' }}>
      <Empty
        image={Empty.PRESENTED_IMAGE_SIMPLE}
        description={t('empty.title')}
      >
        <Typography.Paragraph type="secondary" style={{ marginBottom: 16 }}>
          {t('empty.body', {
            scan: t('empty.scan.shortcut'),
            add: t('empty.add.shortcut'),
          })}
        </Typography.Paragraph>
        <Button
          icon={<BookOutlined />}
          onClick={() => openModal('setup-guide')}
        >
          {t('empty.cta')}
        </Button>
      </Empty>
    </div>
  );
}