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

// relativeTime turns a server-side timestamp into a UI label like
// "12 秒前" or "3 分 5 秒前". Force-rendering depends on the caller
// ticking a clock (the cards call useEffect with setInterval).
export function relativeTime(iso?: string | null, now: number = Date.now()): string {
  if (!iso) return '—';
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return '—';
  const delta = Math.max(0, Math.floor((now - t) / 1000));
  if (delta < 5) return '刚刚';
  if (delta < 60) return `${delta} 秒前`;
  if (delta < 3600) {
    const m = Math.floor(delta / 60);
    const s = delta % 60;
    return s === 0 ? `${m} 分前` : `${m} 分 ${s} 秒前`;
  }
  if (delta < 86400) {
    const h = Math.floor(delta / 3600);
    const m = Math.floor((delta % 3600) / 60);
    return m === 0 ? `${h} 时前` : `${h} 时 ${m} 分前`;
  }
  const d = Math.floor(delta / 86400);
  const h = Math.floor((delta % 86400) / 3600);
  return h === 0 ? `${d} 天前` : `${d} 天 ${h} 时前`;
}
