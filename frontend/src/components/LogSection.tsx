import { Alert, Collapse, Space, Input, Select, Button, Tooltip, message } from 'antd';
import { DownloadOutlined, ClearOutlined, SearchOutlined } from '@ant-design/icons';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useI18n } from '../state/I18nContext';
import { EventsOff, EventsOn } from '../../wailsjs/runtime/runtime';
import { StartLogStream, StopLogStream } from '../../wailsjs/go/main/LogStreamApp';

interface LogLine {
  ts: string;
  line: string;
  cursor: string;
}

const MAX_LINES = 1000;

type LevelFilter = 'all' | 'error' | 'warn' | 'info' | 'debug';

interface Props {
  deviceID: string;
  online: boolean;
}

function classify(line: string): LevelFilter {
  // Heuristic classifier for systemd journal's PRIORITY conventions;
  // journalctl JSON has a numeric priority field but the agent
  // strips it before forwarding — we fall back to substring matching.
  const l = line.toLowerCase();
  if (/\b(err|fatal|panic|fail|denied)\b/.test(l)) return 'error';
  if (/\bwarn/.test(l)) return 'warn';
  if (/\bdebug/.test(l)) return 'debug';
  return 'info';
}

export default function LogSection({ deviceID, online }: Props) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [lines, setLines] = useState<LogLine[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [query, setQuery] = useState('');
  const [level, setLevel] = useState<LevelFilter>('all');
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

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return lines.filter((l) => {
      if (level !== 'all' && classify(l.line) !== level) return false;
      if (q && !l.line.toLowerCase().includes(q)) return false;
      return true;
    });
  }, [lines, query, level]);

  const exportLog = (format: 'log' | 'ndjson') => {
    try {
      const body = format === 'log'
        ? filtered.map((l) => `${l.ts} ${l.line}`).join('\n')
        : filtered.map((l) => JSON.stringify(l)).join('\n');
      const blob = new Blob([body], { type: format === 'log' ? 'text/plain' : 'application/x-ndjson' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `${deviceID}-${new Date().toISOString().replace(/[:.]/g, '-')}.${format}`;
      a.click();
      URL.revokeObjectURL(url);
      void message.success(t('log.export.ok'));
    } catch (e) {
      void message.error(t('log.export.failed') + ': ' + String(e));
    }
  };

  const highlight = (text: string): React.ReactNode => {
    const q = query.trim();
    if (!q) return text;
    const lower = text.toLowerCase();
    const ql = q.toLowerCase();
    const out: React.ReactNode[] = [];
    let i = 0;
    let key = 0;
    while (i < text.length) {
      const j = lower.indexOf(ql, i);
      if (j < 0) {
        out.push(text.slice(i));
        break;
      }
      out.push(text.slice(i, j));
      out.push(<mark key={key++} style={{ background: '#ffd54f', color: '#000' }}>{text.slice(j, j + q.length)}</mark>);
      i = j + q.length;
    }
    return out;
  };

  const colourFor = (line: string): string => {
    switch (classify(line)) {
      case 'error': return '#ff7875';
      case 'warn': return '#ffc53d';
      case 'debug': return '#888';
      default: return '#d4d4d4';
    }
  };

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
                ● {t('log.streaming')} ({filtered.length}/{lines.length})
              </span>
            )}
          </Space>
        ),
        children: error ? (
          <Alert type="error" message={error} />
        ) : (
          <div>
            <Space.Compact style={{ width: '100%', marginBottom: 8 }}>
              <Input
                prefix={<SearchOutlined />}
                allowClear
                placeholder={t('log.search.placeholder')}
                value={query}
                onChange={(e) => setQuery(e.target.value)}
              />
              <Select
                value={level}
                onChange={(v) => setLevel(v as LevelFilter)}
                style={{ width: 120 }}
                options={[
                  { value: 'all', label: t('log.level.all') },
                  { value: 'error', label: t('log.level.error') },
                  { value: 'warn', label: t('log.level.warn') },
                  { value: 'info', label: t('log.level.info') },
                  { value: 'debug', label: t('log.level.debug') },
                ]}
              />
              <Tooltip title={t('log.export.log')}>
                <Button icon={<DownloadOutlined />} onClick={() => exportLog('log')}>{'.log'}</Button>
              </Tooltip>
              <Tooltip title={t('log.export.ndjson')}>
                <Button icon={<DownloadOutlined />} onClick={() => exportLog('ndjson')}>{'.ndjson'}</Button>
              </Tooltip>
              <Tooltip title={t('log.clear')}>
                <Button icon={<ClearOutlined />} onClick={() => setLines([])} />
              </Tooltip>
            </Space.Compact>
            <div
              ref={bufRef}
              style={{
                height: 240,
                overflowY: 'auto',
                background: '#0e0e10',
                padding: 8,
                borderRadius: 4,
                fontFamily: 'ui-monospace, Menlo, monospace',
                fontSize: 12,
                whiteSpace: 'pre-wrap',
              }}
            >
              {filtered.length === 0 ? (
                <span style={{ color: '#888' }}>{lines.length === 0 ? t('log.empty') : t('log.noMatch')}</span>
              ) : (
                filtered.map((l, i) => (
                  <div key={i} style={{ color: colourFor(l.line) }}>
                    <span style={{ color: '#7aa2f7' }}>{l.ts}</span>{' '}
                    <span>{highlight(l.line)}</span>
                  </div>
                ))
              )}
            </div>
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
