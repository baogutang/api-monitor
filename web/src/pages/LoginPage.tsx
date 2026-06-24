import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '@/contexts/AuthContext'
import { Button } from '@/components/ui/Button'
import { LanguageSwitch } from '@/components/LanguageSwitch'
import { BrandLogo } from '@/components/BrandLogo'

export function LoginPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { login } = useAuth()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      await login(email, password)
      navigate('/')
    } catch (err) {
      setError(err instanceof Error ? err.message : t('errors.loadFailed'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="auth-page">
      <LanguageSwitch className="auth-language-switch" />
      <AuthBrand />
      <main className="auth-form-area">
        <section className="auth-card">
          <div className="auth-logo">
            <div className="auth-logo-icon"><BrandLogo /></div>
            <div className="auth-logo-name">API Monitor</div>
          </div>
          <h1>{t('auth.loginTitle')}</h1>
          <p>{t('app.tagline')}</p>
          <form onSubmit={onSubmit}>
            <div className="field">
              <label>{t('auth.email')}</label>
              <input
                className="input"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
                autoComplete="email"
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
                autoComplete="current-password"
              />
            </div>
            {error && <p className="text-crit text-sm mb-3">{error}</p>}
            <Button type="submit" variant="primary" className="w-full" loading={loading}>
              {t('auth.login')}
            </Button>
          </form>
        </section>
      </main>
    </div>
  )
}

function AuthBrand() {
  return (
    <aside className="auth-brand">
      <div className="auth-logo">
        <div className="auth-logo-icon"><BrandLogo /></div>
        <div>
          <div className="auth-logo-name">API Monitor</div>
          <div className="text-xs text-muted mono">relay quota observability</div>
        </div>
      </div>
      <h2 className="auth-brand-title">余额告警控制台</h2>
      <p className="auth-brand-copy">面向 API 中转站与多上游团队，把普通用户账号、官方 API Key、手动订阅和通知路由收进同一套运维视图。</p>
      <div className="auth-brand-grid">
        <span>余额扫描</span>
        <span>阈值规则</span>
        <span>短信/邮件</span>
        <span>电话升级</span>
      </div>
    </aside>
  )
}
