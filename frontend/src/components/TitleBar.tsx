import { MinusOutlined, BorderOutlined, CloseOutlined } from '@ant-design/icons';
import styles from './TitleBar.module.css';

export default function TitleBar() {
  return (
    <div className={styles.bar}>
      <div className={styles.left}>
        <span className={styles.logo}>●</span>
        <span className={styles.title}>Spotter</span>
      </div>
      <div className={styles.middle} />
      <div className={styles.right}>
        <button className={styles.btn} aria-label="minimise"><MinusOutlined /></button>
        <button className={styles.btn} aria-label="toggle maximise"><BorderOutlined /></button>
        <button className={`${styles.btn} ${styles.close}`} aria-label="close"><CloseOutlined /></button>
      </div>
    </div>
  );
}