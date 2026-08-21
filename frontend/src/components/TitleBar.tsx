import { MinusOutlined, BorderOutlined, CloseOutlined } from '@ant-design/icons';
import {
  WindowMinimise,
  WindowToggleMaximise,
  Quit,
} from '../../wailsjs/runtime/runtime';
import styles from './TitleBar.module.css';

export default function TitleBar() {
  return (
    <div className={styles.bar}>
      <div className={styles.left}>
        <span className={styles.logo}>●</span>
        <span className={styles.title}>Spotter</span>
      </div>
      <div className={styles.middle} onDoubleClick={WindowToggleMaximise} />
      <div className={styles.right}>
        <button className={styles.btn} aria-label="minimise" onClick={WindowMinimise}>
          <MinusOutlined />
        </button>
        <button className={styles.btn} aria-label="toggle maximise" onClick={WindowToggleMaximise}>
          <BorderOutlined />
        </button>
        <button
          className={`${styles.btn} ${styles.close}`}
          aria-label="close"
          onClick={Quit}
        >
          <CloseOutlined />
        </button>
      </div>
    </div>
  );
}
