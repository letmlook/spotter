import { Tag } from 'antd';
import type { RegistryEntry } from '../state/DeviceContext';
import styles from './DeviceRow.module.css';

export default function DeviceRow({
  device,
  selected,
  onClick,
}: {
  device: RegistryEntry;
  selected: boolean;
  onClick: () => void;
}) {
  const hostname = device.last_info?.basic?.hostname || '';
  const isJetson = !!device.last_info?.jetson?.model;
  return (
    <div
      className={`${styles.row} ${selected ? styles.selected : ''}`}
      onClick={onClick}
    >
      <span className={`${styles.dot} ${device.online ? styles.online : styles.offline}`} />
      <div className={styles.text}>
        <div className={styles.ip}>{device.ip}</div>
        <div className={styles.sub}>
          {hostname || '—'}
          {device.username && <> · {device.username}</>}
        </div>
      </div>
      {isJetson && <Tag color="orange" style={{ marginRight: 0 }}>Jetson</Tag>}
    </div>
  );
}
