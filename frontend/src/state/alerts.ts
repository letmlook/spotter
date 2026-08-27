// Alert evaluation lives in the GUI (no backend alert engine in
// v0.5.x). The rationale:
//
//  1. Spotter-client is the only component that talks to every
//     device in the registry. Spotter-server aggregates presence
//     but doesn't (yet) pull per-device metrics.
//  2. Alerts are user-facing concerns (toast + banner), not
//     operator-facing ones (audit log). Putting them in the GUI
//     keeps the rule set opinionated per UX, not per fleet.
//  3. v1.0 may add backend-side alert rules; the Alert shape
//     here is the same shape the backend will emit, so a future
//     migration is `setAlerts(alertsFromServer)` not a schema
//     change.
//
// Each rule consumes the latest known state of one device
// (`DeviceEntry`) and returns zero or more `Alert` rows. The
// evaluator is pure: same input → same output. Determinism
// matters for tests and for the "did the alert change?" diff
// in the banner re-render.

export type AlertSeverity = 'info' | 'warning' | 'critical';

export type AlertKind =
  // The only rule wired in v0.5.x. Metric-based rules
  // (high-cpu / high-mem / high-temp) are documented below as
  // TODOs because the agent's Metrics struct (see
  // wailsjs/go/models.ts) ships raw counters (cpu_seconds_total
  // / mem_total_bytes / cpu_temp_c) not the percentage /
  // celsius convenience fields the metric rules assumed.
  // Implementing the metric rules requires either an agent-
  // side schema extension or client-side ratio computation;
  // either is a v0.6 effort.
  | 'device-unreachable';  // last_seen_at > N min ago while online=true

export interface Alert {
  /** Stable per-rule identity. Use `${deviceId}:${kind}` so
   *  React keys stay stable across re-evaluations. */
  id: string;
  deviceId: string;
  hostname: string;
  kind: AlertKind;
  severity: AlertSeverity;
  /** Human-readable, pre-localised message (we don't ship i18n
   *  for rule bodies in v0.5 — operators are fine with English
   *  status text; the banner / chip text uses i18n keys). */
  message: string;
  /** Server-ish timestamp when the rule first fired. */
  firstSeenAt: number; // unix millis
  /** Latest observation backing this alert. */
  observedValue?: number;
  observedUnit?: string;
}

/** Time after which a device that the registry still thinks is
 *  online is considered "unreachable". Configurable so tests
 *  can use a tighter window. The default (5 min) matches the
 *  scanner's pollFailures.threshold of 3 × 5s = 15s plus a
 *  margin; if a device has been silent for 5 min it's not
 *  coming back without operator action. */
export const DEFAULT_OFFLINE_THRESHOLD_MS = 5 * 60 * 1000;

/** Threshold above which a latest-sample metric is considered
 *  "high". These match the docs/operations.md § 1.4
 *  "Health thresholds" reference. */
export const HIGH_CPU_PERCENT = 90;
export const HIGH_MEM_PERCENT = 95;
export const HIGH_TEMP_CELSIUS = 80;

// deviceEntryMirror is the subset of registry.Entry the rules
// need. Defining a narrow interface here (rather than importing
// registry.Entry directly) keeps the rules testable without
// pulling in the Wails bridge types.
export interface DeviceEntryMirror {
  device_id: string;
  ip: string;
  online: boolean;
  last_seen_at: string; // RFC3339
  last_info?: {
    basic?: { hostname?: string };
  };
}

// evaluate runs every rule against the device and returns the
// full set of active alerts. Pure function — no side effects,
// no Date.now() leakage (caller passes `now` so tests are
// deterministic).
export function evaluate(
  devices: DeviceEntryMirror[],
  now: number,
  options?: {
    offlineThresholdMs?: number;
  },
): Alert[] {
  const offlineThresholdMs = options?.offlineThresholdMs
    ?? DEFAULT_OFFLINE_THRESHOLD_MS;
  const out: Alert[] = [];
  for (const d of devices) {
    out.push(...ruleOffline(d, now, offlineThresholdMs));
  }
  return out;
}

function hostname(d: DeviceEntryMirror): string {
  return d.last_info?.basic?.hostname || d.ip || d.device_id;
}

function ruleOffline(
  d: DeviceEntryMirror,
  now: number,
  thresholdMs: number,
): Alert[] {
  // Only fires when the registry says online=true but the last
  // observation is older than the threshold. A device that the
  // scanner has already flipped to online=false shows up in
  // the offline column of the sidebar, not as an alert.
  if (!d.online) return [];
  const t = Date.parse(d.last_seen_at);
  if (Number.isNaN(t)) return [];
  const age = now - t;
  if (age <= thresholdMs) return [];
  return [{
    id: `${d.device_id}:device-unreachable`,
    deviceId: d.device_id,
    hostname: hostname(d),
    kind: 'device-unreachable',
    severity: age > 2 * thresholdMs ? 'critical' : 'warning',
    message: `No heartbeat for ${Math.floor(age / 60000)} min`,
    firstSeenAt: now,
    observedValue: age,
    observedUnit: 'ms',
  }];
}

// Metric-based rules (high-cpu / high-mem / high-temp) were
// prototyped here but require either an agent-side Metrics
// schema extension (add cpu_percent / mem_percent / temp_celsius
// to internal/protocol/info.go) or a client-side diff-and-
// compute step. Both are out of scope for the C-6 frontend
// alert engine. The placeholder constants below are kept so
// future implementations have the thresholds in one place.

// function ruleHighCpu(d: DeviceEntryMirror, now: number): Alert[] {
//   const v = d.last_info?.metrics?.cpu_percent;
//   if (typeof v !== 'number') return [];
//   if (v <= HIGH_CPU_PERCENT) return [];
//   return [{ id: `${d.device_id}:high-cpu`, deviceId: d.device_id, ... }];
// }
