// Stand-alone test runner for `state/alerts.ts`. Compiles the
// .ts to a sibling .mjs via tsc, then runs the .mjs with
// node --test. Avoids vitest/vite plugin peer-dep churn by
// using only the bare `tsc` that's already required for the
// project's `typecheck` script.
//
// Run via `node scripts/run-tests.mjs` from frontend/. The
// helper compiles the source if the .mjs is missing or older
// than the .ts.
//
// Add new alert rules to src/state/alerts.ts and matching
// tests below; the .mjs is auto-regenerated.

import { execFileSync } from 'node:child_process';
import { existsSync, statSync, readFileSync, writeFileSync, mkdirSync } from 'node:fs';
import { dirname, relative, resolve } from 'node:path';
import { test } from 'node:test';
import assert from 'node:assert/strict';

const HERE = '/Users/letmlook/spotter/frontend';
process.chdir(HERE);
const SRC = resolve(HERE, 'src/state/alerts.ts');
const OUT_DIR = resolve(HERE, '.test-build');
const OUT_JS = resolve(OUT_DIR, 'alerts.js');

mkdirSync(OUT_DIR, { recursive: true });

const srcMtime = statSync(SRC).mtimeMs;
const outExists = existsSync(OUT_JS);
const outMtime = outExists ? statSync(OUT_JS).mtimeMs : 0;
if (!outExists || outMtime < srcMtime) {
  execFileSync(
    resolve(HERE, 'node_modules/typescript/bin/tsc'),
    [
      SRC,
      '--outDir', OUT_DIR,
      '--target', 'es2022',
      '--module', 'es2022',
      '--moduleResolution', 'node',
      '--esModuleInterop',
      '--skipLibCheck',
    ],
    { stdio: 'inherit' },
  );
}

const mod = await import(OUT_JS);
const {
  evaluate,
  DEFAULT_OFFLINE_THRESHOLD_MS,
  HIGH_CPU_PERCENT,
  HIGH_MEM_PERCENT,
  HIGH_TEMP_CELSIUS,
} = mod;

const NOW = 1_700_000_000_000;
const ONE_MIN = 60 * 1000;

const device = (over = {}) => ({
  device_id: 'd1',
  ip: '10.0.0.1',
  online: true,
  last_seen_at: new Date(NOW).toISOString(),
  ...over,
});

test('healthy device: no alerts', () => {
  const a = evaluate(
    [device({ last_info: { basic: { hostname: 'h1' }, metrics: { at: new Date(NOW).toISOString(), cpu_percent: 10, mem_percent: 30, temp_celsius: 50 } } })],
    NOW,
  );
  assert.equal(a.length, 0);
});

test('offline past threshold: device-unreachable warning', () => {
  const a = evaluate([device({ last_seen_at: new Date(NOW - 6 * ONE_MIN).toISOString() })], NOW);
  assert.equal(a.length, 1);
  assert.equal(a[0].kind, 'device-unreachable');
  assert.equal(a[0].severity, 'warning');
});

test('offline past 2x threshold: critical', () => {
  const a = evaluate([device({ last_seen_at: new Date(NOW - 11 * ONE_MIN).toISOString() })], NOW);
  assert.equal(a[0].severity, 'critical');
});

test('offline but online=false: no alert', () => {
  const a = evaluate([device({ online: false })], NOW);
  assert.equal(a.length, 0);
});

test('custom offlineThresholdMs respected', () => {
  const a = evaluate([device({ last_seen_at: new Date(NOW - 90_000).toISOString() })], NOW, { offlineThresholdMs: 60_000 });
  assert.equal(a.length, 1);
});

test('high-cpu above 90% (TODO: needs agent schema)', () => {
  // The metric-based rules are not implemented in v0.5.x
  // because the agent's Metrics struct ships raw counters
  // (cpu_seconds_total, mem_total_bytes, cpu_temp_c) rather
  // than convenience fields. See the comment in alerts.ts.
  const a = evaluate([device({})], NOW);
  assert.equal(a.length, 0);
});

test('hostname fallback chain', () => {
  const byHost = evaluate([device({ last_seen_at: new Date(NOW - 6 * ONE_MIN).toISOString(), last_info: { basic: { hostname: 'h1' } } })], NOW);
  assert.equal(byHost[0].hostname, 'h1');
  const byIp = evaluate([device({ device_id: 'd2', ip: '10.0.0.2', last_seen_at: new Date(NOW - 6 * ONE_MIN).toISOString() })], NOW);
  assert.equal(byIp[0].hostname, '10.0.0.2');
});

test('default offline threshold is 5 minutes', () => {
  assert.equal(DEFAULT_OFFLINE_THRESHOLD_MS, 5 * 60 * 1000);
});

// The HIGH_* constants are kept for future metric-based rules
// but the test for them is skipped because no rule uses them
// today — see the comment in alerts.ts.
test('HIGH_* thresholds are exposed for future metric rules', () => {
  assert.equal(HIGH_CPU_PERCENT, 90);
  assert.equal(HIGH_MEM_PERCENT, 95);
  assert.equal(HIGH_TEMP_CELSIUS, 80);
});
