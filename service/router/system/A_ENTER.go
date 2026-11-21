package system

import "github.com/gin-gonic/gin"

func Init(routerGroup *gin.RouterGroup) {
	InitAbout(routerGroup)
	InitLogin(routerGroup)
	InitSecurityRouter(routerGroup)
	InitCaptchaRouter(routerGroup)
	InitUserRouter(routerGroup)
	InitFileRouter(routerGroup)
	InitNoticeRouter(routerGroup)
	InitModuleConfigRouter(routerGroup)
	InitMonitorRouter(routerGroup)
}
