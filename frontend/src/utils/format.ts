// format helpers shared across cards. Imported as named exports.

export function formatUptime(seconds?: number | null): string | null {
  if (seconds == null) return null;
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (d > 0) return `${d}d ${h}h ${m}m`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

// formatTimestamp rewrites an ISO-8601 timestamp into a UI-friendly
// compact form: T → space, drop the trailing Z, truncate at seconds.
//   "2026-08-22T16:13:37.842Z"  →  "2026-08-22 16:13:37"
// Falls back to the raw string if parsing fails.
export function formatTimestamp(iso?: string | null): string {
  if (!iso) return '—';
  // Anchor on the first 'T' — anything before it is the date part.
  const tIdx = iso.indexOf('T');
  if (tIdx < 0) return iso;
  // Replace T with a single space.
  let s = iso.slice(0, tIdx) + ' ' + iso.slice(tIdx + 1);
  // Strip anything after the seconds (i.e. ".sss" and trailing zone).
  const dot = s.indexOf('.');
  if (dot >= 0) s = s.slice(0, dot);
  // Drop a trailing 'Z' (UTC marker) or any trailing zone marker
  // such as '+00:00'. Leaves naive-local-looking timestamps.
  if (s.endsWith('Z')) s = s.slice(0, -1);
  return s;
}

// RelativeTimeParts is a locale-neutral description of how long
// ago `iso` was relative to `now`. The caller (typically a React
// component with access to the i18n `t()` function) is responsible
// for translating the (unit, value) pair into a display string.
// Previously relativeTime inlined the Chinese strings — even when
// locale was 'en' the cards rendered "刚刚" / "秒前". Components
// now do `${value} ${t('time.${unit}.${value===1?'':'s'}.ago')}`.
export interface RelativeTimeParts {
  /** Whole number of `unit`s between iso and now, or null if
   *  iso is missing or unparseable. */
  value: number | null;
  /** 'second' | 'minute' | 'hour' | 'day'. */
  unit: 'second' | 'minute' | 'hour' | 'day' | 'now';
}

// relativeTimeParts turns a server-side timestamp into a
// locale-neutral (value, unit) pair so the UI layer can format it
// via the active dictionary instead of baking Chinese into the
// helper.
export function relativeTimeParts(iso?: string | null, now: number = Date.now()): RelativeTimeParts {
  if (!iso) return { value: null, unit: 'now' };
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return { value: null, unit: 'now' };
  const delta = Math.max(0, Math.floor((now - t) / 1000));
  if (delta < 5) return { value: 0, unit: 'now' };
  if (delta < 60) return { value: delta, unit: 'second' };
  if (delta < 3600) return { value: Math.floor(delta / 60), unit: 'minute' };
  if (delta < 86400) return { value: Math.floor(delta / 3600), unit: 'hour' };
  return { value: Math.floor(delta / 86400), unit: 'day' };
}

// relativeTime is the legacy wrapper that emits Chinese-only
// strings. New code should call relativeTimeParts + the i18n
// dictionary. Kept for callers that have not migrated yet (the
// t() plumbing needs a small refactor — tracked separately).
export function relativeTime(iso?: string | null, now: number = Date.now()): string {
  const p = relativeTimeParts(iso, now);
  if (p.value === null) return '—';
  switch (p.unit) {
    case 'now':    return '刚刚';
    case 'second': return `${p.value} 秒前`;
    case 'minute': return `${p.value} 分前`;
    case 'hour':   return `${p.value} 时前`;
    case 'day':    return `${p.value} 天前`;
  }
}
