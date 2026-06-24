import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '@/contexts/AuthContext'
import { Button } from '@/components/ui/Button'
import { LanguageSwitch } from '@/components/LanguageSwitch'
import { BrandLogo } from '@/components/BrandLogo'

export function SetupPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { setup } = useAuth()
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      await setup(email, password, name || undefined)
      navigate('/login', { replace: true })
    } catch (err) {
      setError(err instanceof Error ? err.message : t('errors.loadFailed'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="auth-page">
      <LanguageSwitch className="auth-language-switch" />
      <aside className="auth-brand">
        <div className="auth-logo">
          <div className="auth-logo-icon"><BrandLogo /></div>
          <div>
            <div className="auth-logo-name">API Monitor</div>
            <div className="text-xs text-muted mono">first-run setup</div>
          </div>
        </div>
        <h2 className="auth-brand-title">先创建第一个管理员，再进入控制台。</h2>
        <p className="auth-brand-copy">初始化只会执行一次。之后所有实例、规则、通知渠道都在页面配置，不需要改配置文件。</p>
      </aside>
      <main className="auth-form-area">
        <section className="auth-card">
          <div className="auth-logo">
            <div className="auth-logo-icon"><BrandLogo /></div>
            <div className="auth-logo-name">API Monitor</div>
          </div>
          <h1>{t('auth.setupTitle')}</h1>
          <p>{t('auth.setupDesc')}</p>
          <form onSubmit={onSubmit}>
            <div className="field">
              <label>{t('auth.adminName')}</label>
              <input className="input" value={name} onChange={(e) => setName(e.target.value)} />
            </div>
            <div className="field">
              <label>{t('auth.email')}</label>
              <input
                className="input"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
              />
            </div>
            <div className="field">
              <label>{t('auth.password')}</label>
              <input
                className="input"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                minLength={8}
              />
            </div>
            {error && <p className="text-crit text-sm mb-3">{error}</p>}
            <Button type="submit" variant="primary" className="w-full" loading={loading}>
              {t('auth.submitSetup')}
            </Button>
          </form>
        </section>
      </main>
    </div>
  )
}
