package panel

import (
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"sun-panel/api/api_v1/common/apiData/panelApiStructs"
	"sun-panel/api/api_v1/common/apiReturn"
	"sun-panel/api/api_v1/common/base"
	"sun-panel/global"
	"sun-panel/lib/siteFavicon"
	"sun-panel/models"

	"github.com/gin-gonic/gin"
)

// 内网可达性探测。
//
// 用途：在「自动模式」下判断某个书签的内网地址是否可以从面板所在网络直接访问。
// 之所以放在后端做，是因为 https 页面无法用 fetch 探测 http 内网地址
// （浏览器混合内容策略会在请求发出前就拦截），前端拿不到任何延迟数据。
//
// 安全约束：
//   - 只接受书签 ID，不接受任意 URL，避免被当成内网端口扫描器；
//   - 只做 TCP 拨号，不发送应用层数据、不读取响应内容；
//   - 单次最多 20 条、并发 8、单次拨号 1.5s 超时；
//   - 同一 host:port 结果缓存 60s，单用户 3s 内只允许触发一次实际探测；
//   - 可通过 conf.ini 的 probe_enabled=0 整体关闭。

const (
	// 单次 TCP 拨号超时
	probeDialTimeout = 1500 * time.Millisecond
	// 单次请求最多探测的书签数量
	probeMaxItems = 20
	// 拨号并发上限
	probeConcurrency = 8
	// 探测结果缓存时长
	probeCacheTTL = 60 * time.Second
	// 同一用户两次实际探测的最小间隔
	probeUserInterval = 3 * time.Second
)

// probeCacheEntry 地址级探测缓存
type probeCacheEntry struct {
	ok bool
	ms int
	at time.Time
}

var (
	probeCacheMu sync.Mutex
	probeCache   = map[string]probeCacheEntry{}

	probeUserMu   sync.Mutex
	probeUserLast = map[uint]time.Time{}
)

// probeEnabled 读取配置，默认开启
func probeEnabled() bool {
	v := global.Config.GetValueStringOrDefault("base", "probe_enabled")
	if v == "" {
		return true
	}
	return v != "0" && !strings.EqualFold(v, "false")
}

// probeHostPort 从 URL 中取出可拨号的 host:port，非法或非 http(s) 返回空
func probeHostPort(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(siteFavicon.NormalizeURL(rawURL))
	if err != nil {
		return ""
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return ""
	}
	host := u.Hostname()
	if host == "" {
		return ""
	}
	port := u.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return net.JoinHostPort(host, port)
}

// dialOnce 拨号一次，返回是否连通与耗时
func dialOnce(addr string) (bool, int) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, probeDialTimeout)
	ms := int(time.Since(start).Milliseconds())
	if err != nil {
		return false, 0
	}
	_ = conn.Close()
	// 本机回环可能不足 1ms，向上取整避免和「不可达」的 0 混淆
	if ms < 1 {
		ms = 1
	}
	return true, ms
}

// probeAddr 带缓存的拨号
func probeAddr(addr string) (bool, int) {
	if addr == "" {
		return false, 0
	}

	probeCacheMu.Lock()
	if entry, ok := probeCache[addr]; ok && time.Since(entry.at) < probeCacheTTL {
		probeCacheMu.Unlock()
		return entry.ok, entry.ms
	}
	probeCacheMu.Unlock()

	ok, ms := dialOnce(addr)

	probeCacheMu.Lock()
	// 顺带清理过期项，避免 map 无限增长
	for k, v := range probeCache {
		if time.Since(v.at) >= probeCacheTTL {
			delete(probeCache, k)
		}
	}
	probeCache[addr] = probeCacheEntry{ok: ok, ms: ms, at: time.Now()}
	probeCacheMu.Unlock()

	return ok, ms
}

// allowUserProbe 限流：同一用户 3s 内只允许触发一次实际探测
func allowUserProbe(userId uint) bool {
	probeUserMu.Lock()
	defer probeUserMu.Unlock()
	now := time.Now()
	if last, ok := probeUserLast[userId]; ok && now.Sub(last) < probeUserInterval {
		return false
	}
	probeUserLast[userId] = now
	return true
}

// clientIsLan 判断本次请求的来源地址是否为私网。
// 反向代理场景下 gin 需要正确配置 trusted_proxies 才能拿到真实来源 IP。
//
// 注意：来源地址是回环地址、且请求里没有任何代理头时，无法区分
// 「用户就在面板本机」和「同机反向代理没有传递来源头」两种情况，
// 此时不下「在内网」的结论——宁可给出外网地址（一定可用），
// 也不要给出一个从公网访问不到的私网地址。
func clientIsLan(c *gin.Context) bool {
	ip := net.ParseIP(c.ClientIP())
	if ip == nil {
		return false
	}
	if !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()) {
		return false
	}
	if ip.IsLoopback() &&
		c.GetHeader("X-Forwarded-For") == "" &&
		c.GetHeader("X-Real-IP") == "" {
		return false
	}
	return true
}

// ProbeNetwork 探测书签内网地址是否可从面板所在网络访问
// POST /panel/itemIcon/probeNetwork  {"ids":[1,2,3]}
func (a *ItemIcon) ProbeNetwork(c *gin.Context) {
	if !probeEnabled() {
		apiReturn.Error(c, "probe disabled")
		return
	}

	userInfo, exist := base.GetCurrentUserInfo(c)
	if !exist {
		apiReturn.ErrorNoAccess(c)
		return
	}

	var req panelApiStructs.ItemIconProbeNetworkReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiReturn.ErrorParamFomat(c, err.Error())
		return
	}
	if len(req.Ids) == 0 {
		apiReturn.ErrorParamFomat(c, "ids is empty")
		return
	}
	if len(req.Ids) > probeMaxItems {
		req.Ids = req.Ids[:probeMaxItems]
	}

	// 只查当前用户自己的书签，杜绝越权探测
	var items []models.ItemIcon
	if err := global.Db.
		Where("id IN ? AND user_id = ?", req.Ids, userInfo.ID).
		Find(&items).Error; err != nil {
		apiReturn.ErrorDatabase(c, err.Error())
		return
	}

	resp := panelApiStructs.ItemIconProbeNetworkResp{
		ClientIsLan: clientIsLan(c),
		Results:     make([]panelApiStructs.ItemIconProbeNetworkResult, 0, len(items)),
	}

	// 限流命中时不真正拨号，直接返回（前端会用上一次的缓存结果）
	if !allowUserProbe(userInfo.ID) {
		resp.Throttled = true
		apiReturn.SuccessData(c, resp)
		return
	}

	// 只探测内网地址：内网通就用内网，不通就用外网。
	// 内网直连不会比绕公网慢，因此无需再测外网做比较。
	sem := make(chan struct{}, probeConcurrency)
	var wg sync.WaitGroup
	results := make([]panelApiStructs.ItemIconProbeNetworkResult, len(items))
	for i := range items {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			item := items[idx]
			res := panelApiStructs.ItemIconProbeNetworkResult{Id: item.ID}
			if item.LanUrl != "" {
				res.LanOk, res.LanMs = probeAddr(probeHostPort(item.LanUrl))
			}
			results[idx] = res
		}(i)
	}
	wg.Wait()
	resp.Results = results

	global.Logger.Info("probeNetwork user=", userInfo.ID, " ids=", req.Ids, " clientIsLan=", resp.ClientIsLan)
	apiReturn.SuccessData(c, resp)
}
