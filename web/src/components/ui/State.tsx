import { useTranslation } from 'react-i18next'
import { Button } from './Button'

export function EmptyState({ message }: { message?: string }) {
  const { t } = useTranslation()
  return <div className="empty-state">{message ?? t('common.noData')}</div>
}

export function ErrorState({
  message,
  onRetry,
}: {
  message?: string
  onRetry?: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className="error-state">
      <p>{message ?? t('errors.loadFailed')}</p>
      {onRetry && (
        <Button className="mt-4" onClick={onRetry}>
          {t('common.retry')}
        </Button>
      )}
    </div>
  )
}

export function LoadingSkeleton({ rows = 5 }: { rows?: number }) {
  return (
    <div className="flex flex-col gap-2 p-4">
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="skeleton h-10 w-full" />
      ))}
    </div>
  )
}
