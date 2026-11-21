package system

import (
	"sun-panel/api/api_v1"
	"sun-panel/api/api_v1/middleware"

	"github.com/gin-gonic/gin"
)

func InitMonitorRouter(router *gin.RouterGroup) {
	api := api_v1.ApiGroupApp.ApiSystem.MonitorApi
	r := router.Group("", middleware.LoginInterceptor)
	r.POST("/system/monitor/getDiskMountpoints", api.GetDiskMountpoints)
	r.POST("/system/monitor/getDiskStateByPath", api.GetDiskStateByPath)
	r.POST("/system/monitor/getAll", api.GetAll)

	// 访客可查看 CPU 和内存的面板指标。
	rPublic := router.Group("", middleware.PublicModeInterceptor)
	rPublic.POST("/system/monitor/getCpuState", api.GetCpuState)
	rPublic.POST("/system/monitor/getMemonyState", api.GetMemonyState)
}
