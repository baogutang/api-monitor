type RingGaugeProps = {
  value: number
  size?: number
  stroke?: number
  label: string
  sublabel?: string
  tone?: 'brand' | 'ok' | 'warn' | 'crit'
}

const TONES = {
  brand: { stroke: 'var(--brand)', glow: 'var(--brand-glow)' },
  ok: { stroke: 'var(--ok)', glow: 'rgba(62,207,142,0.45)' },
  warn: { stroke: 'var(--warn)', glow: 'rgba(245,165,36,0.4)' },
  crit: { stroke: 'var(--crit)', glow: 'rgba(255,107,157,0.45)' },
}

export function RingGauge({
  value,
  size = 120,
  stroke = 8,
  label,
  sublabel,
  tone = 'brand',
}: RingGaugeProps) {
  const r = (size - stroke) / 2
  const c = 2 * Math.PI * r
  const offset = c - (Math.min(100, Math.max(0, value)) / 100) * c
  const t = TONES[tone]
  const id = `glow-${tone}`

  return (
    <div className="ring-gauge" style={{ width: size, height: size }}>
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`}>
        <defs>
          <filter id={id}>
            <feGaussianBlur stdDeviation="2" result="blur" />
            <feMerge>
              <feMergeNode in="blur" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>
        </defs>
        <circle
          cx={size / 2}
          cy={size / 2}
          r={r}
          fill="none"
          stroke="var(--border)"
          strokeWidth={stroke}
        />
        <circle
          cx={size / 2}
          cy={size / 2}
          r={r}
          fill="none"
          stroke={t.stroke}
          strokeWidth={stroke}
          strokeLinecap="round"
          strokeDasharray={c}
          strokeDashoffset={offset}
          transform={`rotate(-90 ${size / 2} ${size / 2})`}
          filter={`url(#${id})`}
          style={{ transition: 'stroke-dashoffset 0.8s var(--ease-out)' }}
        />
      </svg>
      <div className="ring-gauge-center">
        <div className="ring-gauge-value mono">{value}%</div>
        <div className="ring-gauge-label">{label}</div>
        {sublabel && <div className="ring-gauge-sub mono">{sublabel}</div>}
      </div>
    </div>
  )
}
