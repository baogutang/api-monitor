import { useTranslation } from 'react-i18next'
import { Card } from '@/components/ui/Card'

type Props = {
  title: string
  message: string
  severity: 'warning' | 'critical' | 'info'
  targetName?: string
}

export function DingTalkCard({ title, message, severity, targetName }: Props) {
  const { t } = useTranslation()
  const color = severity === 'critical' ? '#ff6b9d' : severity === 'warning' ? '#f5a524' : '#6e6aff'

  return (
    <Card className="overflow-hidden max-w-md">
      <div style={{ height: 4, background: color }} />
      <div className="card-body">
        <div className="text-xs text-text-4 mb-2">{t('channelType.dingtalk')}</div>
        <div className="font-semibold text-base mb-2">{title}</div>
        <div className="text-sm text-text-3 whitespace-pre-wrap">{message}</div>
        {targetName && (
          <div className="mt-3 pt-3 border-t border-border text-xs text-text-4 mono">
            {targetName}
          </div>
        )}
      </div>
    </Card>
  )
}

export function FeishuCard({ title, message, severity, targetName }: Props) {
  const { t } = useTranslation()
  const bar = severity === 'critical' ? 'var(--crit)' : severity === 'warning' ? 'var(--warn)' : 'var(--brand)'

  return (
    <Card className="max-w-md">
      <div className="card-body">
        <div className="flex gap-3">
          <div className="w-1 rounded-full shrink-0" style={{ background: bar }} />
          <div>
            <div className="text-xs text-text-4 mb-1">{t('channelType.feishu')}</div>
            <div className="font-semibold mb-2">{title}</div>
            <div className="text-sm text-text-3">{message}</div>
            {targetName && <div className="mt-2 text-xs text-text-4 mono">{targetName}</div>}
          </div>
        </div>
      </div>
    </Card>
  )
}

export function WeComCard({ title, message, severity, targetName }: Props) {
  const { t } = useTranslation()
  return (
    <Card className="max-w-md bg-bg-elevated">
      <div className="card-body">
        <div className="text-xs text-text-4 mb-2">{t('channelType.wecom')}</div>
        <div
          className={`inline-block text-xs px-2 py-0.5 rounded mb-2 ${
            severity === 'critical' ? 'badge-crit' : severity === 'warning' ? 'badge-warn' : 'badge-brand'
          } badge`}
        >
          {t(`severity.${severity}`)}
        </div>
        <div className="font-semibold mb-2">{title}</div>
        <div className="text-sm text-text-3">{message}</div>
        {targetName && <div className="mt-2 text-xs text-text-4">{targetName}</div>}
      </div>
    </Card>
  )
}

export function PhoneAlert({ title, message }: Pick<Props, 'title' | 'message'>) {
  const { t } = useTranslation()
  return (
    <Card className="max-w-sm border-crit/30">
      <div className="card-body text-center">
        <div className="text-3xl mb-2">📞</div>
        <div className="text-xs text-text-4 mb-1">{t('channelType.phone')}</div>
        <div className="font-semibold text-crit mb-2">{title}</div>
        <div className="text-sm text-text-3">{message}</div>
      </div>
    </Card>
  )
}
