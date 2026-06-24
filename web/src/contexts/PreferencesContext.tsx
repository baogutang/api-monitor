import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import { useTranslation } from 'react-i18next'
import {
  applyTheme,
  defaultPreferences,
  loadPreferences,
  resolveLocale,
  resolveThemeMode,
  savePreferences,
  type LocaleMode,
  type Preferences,
  type ThemeMode,
} from '@/lib/theme'

type PreferencesContextValue = {
  preferences: Preferences
  resolvedTheme: 'light' | 'dark'
  resolvedLocale: 'zh-CN' | 'zh-TW' | 'en'
  setThemeMode: (mode: ThemeMode) => void
  setLocaleMode: (mode: LocaleMode) => void
  updatePreferences: (patch: Partial<Preferences>) => void
}

const PreferencesContext = createContext<PreferencesContextValue | null>(null)

export function PreferencesProvider({ children }: { children: ReactNode }) {
  const { i18n } = useTranslation()
  const [preferences, setPreferences] = useState<Preferences>(loadPreferences)
  const [resolvedTheme, setResolvedTheme] = useState<'light' | 'dark'>(() =>
    resolveThemeMode(preferences.themeMode),
  )
  const resolvedLocale = useMemo(
    () => resolveLocale(preferences.localeMode),
    [preferences.localeMode],
  )

  const applyAll = useCallback((prefs: Preferences) => {
    const theme = resolveThemeMode(prefs.themeMode)
    setResolvedTheme(theme)
    applyTheme(prefs.uiTheme, theme)
    const locale = resolveLocale(prefs.localeMode)
    void i18n.changeLanguage(locale)
  }, [i18n])

  useEffect(() => {
    applyAll(preferences)
  }, [preferences, applyAll])

  const update = useCallback((patch: Partial<Preferences>) => {
    setPreferences((prev) => {
      const next = { ...prev, ...patch }
      savePreferences(next)
      return next
    })
  }, [])

  const value = useMemo<PreferencesContextValue>(
    () => ({
      preferences,
      resolvedTheme,
      resolvedLocale,
      setThemeMode: (themeMode) => update({ themeMode }),
      setLocaleMode: (localeMode) => update({ localeMode }),
      updatePreferences: update,
    }),
    [preferences, resolvedTheme, resolvedLocale, update],
  )

  return (
    <PreferencesContext.Provider value={value}>{children}</PreferencesContext.Provider>
  )
}

export function usePreferences() {
  const ctx = useContext(PreferencesContext)
  if (!ctx) throw new Error('usePreferences must be used within PreferencesProvider')
  return ctx
}

export { defaultPreferences }
