import { cn } from '@/lib/cn'
import type { statusTone } from '@/lib/format'
import type { HealthStatus } from '@/lib/types'

export function StatusDot({ tone }: { tone: ReturnType<typeof statusTone> }) {
  const mapped = tone === 'crit' ? 'dot-crit' : tone === 'warn' ? 'dot-warn' : tone === 'ok' ? 'dot-ok' : 'dot-muted'
  return <span className={cn('dot', mapped)} />
}

export function StatusBadge({
  status,
  label,
}: {
  status: HealthStatus
  label: string
}) {
  const tone =
    status === 'healthy'
      ? 'badge-ok'
      : status === 'warning'
        ? 'badge-warn'
        : status === 'critical'
          ? 'badge-crit'
          : ''
  return <span className={cn('badge', tone)}>{label}</span>
}

export function CapabilityBadge({ label }: { label: string }) {
  return <span className="badge badge-accent">{label}</span>
}
