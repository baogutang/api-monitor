import { useQuery } from '@tanstack/react-query'
import { settingsApi } from '@/api/services'

export function BrandLogo({
  className = '',
  loadCustom = false,
  customSvg,
}: {
  className?: string
  loadCustom?: boolean
  customSvg?: string
}) {
  const settings = useQuery({
    queryKey: ['settings'],
    queryFn: settingsApi.get,
    enabled: loadCustom && customSvg == null,
    retry: false,
    staleTime: 60_000,
  })
  const svg =
    typeof customSvg === 'string'
      ? customSvg
      : typeof settings.data?.brandingLogoSvg === 'string'
        ? settings.data.brandingLogoSvg
        : ''
  const src = svg.trim()
    ? `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg.trim())}`
    : '/logo.svg'

  return <img className={`brand-logo-img ${className}`} src={src} alt="API Monitor" />
}
