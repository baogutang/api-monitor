export type ThemeMode = 'light' | 'dark'
export type UiThemeId = 'mission'
export type LocaleId = 'zh-CN' | 'zh-TW' | 'en'
export type LocaleMode = LocaleId

export type Preferences = {
  themeMode: ThemeMode
  uiTheme: UiThemeId
  localeMode: LocaleMode
}

export const PREFS_KEY = 'api_monitor_preferences_v1'

export const defaultPreferences: Preferences = {
  themeMode: 'light',
  uiTheme: 'mission',
  localeMode: 'zh-CN',
}

export function loadPreferences(): Preferences {
  try {
    const raw = localStorage.getItem(PREFS_KEY)
    if (!raw) return defaultPreferences
    const parsed = JSON.parse(raw) as Partial<Preferences>
    const themeMode = parsed.themeMode === 'dark' ? 'dark' : 'light'
    const localeMode =
      parsed.localeMode === 'zh-TW' || parsed.localeMode === 'en'
        ? parsed.localeMode
        : 'zh-CN'
    return {
      ...defaultPreferences,
      ...parsed,
      themeMode,
      localeMode,
      uiTheme: 'mission',
    }
  } catch {
    return defaultPreferences
  }
}

export function savePreferences(prefs: Preferences) {
  localStorage.setItem(PREFS_KEY, JSON.stringify(prefs))
}

export function resolveThemeMode(mode: ThemeMode): 'light' | 'dark' {
  return mode
}

export function resolveLocale(mode: LocaleMode): LocaleId {
  return mode
}

export type ThemeTokens = Record<string, string>

const darkOps: ThemeTokens = {
  '--bg-void': '#0E100D',
  '--bg-panel': 'rgba(14, 16, 13, 0.92)',
  '--bg-surface': 'rgba(245, 243, 236, 0.04)',
  '--bg-elevated': 'rgba(245, 243, 236, 0.07)',
  '--bg-hover': 'rgba(255, 255, 255, 0.05)',
  '--brand': '#6F93D2',
  '--brand-bright': '#91AFDD',
  '--brand-dim': 'rgba(111, 147, 210, 0.14)',
  '--brand-glow': 'rgba(111, 147, 210, 0.14)',
  '--ok': '#34D399',
  '--ok-dim': 'rgba(52, 211, 153, 0.1)',
  '--warn': '#FBBF24',
  '--warn-dim': 'rgba(251, 191, 36, 0.12)',
  '--crit': '#F87171',
  '--crit-dim': 'rgba(248, 113, 113, 0.12)',
  '--text-1': '#ECE8DC',
  '--text-2': '#CFC8B8',
  '--text-3': '#958E7F',
  '--text-4': '#676256',
  '--border': 'rgba(224, 218, 202, 0.09)',
  '--border-strong': 'rgba(224, 218, 202, 0.18)',
  '--stripe-shadow': 'rgba(0, 0, 0, 0.24)',
  '--grid-color': 'rgba(224, 218, 202, 0.04)',
  '--glow-a': 'rgba(111, 147, 210, 0.05)',
  '--glow-b': 'rgba(109, 91, 53, 0.05)',
}

const lightDaylight: ThemeTokens = {
  '--bg-void': '#F6F4EC',
  '--bg-panel': 'rgba(246, 244, 236, 0.9)',
  '--bg-surface': 'rgba(250, 249, 243, 0.78)',
  '--bg-elevated': 'rgba(255, 255, 250, 0.9)',
  '--bg-hover': 'rgba(59, 55, 44, 0.045)',
  '--brand': '#4F79C5',
  '--brand-bright': '#2F63B7',
  '--brand-dim': 'rgba(79, 121, 197, 0.12)',
  '--brand-glow': 'rgba(79, 121, 197, 0.14)',
  '--ok': '#2F6B2F',
  '--ok-dim': 'rgba(5, 150, 105, 0.1)',
  '--warn': '#7A5B10',
  '--warn-dim': 'rgba(183, 121, 31, 0.12)',
  '--crit': '#A94444',
  '--crit-dim': 'rgba(220, 38, 38, 0.11)',
  '--text-1': '#171713',
  '--text-2': '#3F3B32',
  '--text-3': '#777166',
  '--text-4': '#9A9488',
  '--border': 'rgba(62, 58, 48, 0.13)',
  '--border-strong': 'rgba(62, 58, 48, 0.22)',
  '--stripe-shadow': 'rgba(62, 58, 48, 0.08)',
  '--grid-color': 'rgba(62, 58, 48, 0.05)',
  '--glow-a': 'rgba(79, 121, 197, 0.08)',
  '--glow-b': 'rgba(119, 113, 102, 0.08)',
}

const uiThemes: Record<
  UiThemeId,
  { dark: ThemeTokens; light?: ThemeTokens; labelKey: string }
> = {
  mission: { dark: darkOps, light: lightDaylight, labelKey: 'theme.mission' },
}

export function applyTheme(uiTheme: UiThemeId, resolved: 'light' | 'dark') {
  const preset = uiThemes[uiTheme] ?? uiThemes.mission
  const tokens = resolved === 'light' && preset.light ? preset.light : preset.dark
  const root = document.documentElement
  root.setAttribute('data-theme', resolved)
  root.setAttribute('data-theme-mode', resolved)
  root.setAttribute('data-ui-theme', uiTheme)
  Object.entries(tokens).forEach(([k, v]) => root.style.setProperty(k, v))
  root.style.setProperty('--shadow-elevated', resolved === 'dark' ? '0 20px 60px rgba(0, 0, 0, 0.24)' : '0 22px 70px rgba(15, 23, 42, 0.1)')
  root.style.setProperty('--shadow-glow', '0 0 0 1px rgba(56, 189, 248, 0.12)')
  root.style.setProperty('--font-sans', "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif")
  root.style.setProperty('--font-mono', "'JetBrains Mono', 'SFMono-Regular', Consolas, ui-monospace, monospace")
  root.style.setProperty('--radius-sm', '8px')
  root.style.setProperty('--radius-md', '10px')
  root.style.setProperty('--radius-lg', '14px')
  root.style.setProperty('--radius-xl', '18px')
}
