package router

import (
	"strings"
	"sun-panel/global"
	// "sun-panel/router/admin"
	"sun-panel/router/openness"
	"sun-panel/router/panel"
	"sun-panel/router/system"

	"github.com/gin-gonic/gin"
)

// 初始化总路由
func InitRouters(addr string) error {
	router := gin.Default()
	// 配置可信反向代理，否则 ClientIP() 永远返回反代地址，
	// 「自动模式」判断来源是否为内网时会误判。
	applyTrustedProxies(router)
	router.Use(webNoCache())
	rootRouter := router.Group("/")
	routerGroup := rootRouter.Group("api")

	// 接口
	system.Init(routerGroup)
	panel.Init(routerGroup)
	openness.Init(routerGroup)

	// WEB文件服务
	{
		webPath := "./web"
		router.StaticFile("/", webPath+"/index.html")
		router.StaticFile("/index.html", webPath+"/index.html")
		router.StaticFile("/sw.js", webPath+"/sw.js")
		router.StaticFile("/service-worker.js", webPath+"/service-worker.js")
		router.Static("/assets", webPath+"/assets")
		router.Static("/custom", webPath+"/custom")
		router.StaticFile("/favicon.ico", webPath+"/favicon.ico")
		router.StaticFile("/favicon.svg", webPath+"/favicon.svg")
	}

	// 上传的文件
	sourcePath := global.Config.GetValueString("base", "source_path")
	router.Static(sourcePath[1:], sourcePath)

	global.Logger.Info("XH-Panel is Started.  Listening and serving HTTP on ", addr)
	return router.Run(addr)
}

// applyTrustedProxies 按配置设置可信反向代理网段。
// 配置为空时退回 gin 的默认行为（不信任任何代理，ClientIP 取直连地址）。
func applyTrustedProxies(router *gin.Engine) {
	raw := global.Config.GetValueStringOrDefault("base", "trusted_proxies")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}
	proxies := []string{}
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			proxies = append(proxies, item)
		}
	}
	if len(proxies) == 0 {
		return
	}
	if err := router.SetTrustedProxies(proxies); err != nil {
		global.Logger.Warn("SetTrustedProxies failed, fallback to no proxy trust: ", err.Error())
	}
}

func webNoCache() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/" ||
			path == "/index.html" ||
			path == "/sw.js" ||
			path == "/service-worker.js" ||
			path == "/favicon.ico" ||
			path == "/favicon.svg" ||
			strings.HasPrefix(path, "/assets/") ||
			strings.HasPrefix(path, "/custom/") {
			c.Header("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate, max-age=0")
			c.Header("Pragma", "no-cache")
			c.Header("Expires", "0")
			c.Header("Surrogate-Control", "no-store")
			if path == "/" ||
				path == "/index.html" ||
				path == "/sw.js" ||
				path == "/service-worker.js" {
				c.Header("Clear-Site-Data", `"cache"`)
			}
		}
		c.Next()
	}
}
