import { Alert, Collapse, Space } from 'antd';
import { useEffect, useRef, useState } from 'react';
import { useI18n } from '../state/I18nContext';
import { EventsOff, EventsOn } from '../../wailsjs/runtime/runtime';
import { StartLogStream, StopLogStream } from '../../wailsjs/go/main/App';

interface LogLine {
  ts: string;
  line: string;
  cursor: string;
}

const MAX_LINES = 1000;

interface Props {
  deviceID: string;
  online: boolean;
}

export default function LogSection({ deviceID, online }: Props) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [lines, setLines] = useState<LogLine[]>([]);
  const [error, setError] = useState<string | null>(null);
  const bufRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    if (!online) {
      setError(t('log.offline'));
      return;
    }
    setError(null);
    setLines([]);
    StartLogStream(deviceID).catch((e: unknown) => {
      const msg = (e as { message?: string })?.message ?? String(e);
      setError(msg);
    });
    const onLine = (line: LogLine) => {
      setLines((prev) => {
        if (prev.length >= MAX_LINES) {
          return [...prev.slice(prev.length - MAX_LINES + 1), line];
        }
        return [...prev, line];
      });
    };
    const onErr = (msg: string) => setError(msg);
    EventsOn('device-log:' + deviceID, onLine);
    EventsOn('device-log-error:' + deviceID, onErr);
    return () => {
      StopLogStream(deviceID).catch(() => {});
      EventsOff('device-log:' + deviceID);
      EventsOff('device-log-error:' + deviceID);
    };
  }, [open, deviceID, online, t]);

  useEffect(() => {
    if (bufRef.current) bufRef.current.scrollTop = bufRef.current.scrollHeight;
  }, [lines, open]);

  return (
    <Collapse
      bordered={false}
      activeKey={open ? ['log'] : []}
      onChange={(keys) => setOpen(Array.isArray(keys) ? keys.includes('log') : !!keys)}
      items={[{
        key: 'log',
        label: (
          <Space size="small">
            <span>{t('log.title')}</span>
            {open && (
              <span style={{ color: 'var(--text-secondary)', fontSize: 12 }}>
                ● {t('log.streaming')}
              </span>
            )}
          </Space>
        ),
        children: error ? (
          <Alert type="error" message={error} />
        ) : (
          <div
            ref={bufRef}
            style={{
              height: 240,
              overflowY: 'auto',
              background: '#0e0e10',
              color: '#d4d4d4',
              padding: 8,
              borderRadius: 4,
              fontFamily: 'ui-monospace, Menlo, monospace',
              fontSize: 12,
              whiteSpace: 'pre-wrap',
            }}
          >
            {lines.length === 0 ? (
              <span style={{ color: '#888' }}>{t('log.empty')}</span>
            ) : (
              lines.map((l, i) => (
                <div key={i}>
                  <span style={{ color: '#7aa2f7' }}>{l.ts}</span>{' '}
                  <span>{l.line}</span>
                </div>
              ))
            )}
          </div>
        ),
        extra: online ? null : (
          <span style={{ color: 'var(--text-secondary)', fontSize: 12 }}>
            {t('log.disabled_offline')}
          </span>
        ),
      }]}
    />
  );
}