import { Tag } from 'antd';
import { useI18n } from '../state/I18nContext';
import type { RegistryEntry } from '../state/DeviceContext';
import styles from './DeviceRow.module.css';

interface Props {
  device: RegistryEntry;
  selected: boolean;
  onClick: () => void;
}

export default function DeviceRow({ device, selected, onClick }: Props) {
  const { t } = useI18n();
  const isJetson = !!device.last_info?.jetson?.model;
  const hostname = device.last_info?.basic?.hostname || device.ip;
  const sub = device.username
    ? `${device.ip} · ${device.username}`
    : device.ip;
  return (
    <div
      className={`${styles.row} ${selected ? styles.selected : ''}`}
      onClick={onClick}
    >
      <span className={`${styles.dot} ${device.online ? styles.online : styles.offline}`} />
      <div className={styles.text}>
        <div className={styles.ip}>
          {hostname}
          {isJetson && <Tag color="orange" style={{ marginLeft: 8, marginRight: 0 }}>{t('card.jetson.tag')}</Tag>}
        </div>
        <div className={styles.sub}>{sub}</div>
      </div>
    </div>
  );
}