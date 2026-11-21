package router

import (
	"fmt"
	"strings"

	"sun-panel/global"
	"sun-panel/lib/safefile"
	// "sun-panel/router/admin"
	"sun-panel/router/openness"
	"sun-panel/router/panel"
	"sun-panel/router/system"

	"github.com/gin-gonic/gin"
)

// 初始化总路由
func InitRouters(addr string) error {
	router := gin.Default()
	var trustedProxies []string
	for _, proxy := range strings.Split(global.Config.GetValueString("security", "trusted_proxies"), ",") {
		if proxy = strings.TrimSpace(proxy); proxy != "" {
			trustedProxies = append(trustedProxies, proxy)
		}
	}
	if err := router.SetTrustedProxies(trustedProxies); err != nil {
		return err
	}

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
		router.StaticFile("/login", webPath+"/index.html")
		router.Static("/assets", webPath+"/assets")
		router.Static("/custom", webPath+"/custom")
		router.StaticFile("/favicon.ico", webPath+"/favicon.ico")
		router.StaticFile("/favicon.svg", webPath+"/favicon.svg")
	}

	// 上传的文件
	sourcePath := global.Config.GetValueString("base", "source_path")
	prefix, err := safefile.UploadPrefix(sourcePath)
	if err != nil {
		return fmt.Errorf("invalid source_path: %w", err)
	}
	first, _, _ := strings.Cut(strings.TrimPrefix(prefix, "/"), "/")
	switch first {
	case "api", "assets", "custom", "login", "favicon.ico", "favicon.svg":
		return fmt.Errorf("source_path conflicts with a reserved route: %s", prefix)
	}
	serveUploads := gin.WrapF(safefile.ServeUploads(sourcePath, prefix))
	router.GET(prefix+"/*upload", serveUploads)
	router.HEAD(prefix+"/*upload", serveUploads)

	global.Logger.Info("Sun-Panel is Started.  Listening and serving HTTP on ", addr)
	return router.Run(addr)
}
