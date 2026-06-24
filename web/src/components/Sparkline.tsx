type SparklineProps = {
  data: number[]
  color: 'brand' | 'crit' | 'ok' | 'warn'
}

const COLORS = {
  brand: 'var(--brand)',
  crit: 'var(--crit)',
  ok: 'var(--ok)',
  warn: 'var(--warn)',
}

export function Sparkline({ data, color }: SparklineProps) {
  if (!data.length) return null
  const w = 220
  const h = 40
  const min = Math.min(...data)
  const max = Math.max(...data)
  const range = max - min || 1

  const points = data
    .map((v, i) => {
      const x = data.length === 1 ? w / 2 : (i / (data.length - 1)) * w
      const y = h - ((v - min) / range) * (h - 8) - 4
      return `${x},${y}`
    })
    .join(' ')

  const fillPoints = `0,${h} ${points} ${w},${h}`
  const id = `spark-${color}`

  return (
    <svg viewBox={`0 0 ${w} ${h}`} width="100%" height={h} preserveAspectRatio="none">
      <defs>
        <linearGradient id={id} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor={COLORS[color]} stopOpacity="0.35" />
          <stop offset="100%" stopColor={COLORS[color]} stopOpacity="0" />
        </linearGradient>
      </defs>
      <polygon points={fillPoints} fill={`url(#${id})`} />
      <polyline
        points={points}
        fill="none"
        stroke={COLORS[color]}
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}
