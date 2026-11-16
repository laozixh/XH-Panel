package siteFavicon

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"sun-panel/lib/cmn"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	// 单次 HTTP 请求超时
	requestTimeout = 8 * time.Second
	// 整个图标获取流程的总超时
	fetchTimeout = 20 * time.Second
	// 页面最多读取多少字节用于解析 <link rel="icon">
	pageMaxBytes = 2 * 1024 * 1024
	// 默认图标大小上限
	defaultMaxIconSize = 1024 * 1024

	browserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

// ErrFaviconNotFound 表示页面与默认路径都没有找到可用图标
var ErrFaviconNotFound = errors.New("favicon not found")

// 页面没有声明 <link rel="icon"> 时依次尝试的默认图标路径
var defaultFaviconPaths = []string{
	"/favicon.ico",
	"/favicon.png",
	"/apple-touch-icon.png",
}

// 允许保存的图标扩展名（与后台上传白名单保持一致）
var allowedIconExts = map[string]bool{
	".ico":  true,
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".webp": true,
	".svg":  true,
}

func IsHTTPURL(url string) bool {
	httpPattern := `^(http://|https://|//)`
	match, err := regexp.MatchString(httpPattern, url)
	if err != nil {
		return false
	}
	return match
}

// NormalizeURL 把用户输入的地址补全成可请求的绝对地址。
// 没有协议时：内网/本机地址优先 http，公网地址优先 https。
func NormalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return raw
	}
	if isLikelyLocalHost(raw) {
		return "http://" + raw
	}
	return "https://" + raw
}

// pageURLCandidates 返回按优先级排列的页面地址（无协议时给出 http/https 两种）
func pageURLCandidates(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return []string{raw}
	}
	if isLikelyLocalHost(raw) {
		return []string{"http://" + raw, "https://" + raw}
	}
	return []string{"https://" + raw, "http://" + raw}
}

func isLikelyLocalHost(raw string) bool {
	host := raw
	if u, err := url.Parse("//" + raw); err == nil && u.Host != "" {
		host = u.Hostname()
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" {
		return false
	}
	if host == "localhost" || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".lan") || strings.HasSuffix(host, ".internal") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
	}
	// 没有点的主机名（如 nas、router）按内网处理
	return !strings.Contains(host, ".")
}

func newHTTPClient() *http.Client {
	return &http.Client{Timeout: requestTimeout}
}

func newBrowserRequest(ctx context.Context, method, rawURL string, accept string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", browserUserAgent)
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	return req, nil
}

// pageInfo 是抓取页面后得到的图标线索
type pageInfo struct {
	base     *url.URL // 跟随跳转后的最终地址
	icons    []string // 页面声明的图标（绝对地址，按优先级排序）
	ogImage  string   // og:image，作为最后的兜底
	reached  bool     // 是否拿到了 HTTP 响应（哪怕状态码不是 200）
	lastErr  error
	pageBody []byte
}

func fetchPage(ctx context.Context, rawURL string) *pageInfo {
	info := &pageInfo{}
	var lastErr error
	for _, candidate := range pageURLCandidates(rawURL) {
		req, err := newBrowserRequest(ctx, http.MethodGet, candidate, "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
		if err != nil {
			lastErr = err
			continue
		}
		resp, err := newHTTPClient().Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, pageMaxBytes))
		resp.Body.Close()

		base := resp.Request.URL
		if base == nil {
			base, _ = url.Parse(candidate)
		}
		info.base = base
		info.reached = true

		if readErr != nil {
			lastErr = readErr
			continue
		}
		// 有些站点会对不带协议的裸域名返回 3xx 到 https，这里只看是否拿到正文
		if resp.StatusCode < 200 || resp.StatusCode >= 400 {
			// 不少站点的反爬/拦截页（403）依然带着完整的 <head>，
			// 里面的图标地址往往可以直接下载（例如 sciencedirect 的 CDN 图标），
			// 所以先把正文解析一遍，解析到了就当作成功。
			parseIconLinks(info, body)
			if len(info.icons) > 0 || info.ogImage != "" {
				info.pageBody = body
				return info
			}
			lastErr = fmt.Errorf("HTTP request failed with status code %d", resp.StatusCode)
			continue
		}
		info.pageBody = body
		parseIconLinks(info, body)
		return info
	}
	info.lastErr = lastErr
	if info.base == nil {
		if u, err := url.Parse(NormalizeURL(rawURL)); err == nil {
			info.base = u
		}
	}
	return info
}

func parseIconLinks(info *pageInfo, body []byte) {
	if info.base == nil || len(body) == 0 {
		return
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return
	}

	type iconLink struct {
		href  string
		score int
	}
	links := make([]iconLink, 0, 8)

	doc.Find("link").Each(func(i int, s *goquery.Selection) {
		rel, _ := s.Attr("rel")
		href, _ := s.Attr("href")
		href = strings.TrimSpace(href)
		if href == "" {
			return
		}
		lrel := strings.ToLower(rel)
		if !strings.Contains(lrel, "icon") {
			return
		}
		// data: URI（内联 base64 图标）无法直接下载，跳过
		if strings.HasPrefix(strings.ToLower(href), "data:") {
			return
		}
		score := 10
		switch {
		case strings.Contains(lrel, "shortcut"):
			score = 12
		case strings.Contains(lrel, "mask-icon"), strings.Contains(lrel, "fluid-icon"):
			// 单色蒙版图标，观感差，放到最后
			score = 1
		case strings.Contains(lrel, "apple-touch-icon"):
			score = 6
		}
		links = append(links, iconLink{href: href, score: score})
	})

	sort.SliceStable(links, func(i, j int) bool { return links[i].score > links[j].score })

	seen := map[string]bool{}
	for _, l := range links {
		abs := resolveAgainst(info.base, l.href)
		if abs == "" || seen[abs] {
			continue
		}
		seen[abs] = true
		info.icons = append(info.icons, abs)
	}

	// 兜底：社交分享图（一般是大图，但总比没有好）
	doc.Find("meta").EachWithBreak(func(i int, s *goquery.Selection) bool {
		prop, _ := s.Attr("property")
		name, _ := s.Attr("name")
		content, _ := s.Attr("content")
		key := strings.ToLower(strings.TrimSpace(prop + " " + name))
		if !strings.Contains(key, "og:image") && !strings.Contains(key, "twitter:image") {
			return true
		}
		content = strings.TrimSpace(content)
		if content == "" || strings.HasPrefix(strings.ToLower(content), "data:") {
			return true
		}
		if abs := resolveAgainst(info.base, content); abs != "" {
			info.ogImage = abs
			return false
		}
		return true
	})
}

func resolveAgainst(base *url.URL, href string) string {
	ref, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return ""
	}
	abs := base.ResolveReference(ref)
	if abs.Scheme != "http" && abs.Scheme != "https" {
		return ""
	}
	if abs.Host == "" {
		return ""
	}
	return abs.String()
}

// FaviconCandidates 返回该页面可尝试的图标地址（绝对地址，按优先级排序）。
// 顺序：页面声明的图标 → 站点根目录常见图标 → og:image。
func FaviconCandidates(rawURL string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()
	return faviconCandidates(ctx, rawURL)
}

func faviconCandidates(ctx context.Context, rawURL string) []string {
	list := faviconCandidateList(ctx, rawURL)
	out := make([]string, 0, len(list))
	for _, c := range list {
		out = append(out, c.url)
	}
	return out
}

// faviconCandidate 是候选图标地址及其来源标记
type faviconCandidate struct {
	url        string
	isFallback bool // og:image 等分享图，通常不是正方形图标
}

// faviconCandidateList 返回候选图标地址（按优先级），并标记兜底项
func faviconCandidateList(ctx context.Context, rawURL string) []faviconCandidate {
	info := fetchPage(ctx, rawURL)

	out := make([]faviconCandidate, 0, 8)
	seen := map[string]bool{}
	add := func(u string, fallback bool) {
		if u == "" || seen[u] {
			return
		}
		seen[u] = true
		out = append(out, faviconCandidate{url: u, isFallback: fallback})
	}

	for _, u := range info.icons {
		add(u, false)
	}

	// 页面抓取到的是网络层错误（域名解析/连接失败）时，再猜默认路径也没有意义
	base := info.base
	if base != nil && (info.reached || info.lastErr == nil) {
		origin := &url.URL{Scheme: base.Scheme, Host: base.Host}
		for _, p := range defaultFaviconPaths {
			add(origin.String()+p, false)
		}
		if info.ogImage != "" {
			add(info.ogImage, true)
		}
	}
	return out
}

// GetOneFaviconURL 返回页面首选的图标地址（不校验是否可下载）。
func GetOneFaviconURL(urlStr string) (string, error) {
	candidates := FaviconCandidates(urlStr)
	if len(candidates) == 0 {
		return "", ErrFaviconNotFound
	}
	return candidates[0], nil
}

// GetOneFaviconURLAndUpload 保留旧接口：返回首个候选图标地址
func GetOneFaviconURLAndUpload(urlStr string) (string, bool) {
	candidates := FaviconCandidates(urlStr)
	if len(candidates) == 0 {
		return "", false
	}
	return candidates[0], true
}

// fetchImage 下载图标内容并推断扩展名，非图片内容会被拒绝
func fetchImage(ctx context.Context, rawURL string, maxSize int64) ([]byte, string, error) {
	if maxSize <= 0 {
		maxSize = defaultMaxIconSize
	}
	req, err := newBrowserRequest(ctx, http.MethodGet, rawURL, "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	if err != nil {
		return nil, "", err
	}
	resp, err := newHTTPClient().Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP request failed, status code: %d", resp.StatusCode)
	}
	if resp.ContentLength > maxSize {
		return nil, "", fmt.Errorf("文件太大，不下载。大小：%d字节", resp.ContentLength)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSize+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > maxSize {
		return nil, "", fmt.Errorf("文件太大，不下载。大小：%d字节", len(data))
	}
	if len(data) == 0 {
		return nil, "", errors.New("图标内容为空")
	}

	ext := detectImageExt(data, rawURL)
	if ext == "" {
		return nil, "", errors.New("返回内容不是可识别的图片")
	}
	return data, ext, nil
}

// detectImageExt 通过内容嗅探判断图片类型，识别失败时回退到 URL 后缀
func detectImageExt(data []byte, rawURL string) string {
	contentType := http.DetectContentType(data)
	switch {
	case strings.Contains(contentType, "icon"):
		return ".ico"
	case strings.Contains(contentType, "svg"):
		return ".svg"
	case strings.Contains(contentType, "png"):
		return ".png"
	case strings.Contains(contentType, "jpeg"):
		return ".jpg"
	case strings.Contains(contentType, "gif"):
		return ".gif"
	case strings.Contains(contentType, "webp"):
		return ".webp"
	case strings.Contains(contentType, "bmp"):
		return ".bmp"
	}
	// Go 的嗅探器不识别 svg，用文本特征再判一次
	if strings.HasPrefix(contentType, "text/") || strings.Contains(contentType, "xml") {
		head := data
		if len(head) > 1024 {
			head = head[:1024]
		}
		if bytes.Contains(bytes.ToLower(head), []byte("<svg")) {
			return ".svg"
		}
		return ""
	}
	// 最后按 URL 后缀兜底
	if u, err := url.Parse(rawURL); err == nil {
		if ext := strings.ToLower(path.Ext(u.Path)); allowedIconExts[ext] {
			return ext
		}
	}
	return ""
}

// IconOption 是一个可选的站点图标
type IconOption struct {
	SourceURL  string // 图标原始地址
	Data       []byte // 图标原始内容
	Preview    string // base64 缩略图，供前端展示
	Width      int    // 原始宽度（SVG 为 0）
	Height     int    // 原始高度（SVG 为 0）
	Ext        string // 原始扩展名
	IsFallback bool   // 是否为 og:image 之类的分享图兜底
}

// quality 表示图标的清晰度权重：矢量图最清晰，其余取短边像素。
func (o IconOption) quality() int {
	if o.Ext == ".svg" {
		return 1024
	}
	if o.Width < o.Height {
		return o.Width
	}
	return o.Height
}

// FetchIcon 下载单个图标地址的内容
func FetchIcon(rawURL string, maxSize int64) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()
	return fetchImage(ctx, rawURL, maxSize)
}

// CollectIconOptions 抓取页面并下载所有可用的候选图标，供用户挑选。
// 返回结果按清晰度从高到低排序（矢量图最优先），并剔除内容完全相同的重复项。
func CollectIconOptions(pageURL string, maxSize int64, limit int) ([]IconOption, error) {
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout*2)
	defer cancel()
	return collectIconOptions(ctx, pageURL, maxSize, limit, 0)
}

// collectIconOptions 是 CollectIconOptions 的内部实现。
// stopQuality > 0 时，一旦拿到清晰度达到该值的非兜底图标就提前结束，
// 用于"自动取图标"场景，避免为了比较清晰度而下载过多候选。
func collectIconOptions(ctx context.Context, pageURL string, maxSize int64, limit int, stopQuality int) ([]IconOption, error) {
	candidates := faviconCandidateList(ctx, pageURL)
	if len(candidates) == 0 {
		return nil, ErrFaviconNotFound
	}
	if limit <= 0 {
		limit = 8
	}

	options := make([]IconOption, 0, limit)
	seen := map[string]bool{}
	for _, candidate := range candidates {
		if len(options) >= limit {
			break
		}
		data, ext, err := fetchImage(ctx, candidate.url, maxSize)
		if err != nil {
			continue
		}
		key := hashIcon(data)
		if seen[key] {
			continue
		}
		seen[key] = true
		width, height := IconDimensions(data)
		option := IconOption{
			SourceURL:  candidate.url,
			Data:       data,
			Preview:    MakePreview(data),
			Width:      width,
			Height:     height,
			Ext:        ext,
			IsFallback: candidate.isFallback,
		}
		options = append(options, option)
		if stopQuality > 0 && !option.IsFallback && option.quality() >= stopQuality {
			break
		}
	}
	if len(options) == 0 {
		return nil, ErrFaviconNotFound
	}

	// 清晰度优先；兜底的分享图排到最后
	sort.SliceStable(options, func(i, j int) bool {
		if options[i].IsFallback != options[j].IsFallback {
			return !options[i].IsFallback
		}
		return options[i].quality() > options[j].quality()
	})
	return options, nil
}

// SaveIconData 把图标内容统一尺寸后保存到 savePath，返回落盘文件名。
func SaveIconData(data []byte, savePath string) (string, error) {
	if strings.TrimSpace(savePath) == "" {
		return "", errors.New("save path is empty")
	}
	if err := os.MkdirAll(savePath, os.ModePerm); err != nil {
		return "", err
	}
	normalized, ext, _, _, err := NormalizeIcon(data)
	if err != nil {
		// 无法解码的图标（例如特殊格式）按原样保存，至少不丢图标
		normalized, ext = data, detectImageExt(data, "")
		if ext == "" {
			return "", err
		}
	}
	fileName := cmn.Md5(fmt.Sprintf("%s%d", time.Now().String(), len(normalized))) + ext
	if err := os.WriteFile(path.Join(savePath, fileName), normalized, 0o644); err != nil {
		return "", err
	}
	return fileName, nil
}

// SaveFavicon 抓取站点图标并保存到 savePath。
// 会挑选清晰度最高的一个（而不是页面声明的第一个），
// 这样 16x16 的小图标不会盖过同站点提供的 180x180 大图标。
// 返回实际使用的图标地址与落盘文件名（含扩展名）。
func SaveFavicon(pageURL, savePath string, maxSize int64) (string, string, error) {
	if strings.TrimSpace(savePath) == "" {
		return "", "", errors.New("save path is empty")
	}
	if err := os.MkdirAll(savePath, os.ModePerm); err != nil {
		return "", "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout*2)
	defer cancel()

	// 一旦拿到 >=64px 的正式图标就不再继续下载，兼顾速度与清晰度
	options, err := collectIconOptions(ctx, pageURL, maxSize, 8, 64)
	if err != nil {
		return "", "", err
	}

	fileName, err := SaveIconData(options[0].Data, savePath)
	if err != nil {
		return "", "", err
	}
	return options[0].SourceURL, fileName, nil
}

// GetRemoteFileSize 获取远程文件的大小（保留旧接口）
func GetRemoteFileSize(rawURL string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	req, err := newBrowserRequest(ctx, http.MethodHead, rawURL, "*/*")
	if err != nil {
		return 0, err
	}
	resp, err := newHTTPClient().Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("HTTP request failed, status code: %d", resp.StatusCode)
	}
	return resp.ContentLength, nil
}

// DownloadImage 下载图片到 savePath（保留旧接口，返回已落盘的文件句柄）
func DownloadImage(rawURL, savePath string, maxSize int64) (*os.File, error) {
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()

	data, ext, err := fetchImage(ctx, rawURL, maxSize)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(savePath, os.ModePerm); err != nil {
		return nil, err
	}
	fileName := cmn.Md5(fmt.Sprintf("%s%s%d", rawURL, time.Now().String(), len(data))) + ext
	destination := path.Join(savePath, fileName)
	if err := os.WriteFile(destination, data, 0o644); err != nil {
		return nil, err
	}
	return os.Open(destination)
}
