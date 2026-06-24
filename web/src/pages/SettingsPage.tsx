import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { settingsApi, versionApi } from '@/api/services'
import { AppShell } from '@/components/layout/AppShell'
import { BrandLogo } from '@/components/BrandLogo'
import { Button } from '@/components/ui/Button'
import { Card, CardBody, CardHeader } from '@/components/ui/Card'
import { ErrorState, LoadingSkeleton } from '@/components/ui/State'
import { usePreferences } from '@/contexts/PreferencesContext'
import type { LocaleMode } from '@/lib/theme'
import { useState, useEffect } from 'react'

export function SettingsPage() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const {
    preferences,
    setThemeMode,
    setLocaleMode,
  } = usePreferences()

  const settings = useQuery({ queryKey: ['settings'], queryFn: settingsApi.get })
  const version = useQuery({ queryKey: ['version'], queryFn: versionApi.current })

  const [scanInterval, setScanInterval] = useState(300)
  const [retention, setRetention] = useState(90)
  const [logoSvg, setLogoSvg] = useState('')

  useEffect(() => {
    if (settings.data?.defaultScanIntervalSeconds != null) {
      setScanInterval(settings.data.defaultScanIntervalSeconds)
    }
    if (settings.data?.retentionDays != null) {
      setRetention(settings.data.retentionDays)
    }
    if (typeof settings.data?.brandingLogoSvg === 'string') {
      setLogoSvg(settings.data.brandingLogoSvg)
    }
  }, [settings.data])

  const saveMut = useMutation({
    mutationFn: () =>
      settingsApi.patch({
        defaultScanIntervalSeconds: scanInterval,
        retentionDays: retention,
        brandingLogoSvg: logoSvg.trim(),
      }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['settings'] }),
  })

  const checkVersionMut = useMutation({
    mutationFn: () => versionApi.check(),
    onSuccess: (data) => {
      qc.setQueryData(['version'], data)
    },
  })

  const updateMut = useMutation({
    mutationFn: () => versionApi.update(),
  })

  return (
    <AppShell
      title={t('settings.title')}
      description={t('settings.desc')}
      onRefresh={() => {
        void settings.refetch()
        void version.refetch()
      }}
      refreshing={settings.isFetching || version.isFetching}
    >
      <div className="settings-layout">
        <Card className="settings-hero-card">
          <CardBody>
            <div className="settings-brand-preview">
              <div className="settings-logo-orbit">
                <BrandLogo customSvg={logoSvg} />
              </div>
              <div>
                <div className="settings-kicker">API Monitor</div>
                <h2>首页展示与品牌标识</h2>
                <p>侧边栏、登录页、初始化页和浏览器图标默认使用 N1KO mark。这里保存 SVG 后，登录后的控制台会同步使用自定义标识。</p>
              </div>
            </div>
            <div className="settings-mini-dashboard" aria-hidden="true">
              <span>余额扫描</span>
              <span>官方账号窗口</span>
              <span>通知测试</span>
            </div>
          </CardBody>
        </Card>

        <Card>
          <CardHeader>
            <span className="card-title">{t('settings.appearance')}</span>
          </CardHeader>
          <CardBody className="flex flex-col gap-4">
            <div className="field mb-0">
              <label>{t('theme.mode')}</label>
              <div className="flex gap-2 flex-wrap">
                {(
                  [
                    ['light', 'theme.modeLight'],
                    ['dark', 'theme.modeDark'],
                  ] as const
                ).map(([mode, labelKey]) => (
                  <Button
                    key={mode}
                    size="sm"
                    variant={preferences.themeMode === mode ? 'primary' : 'default'}
                    onClick={() => setThemeMode(mode)}
                  >
                    {t(labelKey)}
                  </Button>
                ))}
              </div>
            </div>

            <div className="field mb-0">
              <label>{t('locale.title')}</label>
              <select
                className="select"
                value={preferences.localeMode}
                onChange={(e) => setLocaleMode(e.target.value as LocaleMode)}
              >
                <option value="zh-CN">{t('locale.zh-CN')}</option>
                <option value="zh-TW">{t('locale.zh-TW')}</option>
                <option value="en">{t('locale.en')}</option>
              </select>
            </div>

            <div className="field mb-0">
              <label>自定义 Logo SVG</label>
              <input
                className="input"
                type="file"
                accept=".svg,image/svg+xml"
                onChange={(event) => {
                  const file = event.target.files?.[0]
                  if (!file) return
                  void file.text().then((text) => setLogoSvg(text))
                }}
              />
              <textarea
                className="textarea mono mt-2"
                value={logoSvg}
                onChange={(event) => setLogoSvg(event.target.value)}
                placeholder="可粘贴 SVG 源码；留空则使用默认 N1KO mark"
              />
            </div>
          </CardBody>
        </Card>

        <Card>
          <CardHeader>
            <span className="card-title">{t('settings.system')}</span>
          </CardHeader>
          <CardBody>
            {settings.isLoading ? (
              <LoadingSkeleton rows={2} />
            ) : settings.isError ? (
              <ErrorState onRetry={() => void settings.refetch()} />
            ) : (
              <>
                <div className="field">
                  <label>{t('settings.defaultScanInterval')}</label>
                  <input
                    className="input"
                    type="number"
                    value={scanInterval}
                    onChange={(e) => setScanInterval(Number(e.target.value))}
                  />
                </div>
                <div className="field">
                  <label>{t('settings.retentionDays')}</label>
                  <input
                    className="input"
                    type="number"
                    value={retention}
                    onChange={(e) => setRetention(Number(e.target.value))}
                  />
                </div>
                <Button variant="primary" loading={saveMut.isPending} onClick={() => saveMut.mutate()}>
                  {t('common.save')}
                </Button>
              </>
            )}
          </CardBody>
        </Card>

        <Card>
          <CardHeader>
            <span className="card-title">{t('settings.version')}</span>
          </CardHeader>
          <CardBody>
            {version.isLoading ? (
              <LoadingSkeleton rows={2} />
            ) : version.isError ? (
              <ErrorState onRetry={() => void version.refetch()} />
            ) : (
              <div className="flex flex-col gap-3 text-sm">
                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <div className="text-text-4">{t('settings.currentVersion')}</div>
                    <div className="mono">{version.data?.version ?? '—'}</div>
                  </div>
                  <div>
                    <div className="text-text-4">{t('settings.latestVersion')}</div>
                    <div className="mono">{version.data?.latestVersion ?? '—'}</div>
                  </div>
                  <div>
                    <div className="text-text-4">{t('settings.repository')}</div>
                    <div className="mono">{version.data?.repository ?? '—'}</div>
                  </div>
                  <div>
                    <div className="text-text-4">{t('settings.selfUpdate')}</div>
                    <div>{version.data?.selfUpdateEnabled ? t('common.enabled') : t('common.disabled')}</div>
                  </div>
                </div>
                {version.data?.error && <div className="text-warning">{version.data.error}</div>}
                {version.data?.latestUrl && (
                  <a className="text-brand underline" href={version.data.latestUrl} target="_blank" rel="noreferrer">
                    {t('settings.releaseNotes')}
                  </a>
                )}
                {version.data?.updateAvailable && (
                  <div className="text-warning">{t('settings.updateAvailable')}</div>
                )}
                {updateMut.isError && (
                  <div className="text-danger">{(updateMut.error as Error).message}</div>
                )}
                {updateMut.data?.output && (
                  <pre className="mono text-xs overflow-auto rounded-md border border-line bg-surface-2 p-3">
                    {updateMut.data.output}
                  </pre>
                )}
                <div className="flex gap-2">
                  <Button loading={checkVersionMut.isPending} onClick={() => checkVersionMut.mutate()}>
                    {t('settings.checkUpdate')}
                  </Button>
                  <Button
                    variant="primary"
                    disabled={!version.data?.selfUpdateEnabled}
                    loading={updateMut.isPending}
                    onClick={() => updateMut.mutate()}
                  >
                    {t('settings.runUpdate')}
                  </Button>
                </div>
              </div>
            )}
          </CardBody>
        </Card>
      </div>
    </AppShell>
  )
}
