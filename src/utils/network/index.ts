import { PanelStateNetworkModeEnum } from '@/enums'

/**
 * 判断当前页面是否通过局域网地址访问。
 * 用于「自动模式」：在局域网内优先使用内网地址，在公网访问时使用外网地址。
 */
export function isLanAccess(): boolean {
  try {
    const host = (window.location.hostname || '').toLowerCase().replace(/^\[|\]$/g, '')
    if (!host)
      return false

    // 常见的内网域名后缀
    if (
      host === 'localhost'
      || host.endsWith('.localhost')
      || host.endsWith('.local')
      || host.endsWith('.lan')
      || host.endsWith('.internal')
    ) {
      return true
    }

    // IPv4 私有地址 / 回环 / 链路本地
    const ipv4 = host.match(/^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/)
    if (ipv4) {
      const a = Number(ipv4[1])
      const b = Number(ipv4[2])
      return (
        a === 10
        || a === 127
        || (a === 192 && b === 168)
        || (a === 172 && b >= 16 && b <= 31)
        || (a === 169 && b === 254)
      )
    }

    // IPv6 回环 / 唯一本地地址 / 链路本地
    if (host === '::1' || /^f[cd]/.test(host) || host.startsWith('fe80:'))
      return true

    // 不含点号的主机名（如 nas、router）通常只能在内网解析
    return !host.includes('.') && !host.includes(':')
  }
  catch {
    return false
  }
}

/**
 * 根据网络模式解析出实际要访问的地址。
 * - lan：优先内网地址，没有则回退外网地址
 * - wan：优先外网地址，没有则回退内网地址
 * - auto：当前在局域网内且配置了内网地址时使用内网地址，否则使用外网地址
 */
export function resolveNetworkUrl(item: Panel.ItemInfo | null | undefined, mode: PanelStateNetworkModeEnum | null | undefined): string {
  const wanUrl = item?.url || ''
  const lanUrl = item?.lanUrl || ''
  if (mode === PanelStateNetworkModeEnum.lan)
    return lanUrl || wanUrl

  if (mode === PanelStateNetworkModeEnum.wan)
    return wanUrl || lanUrl

  // 自动模式（以及历史数据中模式为空的情况）
  if (!lanUrl)
    return wanUrl

  return isLanAccess() ? lanUrl : wanUrl
}
