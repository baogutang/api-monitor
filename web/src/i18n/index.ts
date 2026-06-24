import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import zhCN from './locales/zh-CN.json'
import zhTW from './locales/zh-TW.json'
import en from './locales/en.json'

export const resources = {
  'zh-CN': { translation: zhCN },
  'zh-TW': { translation: zhTW },
  en: { translation: en },
} as const

export type AppLocale = keyof typeof resources

void i18n.use(initReactI18next).init({
  resources,
  lng: 'zh-CN',
  fallbackLng: 'en',
  interpolation: { escapeValue: false },
})

export default i18n
