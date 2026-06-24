import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { alertsApi } from '@/api/services'
import { AppShell } from '@/components/layout/AppShell'
import { Button } from '@/components/ui/Button'
import { StatusDot } from '@/components/ui/Badge'
import { ErrorState, LoadingSkeleton } from '@/components/ui/State'
import { formatRelative } from '@/lib/format'
import { usePreferences } from '@/contexts/PreferencesContext'

export function AlertsPage() {
  const { t } = useTranslation()
  const { resolvedLocale } = usePreferences()
  const qc = useQueryClient()

  const list = useQuery({
    queryKey: ['alerts'],
    queryFn: () => alertsApi.list({ limit: 50 }),
    refetchInterval: 15000,
  })

  const ackMut = useMutation({
    mutationFn: (id: string) => alertsApi.ack(id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['alerts'] }),
  })
  const silenceMut = useMutation({
    mutationFn: (id: string) => alertsApi.silence(id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['alerts'] }),
  })
  const resolveMut = useMutation({
    mutationFn: (id: string) => alertsApi.resolve(id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['alerts'] }),
  })

  return (
    <AppShell
      title={t('alerts.title')}
      description={t('alerts.desc')}
      onRefresh={() => void list.refetch()}
      refreshing={list.isFetching}
    >
      {list.isLoading ? (
        <LoadingSkeleton />
      ) : list.isError ? (
        <ErrorState onRetry={() => void list.refetch()} />
      ) : (list.data?.items ?? []).length === 0 ? (
        <div className="empty-state">
          <h3>暂无告警事件</h3>
          <p>当余额、套餐窗口或扫描健康命中规则后，事件会显示在这里。</p>
        </div>
      ) : (
        <div className="data-table-wrap">
          <table className="data-table">
            <thead>
              <tr>
                <th>{t('alerts.severity')}</th>
                <th>{t('assets.name')}</th>
                <th>{t('common.status')}</th>
                <th>{t('alerts.openedAt')}</th>
                <th>{t('common.actions')}</th>
              </tr>
            </thead>
            <tbody>
              {(list.data?.items ?? []).map((a) => (
                <tr key={a.id}>
                  <td>
                    <div className="flex items-center gap-2">
                      <StatusDot
                        tone={
                          a.severity === 'critical' || a.severity === 'phone'
                            ? 'crit'
                            : a.severity === 'warning'
                              ? 'warn'
                              : 'muted'
                        }
                      />
                      {t(`severity.${a.severity}`)}
                    </div>
                  </td>
                  <td>
                    <div className="font-medium">{a.title}</div>
                    <div className="text-xs text-text-4">{a.message}</div>
                  </td>
                  <td>{t(`status.${a.status}`)}</td>
                  <td className="text-xs">{formatRelative(a.openedAt, resolvedLocale)}</td>
                  <td>
                    {a.status === 'open' && (
                      <div className="flex gap-1">
                        <Button size="sm" onClick={() => ackMut.mutate(a.id)}>
                          {t('common.acknowledge')}
                        </Button>
                        <Button size="sm" variant="ghost" onClick={() => silenceMut.mutate(a.id)}>
                          {t('common.silence')}
                        </Button>
                        <Button size="sm" variant="ghost" onClick={() => resolveMut.mutate(a.id)}>
                          {t('common.resolve')}
                        </Button>
                      </div>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </AppShell>
  )
}
