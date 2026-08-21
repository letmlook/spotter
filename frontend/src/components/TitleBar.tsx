import { useEffect, useRef } from 'react';
import { MinusOutlined, BorderOutlined, CloseOutlined } from '@ant-design/icons';
import {
  WindowMinimise,
  WindowToggleMaximise,
  Quit,
  WindowGetPosition,
} from '../../wailsjs/runtime/runtime';
import styles from './TitleBar.module.css';

export default function TitleBar() {
  const draggingRef = useRef(false);
  const startRef = useRef({ x: 0, y: 0, winX: 0, winY: 0 });

  useEffect(() => {
    const onMove = (e: MouseEvent) => {
      if (!draggingRef.current) return;
      const dx = e.screenX - startRef.current.x;
      const dy = e.screenY - startRef.current.y;
      // Use Wails runtime to set window position. We import dynamically
      // to avoid SSR / circular issues.
      import('../../wailsjs/runtime/runtime').then(({ WindowSetPosition }) => {
        WindowSetPosition(startRef.current.winX + dx, startRef.current.winY + dy);
      });
    };
    const onUp = () => { draggingRef.current = false; };
    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseup', onUp);
    return () => {
      window.removeEventListener('mousemove', onMove);
      window.removeEventListener('mouseup', onUp);
    };
  }, []);

  const onMouseDown = async (e: React.MouseEvent) => {
    // Only left button.
    if (e.button !== 0) return;
    draggingRef.current = true;
    const pos = await WindowGetPosition();
    startRef.current = { x: e.screenX, y: e.screenY, winX: pos.x, winY: pos.y };
  };

  return (
    <div className={styles.bar}>
      <div className={styles.left}>
        <svg className={styles.logo} viewBox="0 0 32 32" fill="none" xmlns="http://www.w3.org/2000/svg" aria-label="Spotter logo">
          <path d="M 7 23.5 A 12.5 12.5 0 0 1 25 23.5" stroke="#1677ff" strokeOpacity="0.30" strokeWidth="2" strokeLinecap="round"/>
          <path d="M 11 22 A 8.5 8.5 0 0 1 21 22" stroke="#1677ff" strokeOpacity="0.60" strokeWidth="2" strokeLinecap="round"/>
          <path d="M 14.5 20.5 A 4.5 4.5 0 0 1 17.5 20.5" stroke="#69b1ff" strokeWidth="2" strokeLinecap="round"/>
          <circle cx="16" cy="20.75" r="1.75" fill="#69b1ff"/>
          <circle cx="16" cy="20.75" r="0.75" fill="#0a0a0a"/>
        </svg>
        <span className={styles.title}>Spotter</span>
      </div>
      <div
        className={styles.middle}
        onMouseDown={onMouseDown}
        onDoubleClick={WindowToggleMaximise}
      />
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
