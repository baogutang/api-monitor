import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { scansApi } from '@/api/services'
import { AppShell } from '@/components/layout/AppShell'
import { ErrorState, LoadingSkeleton } from '@/components/ui/State'
import { formatDate } from '@/lib/format'
import { usePreferences } from '@/contexts/PreferencesContext'

export function ScansPage() {
  const { t } = useTranslation()
  const { resolvedLocale } = usePreferences()

  const list = useQuery({
    queryKey: ['scans'],
    queryFn: () => scansApi.list({ limit: 50 }),
  })

  return (
    <AppShell
      title={t('scans.title')}
      description={t('scans.desc')}
      onRefresh={() => void list.refetch()}
      refreshing={list.isFetching}
    >
      {list.isLoading ? (
        <LoadingSkeleton />
      ) : list.isError ? (
        <ErrorState onRetry={() => void list.refetch()} />
      ) : (list.data?.items ?? []).length === 0 ? (
        <div className="empty-state">
          <h3>暂无扫描记录</h3>
          <p>保存上游实例并执行扫描后，会在这里看到每次采集的状态、耗时和错误详情。</p>
        </div>
      ) : (
        <div className="data-table-wrap">
          <table className="data-table">
            <thead>
              <tr>
                <th>{t('common.status')}</th>
                <th>{t('scans.startedAt')}</th>
                <th>{t('scans.finishedAt')}</th>
                <th>{t('scans.error')}</th>
              </tr>
            </thead>
            <tbody>
              {(list.data?.items ?? []).map((run) => (
                <tr key={run.id}>
                  <td>{run.status}</td>
                  <td className="text-xs">{formatDate(run.startedAt, resolvedLocale)}</td>
                  <td className="text-xs">{formatDate(run.finishedAt, resolvedLocale)}</td>
                  <td className="text-xs text-crit">{run.error ?? '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </AppShell>
  )
}
