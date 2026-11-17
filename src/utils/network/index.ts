import { PanelStateNetworkModeEnum } from '@/enums'
import { probeNetwork } from '@/api/panel/itemIcon'

/**
 * 判断当前页面是否通过局域网地址访问。
 * 仅在无法做延迟探测时（例如 https 页面探测 http 地址会被混合内容策略拦截）作为兜底。
 */
export function isLanAccess(): boolean {
  try {
    const host = (window.location.hostname || '').toLowerCase().replace(/^\[|\]$/g, '')
    if (!host)
      return false

    if (
      host === 'localhost'
      || host.endsWith('.localhost')
      || host.endsWith('.local')
      || host.endsWith('.lan')
      || host.endsWith('.internal')
    ) {
      return true
    }

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

    if (host === '::1' || /^f[cd]/.test(host) || host.startsWith('fe80:'))
      return true

    return !host.includes('.') && !host.includes(':')
  }
  catch {
    return false
  }
}

/** 预热探测超时（毫秒）。内网一般几毫秒就返回，公网超出这个时间说明内网并不通。 */
const PROBE_TIMEOUT = 1500
/** 用户点击时的探测超时：比预热更短，保证点击不会明显卡顿。 */
const CLICK_PROBE_TIMEOUT = 800
/** 探测结果缓存时长。 */
const CACHE_TTL = 5 * 60 * 1000

interface ProbeResult {
  /** 往返耗时（毫秒），不可达为 Infinity */
  ms: number
  /** 是否因浏览器策略无法探测（https 页面探测 http 地址） */
  blocked: boolean
}

interface CacheEntry {
  url: string
  at: number
}

/** 以「条目 id + 模式」为键缓存上一次的选择结果，避免每次点击都重新探测。 */
const pickCache = new Map<string, CacheEntry>()
/** 进行中的探测，避免同一地址被并发重复探测。 */
const probeInflight = new Map<string, Promise<ProbeResult>>()

function cacheKey(item: Panel.ItemInfo | null | undefined): string {
  if (!item)
    return ''
  return String(item.id ?? `${item.url}|${item.lanUrl}`)
}

function isMixedContentBlocked(url: string): boolean {
  try {
    return window.location.protocol === 'https:' && url.toLowerCase().startsWith('http://')
  }
  catch {
    return false
  }
}

/**
 * 探测一个地址的可达性与往返耗时。
 * 使用 no-cors 的 GET 请求：跨域也能拿到响应（内容不可读，但耗时是真实的）。
 */
export function probeLatency(url: string, timeout = PROBE_TIMEOUT): Promise<ProbeResult> {
  if (!url)
    return Promise.resolve({ ms: Number.POSITIVE_INFINITY, blocked: false })

  // https 页面无法 fetch http 地址（混合内容），直接标记为不可探测
  if (isMixedContentBlocked(url))
    return Promise.resolve({ ms: Number.POSITIVE_INFINITY, blocked: true })

  const cached = probeInflight.get(url)
  if (cached)
    return cached

  const task = new Promise<ProbeResult>((resolve) => {
    const controller = typeof AbortController !== 'undefined' ? new AbortController() : null
    const started = Date.now()
    let settled = false
    const finish = (result: ProbeResult) => {
      if (settled)
        return
      settled = true
      resolve(result)
    }
    const timer = setTimeout(() => {
      controller?.abort()
      finish({ ms: Number.POSITIVE_INFINITY, blocked: false })
    }, timeout)

    fetch(url, {
      method: 'GET',
      mode: 'no-cors',
      cache: 'no-store',
      redirect: 'follow',
      signal: controller?.signal,
    })
      .then(() => {
        clearTimeout(timer)
        finish({ ms: Date.now() - started, blocked: false })
      })
      .catch(() => {
        clearTimeout(timer)
        finish({ ms: Number.POSITIVE_INFINITY, blocked: false })
      })
  }).finally(() => {
    probeInflight.delete(url)
  })

  probeInflight.set(url, task)
  return task
}

/**
 * 比较内外网地址的延迟，返回应当优先使用的地址。
 * 规则：内网明显更快则用内网；两者都不可达时退回主机名判断。
 */
export async function pickFasterUrl(item: Panel.ItemInfo | null | undefined, timeout = PROBE_TIMEOUT): Promise<string> {
  const wanUrl = item?.url || ''
  const lanUrl = item?.lanUrl || ''
  if (!lanUrl)
    return wanUrl
  if (!wanUrl)
    return lanUrl

  const [lanProbe, wanProbe] = await Promise.all([probeLatency(lanUrl, timeout), probeLatency(wanUrl, timeout)])

  // 内网地址探测被浏览器拦截时，无法比较延迟，退回主机名判断
  if (lanProbe.blocked)
    return isLanAccess() ? lanUrl : wanUrl

  const lanOk = Number.isFinite(lanProbe.ms)
  const wanOk = Number.isFinite(wanProbe.ms)

  if (lanOk && !wanOk)
    return lanUrl
  if (!lanOk && wanOk)
    return wanUrl
  if (!lanOk && !wanOk)
    return isLanAccess() ? lanUrl : wanUrl

  // 内网延迟不高于公网 15%（+10ms 容差）时优先内网：
  // 局域网带宽通常更好，而首次 DNS/TLS 建连的抖动会让公网偶尔看起来略快。
  return lanProbe.ms <= wanProbe.ms * 1.15 + 10 ? lanUrl : wanUrl
}

/** 同步解析地址：自动模式只用缓存结果，保证点击零延迟。 */
export function resolveNetworkUrl(item: Panel.ItemInfo | null | undefined, mode: PanelStateNetworkModeEnum | null | undefined): string {
  const wanUrl = item?.url || ''
  const lanUrl = item?.lanUrl || ''
  if (mode === PanelStateNetworkModeEnum.lan)
    return lanUrl || wanUrl

  if (mode === PanelStateNetworkModeEnum.wan)
    return wanUrl || lanUrl

  if (!lanUrl)
    return wanUrl

  const cached = pickCache.get(cacheKey(item))
  if (cached && cached.url && Date.now() - cached.at < CACHE_TTL)
    return cached.url

  // 还没有探测结果时先用公网地址，探测完成后会写入缓存
  return wanUrl
}

/** 异步解析地址：自动模式下若无缓存则现场比较延迟。 */
export async function resolveNetworkUrlAsync(item: Panel.ItemInfo | null | undefined, mode: PanelStateNetworkModeEnum | null | undefined): Promise<string> {
  const wanUrl = item?.url || ''
  const lanUrl = item?.lanUrl || ''
  if (mode === PanelStateNetworkModeEnum.lan)
    return lanUrl || wanUrl

  if (mode === PanelStateNetworkModeEnum.wan)
    return wanUrl || lanUrl

  if (!lanUrl)
    return wanUrl

  const key = cacheKey(item)
  const cached = pickCache.get(key)
  if (cached && cached.url && Date.now() - cached.at < CACHE_TTL)
    return cached.url

  // https 页面探测 http 内网地址会被浏览器拦截，改由后端代测
  if (isMixedContentBlocked(lanUrl)) {
    await runServerProbe([item])
    const after = pickCache.get(key)
    if (after && after.url)
      return after.url
    // 后端也拿不到结果时退回主机名判断；
    // 若后端已告知本次访问来自内网，则直接信任它
    const onLan = serverProbeAt ? serverClientIsLan : isLanAccess()
    return onLan ? lanUrl : wanUrl
  }

  // 点击路径用更短的超时，宁可这次选公网也不要让用户等
  const url = await pickFasterUrl(item, CLICK_PROBE_TIMEOUT)
  pickCache.set(key, { url, at: Date.now() })
  return url
}

/** 后端探测状态：记录本次请求来源是否为私网 */
let serverProbeAt = 0
let serverClientIsLan = false

/** 后端探测结果缓存时长，与前端一致 */
const SERVER_PROBE_TTL = 5 * 60 * 1000

/**
 * 由后端探测内网地址是否可直连。
 * 只传书签 ID，后端从数据库取出地址后做 TCP 拨号，不接受任意 URL。
 */
async function runServerProbe(items: Panel.ItemInfo[]) {
  const targets = items.filter(item => item && item.id && item.lanUrl && item.url).slice(0, WARM_LIMIT)
  if (targets.length === 0)
    return

  try {
    const { code, data } = await probeNetwork<Common.ProbeNetworkResponse>(targets.map(item => item.id as number))
    if (code !== 0 || !data)
      return

    serverProbeAt = Date.now()
    serverClientIsLan = !!data.clientIsLan

    const byId = new Map<number, Common.ProbeNetworkResult>()
    for (const r of data.results || [])
      byId.set(r.id, r)

    for (const item of targets) {
      const r = byId.get(item.id as number)
      if (!r)
        continue
      // 内网可直连就用内网，否则用外网
      const url = r.lanOk ? item.lanUrl : item.url
      if (url)
        pickCache.set(cacheKey(item), { url, at: Date.now() })
    }
  }
  catch {
    // 接口失败时不影响正常使用，前端会退回其它判断方式
  }
}

/** 预热上限：书签很多时只预热前若干个，其余在点击时再测速。 */
const WARM_LIMIT = 60

/**
 * 预热：页面加载后后台探测所有配置了内网地址的条目，
 * 这样用户真正点击时可以直接命中缓存。
 */
export function warmUpNetworkLatency(items: Panel.ItemInfo[] | null | undefined, concurrency = 3) {
  if (!items || items.length === 0)
    return
  const targets = items.filter(item => item && item.lanUrl && item.url).slice(0, WARM_LIMIT)
  if (targets.length === 0)
    return

  const run = () => {
    // https 页面无法探测 http 内网地址，交给后端代测
    if (targets.some(item => isMixedContentBlocked(item.lanUrl))) {
      runServerProbe(targets)
      return
    }

    let index = 0
    const worker = async () => {
      while (index < targets.length) {
        const item = targets[index++]
        try {
          const url = await pickFasterUrl(item)
          pickCache.set(cacheKey(item), { url, at: Date.now() })
        }
        catch {
          // 预热失败不影响正常使用
        }
      }
    }
    for (let i = 0; i < Math.min(concurrency, targets.length); i++)
      worker()
  }

  // 空闲时再探测，避免和首页渲染抢带宽
  if (typeof requestIdleCallback === 'function')
    requestIdleCallback(run, { timeout: 2000 })
  else
    setTimeout(run, 300)
}

/** 清空缓存（切换网络模式时调用，让下次点击重新判断）。 */
export function clearNetworkLatencyCache() {
  pickCache.clear()
  probeInflight.clear()
  serverProbeAt = 0
  serverClientIsLan = false
}

/** 供设置页显示用：读取某个条目上一次的判定结果。 */
export function getCachedChoice(item: Panel.ItemInfo | null | undefined): 'lan' | 'wan' | '' {
  const cached = pickCache.get(cacheKey(item))
  if (!cached || Date.now() - cached.at >= CACHE_TTL)
    return ''
  if (!item)
    return ''
  return cached.url === item.lanUrl ? 'lan' : 'wan'
}
