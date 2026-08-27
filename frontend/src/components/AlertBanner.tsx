import { useMemo } from 'react';
import { Alert, Tag, Tooltip } from 'antd';
import { WarningTwoTone, CloseCircleTwoTone, InfoCircleTwoTone } from '@ant-design/icons';
import { useI18n } from '../state/I18nContext';
import { useDevices } from '../state/DeviceContext';
import {
  evaluate,
  type Alert as RuleAlert,
  type AlertSeverity,
} from '../state/alerts';

interface Props {
  /** Override the current time for tests. Default `Date.now`. */
  now?: number;
}

// AlertBanner is a thin wrapper over `evaluate(...)` that turns
// the resulting alerts into a stacked list of AntD `<Alert>`
// cards at the top of the page. The component is intentionally
// render-only: it doesn't dispatch any side effects, doesn't
// deduplicate against a server, doesn't pageinate. v0.5.x's
// needs are "show me what's wrong right now", not "give me
// everything since the dawn of time".
export default function AlertBanner({ now }: Props) {
  const { state } = useDevices();
  const { t } = useI18n();

  const alerts = useMemo<RuleAlert[]>(
    () => evaluate(state.devices, now ?? Date.now()),
    [state.devices, now],
  );

  if (alerts.length === 0) {
    return null;
  }

  // Critical first, then warning, then info. Within the same
  // severity, sort by device id so the banner doesn't dance
  // when alerts come and go in arbitrary order.
  const sorted = [...alerts].sort((a, b) => {
    const sevOrder = { critical: 0, warning: 1, info: 2 } as const;
    const ds = sevOrder[a.severity] - sevOrder[b.severity];
    return ds !== 0 ? ds : a.deviceId.localeCompare(b.deviceId);
  });

  return (
    <div
      data-testid="alert-banner"
      style={{
        display: 'flex',
        flexDirection: 'column',
        gap: 6,
        padding: '8px 12px',
        background: 'var(--bg-app)',
        borderBottom: '1px solid var(--border)',
      }}
    >
      {sorted.map((a) => (
        <Alert
          key={a.id}
          type={severityToAntdType(a.severity)}
          showIcon
          icon={iconFor(a.severity)}
          message={
            <span>
              <Tag color={a.severity === 'critical' ? 'red' : a.severity === 'warning' ? 'orange' : 'blue'}>
                {t(`alert.severity.${a.severity}`) || a.severity}
              </Tag>
              <strong style={{ marginLeft: 6 }}>{a.hostname}</strong>
              <span style={{ marginLeft: 8, color: 'var(--text-secondary)' }}>
                {a.message}
              </span>
            </span>
          }
          banner
          style={{ padding: '4px 10px' }}
        />
      ))}
      {sorted.length > 5 && (
        <Tooltip title={t('alert.tooltip.more') || ''}>
          <div style={{ fontSize: 12, color: 'var(--text-secondary)' }}>
            {t('alert.moreCount', { n: sorted.length - 5 }) ||
              `+${sorted.length - 5} more (see device list)`}
          </div>
        </Tooltip>
      )}
    </div>
  );
}

function severityToAntdType(sev: AlertSeverity): 'error' | 'warning' | 'info' {
  switch (sev) {
    case 'critical': return 'error';
    case 'warning': return 'warning';
    case 'info': return 'info';
  }
}

function iconFor(sev: AlertSeverity) {
  if (sev === 'critical') return <CloseCircleTwoTone twoToneColor="#b71c1c" />;
  if (sev === 'warning') return <WarningTwoTone twoToneColor="#faad14" />;
  return <InfoCircleTwoTone twoToneColor="#1677ff" />;
}
