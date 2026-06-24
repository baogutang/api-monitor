import { siAnthropic, siGmail, siGooglegemini, siGoogle, siWechat } from 'simple-icons'
import type { ProviderKind } from '@/lib/types'

type BrandKey =
  | ProviderKind
  | 'openai'
  | 'anthropic'
  | 'gemini'
  | 'google'
  | 'dingtalk'
  | 'feishu'
  | 'wecom'
  | 'email'
  | 'newapi'
  | 'sub2api'

const iconMap = {
  anthropic: siAnthropic,
  anthropic_key: siAnthropic,
  anthropic_account: siAnthropic,
  gemini: siGooglegemini,
  gemini_account: siGooglegemini,
  google: siGoogle,
  wecom: siWechat,
  email: siGmail,
} as const

const textMap: Partial<Record<BrandKey, string>> = {
  dingtalk: '钉',
  feishu: '飞',
  newapi: 'NA',
  newapi_user: 'NA',
  newapi_token: 'NA',
  sub2api: 'S2',
  sub2api_user: 'S2',
  sub2api_token: 'S2',
  manual_subscription: 'M',
  generic_http: 'HTTP',
}

const openAIPath = 'M22.2819 9.8211a5.9847 5.9847 0 0 0-.5157-4.9108 6.0462 6.0462 0 0 0-6.5098-2.9A6.0651 6.0651 0 0 0 4.9817 4.1818a5.9847 5.9847 0 0 0-3.9977 2.9 6.0462 6.0462 0 0 0 .7427 7.0966 5.98 5.98 0 0 0 .511 4.9107 6.051 6.051 0 0 0 6.5146 2.9001A5.9847 5.9847 0 0 0 13.2599 24a6.0557 6.0557 0 0 0 5.7718-4.2058 5.9894 5.9894 0 0 0 3.9977-2.9001 6.0557 6.0557 0 0 0-.7475-7.0729zm-9.022 12.6081a4.4755 4.4755 0 0 1-2.8764-1.0408l.1419-.0804 4.7783-2.7582a.7948.7948 0 0 0 .3927-.6813v-6.7369l2.0207 1.1686a.071.071 0 0 1 .038.052v5.5826a4.504 4.504 0 0 1-4.4945 4.4944zm-9.6607-4.1254a4.4708 4.4708 0 0 1-.5346-3.0137l.142.0852 4.783 2.7582a.7712.7712 0 0 0 .7806 0l5.8428-3.3685v2.3324a.0804.0804 0 0 1-.0332.0615L9.74 19.9502a4.4992 4.4992 0 0 1-6.1408-1.6464zM2.3408 7.8956a4.485 4.485 0 0 1 2.3655-1.9728v5.67a.7952.7952 0 0 0 .3927.6813l5.8144 3.3543-2.0207 1.1685a.0757.0757 0 0 1-.071 0l-4.8303-2.7865A4.504 4.504 0 0 1 2.3408 7.872zm16.5963 3.8558L13.1038 8.364 15.1197 7.2a.0757.0757 0 0 1 .071 0l4.8303 2.7913a4.4944 4.4944 0 0 1-.676 8.1042v-5.6649a.7759.7759 0 0 0-.4079-.6792zm2.0107-2.9233l-.142-.0852-4.7735-2.7818a.7759.7759 0 0 0-.7854 0L9.409 9.3304V6.998a.0662.0662 0 0 1 .0284-.0615l4.8303-2.7866a4.4992 4.4992 0 0 1 6.6802 4.66zM8.3065 12.863l-2.0207-1.1638a.0804.0804 0 0 1-.038-.0568V6.0742a4.4992 4.4992 0 0 1 7.3757-3.4537l-.142.0805-4.7783 2.7582a.7948.7948 0 0 0-.3927.6813zm1.0976-2.264 2.602-1.4998 2.6069 1.4998v2.9994l-2.5974 1.4997-2.6067-1.4997z'

export function BrandIcon({ kind, className = '' }: { kind: BrandKey; className?: string }) {
  if (kind === 'openai' || kind === 'openai_account' || kind === 'openai_admin' || kind === 'openai_key') {
    return (
      <span className={`brand-icon ${className}`} style={{ color: 'var(--text-1)' }}>
        <svg viewBox="0 0 24 24" aria-hidden="true">
          <path d={openAIPath} fill="currentColor" />
        </svg>
      </span>
    )
  }
  const icon = iconMap[kind as keyof typeof iconMap]
  if (icon) {
    return (
      <span className={`brand-icon ${className}`} style={{ color: `#${icon.hex}` }}>
        <svg viewBox="0 0 24 24" aria-hidden="true">
          <path d={icon.path} fill="currentColor" />
        </svg>
      </span>
    )
  }
  return <span className={`brand-icon brand-icon-text ${className}`}>{textMap[kind] ?? 'API'}</span>
}
