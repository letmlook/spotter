// MetricSpark renders a tiny SVG sparkline for a single counter
// series. Used inside the device detail panel for CPU / memory /
// CPU temperature. No external deps — recharts is overkill for three
// series. The component is pure / presentational; parents compute
// the points array (delta across the last N samples).

import { useMemo } from 'react';

interface Props {
  label: string;
  unit?: string;
  points: number[]; // most-recent last
  max?: number;
  min?: number;
  color?: string;
  decimals?: number;
}

const W = 120;
const H = 32;
const PAD_X = 2;
const PAD_Y = 4;

export default function MetricSpark({ label, unit, points, color = '#69b1ff', decimals = 1 }: Props) {
  const path = useMemo(() => {
    if (points.length === 0) return '';
    const min = Math.min(...points);
    const max = Math.max(...points);
    const span = max - min || 1;
    return points
      .map((v, i) => {
        const x = PAD_X + (i / Math.max(1, points.length - 1)) * (W - PAD_X * 2);
        const y = H - PAD_Y - ((v - min) / span) * (H - PAD_Y * 2);
        return `${i === 0 ? 'M' : 'L'} ${x.toFixed(1)} ${y.toFixed(1)}`;
      })
      .join(' ');
  }, [points]);

  const last = points[points.length - 1];

  return (
    <div style={{ display: 'inline-flex', flexDirection: 'column', padding: 4 }}>
      <div style={{ fontSize: 11, color: 'var(--text-secondary, #888)' }}>{label}</div>
      <svg width={W} height={H} role="img" aria-label={`${label} sparkline`}>
        <path d={path} fill="none" stroke={color} strokeWidth="1.5" />
      </svg>
      <div style={{ fontSize: 12, color }}>
        {last === undefined ? '—' : last.toFixed(decimals)}{unit ? ` ${unit}` : ''}
      </div>
    </div>
  );
}
