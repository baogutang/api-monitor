import { usePreferences } from '@/contexts/PreferencesContext'

export function LanguageSwitch({ className = '' }: { className?: string }) {
  const { preferences, setLocaleMode } = usePreferences()
  return (
    <div className={`locale-switch ${className}`} aria-label="Language">
      <button
        type="button"
        className={preferences.localeMode !== 'en' ? 'active' : ''}
        onClick={() => setLocaleMode('zh-CN')}
      >
        中文
      </button>
      <button
        type="button"
        className={preferences.localeMode === 'en' ? 'active' : ''}
        onClick={() => setLocaleMode('en')}
      >
        EN
      </button>
    </div>
  )
}
