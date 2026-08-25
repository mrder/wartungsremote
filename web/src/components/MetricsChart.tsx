interface Point {
  t: number // ms epoch
  v: number
}

interface Props {
  title: string
  points: Point[]
  unit: string
  max?: number
  color?: string
}

const WIDTH = 640
const HEIGHT = 160
const PAD_LEFT = 40
const PAD_BOTTOM = 20
const PAD_TOP = 10

export default function MetricsChart({ title, points, unit, max, color = 'var(--accent, #4a9eff)' }: Props) {
  if (points.length === 0) {
    return (
      <div className="metrics-chart">
        <h4>{title}</h4>
        <p>No data in this range.</p>
      </div>
    )
  }

  const sorted = [...points].sort((a, b) => a.t - b.t)
  const minT = sorted[0].t
  const maxT = sorted[sorted.length - 1].t
  const spanT = Math.max(maxT - minT, 1)
  const maxV = max ?? Math.max(...sorted.map((p) => p.v), 1)

  const plotW = WIDTH - PAD_LEFT - 10
  const plotH = HEIGHT - PAD_TOP - PAD_BOTTOM

  function x(t: number) {
    return PAD_LEFT + ((t - minT) / spanT) * plotW
  }
  function y(v: number) {
    return PAD_TOP + plotH - (Math.min(v, maxV) / maxV) * plotH
  }

  const path = sorted.map((p, i) => `${i === 0 ? 'M' : 'L'}${x(p.t).toFixed(1)},${y(p.v).toFixed(1)}`).join(' ')
  const last = sorted[sorted.length - 1]

  return (
    <div className="metrics-chart">
      <h4>
        {title} <span className="metrics-chart-last">{last.v.toFixed(1)}{unit}</span>
      </h4>
      <svg viewBox={`0 0 ${WIDTH} ${HEIGHT}`} width="100%" height={HEIGHT} preserveAspectRatio="none">
        <line x1={PAD_LEFT} y1={PAD_TOP} x2={PAD_LEFT} y2={PAD_TOP + plotH} stroke="var(--border, #444)" strokeWidth="1" />
        <line x1={PAD_LEFT} y1={PAD_TOP + plotH} x2={WIDTH - 10} y2={PAD_TOP + plotH} stroke="var(--border, #444)" strokeWidth="1" />
        <text x="2" y={PAD_TOP + 6} fontSize="10" fill="var(--muted, #888)">{maxV.toFixed(0)}{unit}</text>
        <text x="2" y={PAD_TOP + plotH} fontSize="10" fill="var(--muted, #888)">0</text>
        <path d={path} fill="none" stroke={color} strokeWidth="1.5" />
      </svg>
      <div className="metrics-chart-range">
        <span>{new Date(minT).toLocaleString()}</span>
        <span>{new Date(maxT).toLocaleString()}</span>
      </div>
    </div>
  )
}
