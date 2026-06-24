import { cn } from '@/lib/cn'
import { Loader2 } from 'lucide-react'
import type { ButtonHTMLAttributes } from 'react'

type Variant = 'default' | 'primary' | 'ghost' | 'danger'
type Size = 'sm' | 'md' | 'icon'

type Props = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: Variant
  size?: Size
  loading?: boolean
}

export function Button({
  className,
  variant = 'default',
  size = 'md',
  loading,
  children,
  disabled,
  ...props
}: Props) {
  return (
    <button
      className={cn(
        'btn',
        variant === 'primary' && 'btn-primary',
        variant === 'ghost' && 'btn-ghost',
        variant === 'danger' && 'text-crit border-crit/30',
        size === 'sm' && 'btn-sm',
        size === 'icon' && 'btn-icon',
        className,
      )}
      disabled={disabled || loading}
      {...props}
    >
      {loading && <Loader2 size={14} className="animate-spin" />}
      {children}
    </button>
  )
}
